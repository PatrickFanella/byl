// Package jobs runs background work from the jobs table.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/sources"
)

// ErrNoRetry marks a job failure that retrying cannot fix; finish records it as final at once.
var ErrNoRetry = errors.New("no retry")

// Worker claims and runs jobs.
type Worker struct {
	// Sources and Drafter are optional; jobs that need them fail without retry when unset.
	Sources *sources.Registry
	Drafter llm.Drafter
	// Research is an optional staged processor; absent adapters leave a named research gap.
	Research  ResearchProcessor
	Evidence  sources.EvidenceRetriever
	Discovery *sources.Discovery
	// Reconcile enables the local identity stage without requiring a model provider.
	Reconcile bool

	store    *repository.Store
	router   routing.Router
	narrator narration.Narrator
	blob     blob.Store
	workers  int
	log      *slog.Logger
	poll     time.Duration
	now      func() time.Time
	backoff  func(attempts int) time.Duration
}

// New builds a worker. workers is the number of concurrent goroutines Run starts.
func New(store *repository.Store, router routing.Router, narrator narration.Narrator, blobStore blob.Store, workers int, log *slog.Logger) *Worker {
	if workers < 0 {
		workers = 0
	}
	if log == nil {
		log = slog.Default()
	}
	return &Worker{store: store, router: router, narrator: narrator, blob: blobStore, workers: workers, log: log,
		poll: 2 * time.Second, now: time.Now, backoff: defaultBackoff}
}

// defaultBackoff is 30s, 60s, 120s, ...
func defaultBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	return 30 * time.Second * time.Duration(1<<(attempts-1))
}

// Run processes jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				processed, err := w.RunOnce(ctx)
				if err != nil && !errors.Is(err, context.Canceled) {
					w.log.Error("job loop", "error", err)
				}
				if processed {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(w.poll):
				}
			}
		}()
	}
	wg.Wait()
}

// staleAfter is how long a job may sit in running before another worker reclaims it.
const staleAfter = 10 * time.Minute

// RunOnce claims and runs at most one job. It reports whether a job was claimed.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	if n, err := w.store.ReclaimStaleJobs(ctx, staleAfter); err != nil {
		return false, err
	} else if n > 0 {
		w.log.Warn("reclaimed stale jobs", "count", n)
	}
	job, err := w.store.ClaimJob(ctx, w.now())
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}
	note, runErr := w.dispatch(ctx, job)
	return true, w.finish(ctx, job, note, runErr)
}

// finish records the outcome, with the job's own completion note when it produced one. It
// uses a context that survives cancellation so a shutdown mid-job still leaves the row queued
// or done rather than stuck in running.
func (w *Worker) finish(ctx context.Context, job *models.Job, note string, runErr error) error {
	fenced := job.Kind == models.JobKindResearchBrief || job.Kind == models.JobKindFetchEvidence
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if runErr == nil {
		if fenced {
			return w.store.FinishClaimedJob(wctx, job, note, nil, false)
		}
		if err := w.store.CompleteJob(wctx, job.ID); err != nil {
			return err
		}
		if note != "" {
			return w.store.SetJobNote(wctx, job.ID, note)
		}
		return nil
	}
	interrupted := ctx.Err() != nil
	final := (job.Attempts >= job.MaxAttempts || errors.Is(runErr, ErrNoRetry)) && !interrupted
	w.log.Warn("job failed", "id", job.ID, "kind", job.Kind, "attempt", job.Attempts, "final", final, "interrupted", interrupted, "error", runErr)
	if final {
		if err := w.onFinalFailure(wctx, job, runErr); err != nil {
			if fenced {
				// Keep the claim recoverable until the private brief outcome is durable.
				return err
			}
			w.log.Error("record final failure", "id", job.ID, "error", err)
		}
		if fenced {
			return w.store.FinishClaimedJob(wctx, job, runErr.Error(), nil, true)
		}
		return w.store.FailJob(wctx, job.ID, runErr.Error(), nil)
	}
	retry := w.now().Add(w.backoff(job.Attempts))
	if interrupted {
		retry = w.now()
	}
	if fenced {
		if retry.After(w.now().Add(30 * time.Second)) {
			retry = w.now().Add(30 * time.Second)
		}
		return w.store.FinishClaimedJob(wctx, job, runErr.Error(), &retry, false)
	}
	return w.store.FailJob(wctx, job.ID, runErr.Error(), &retry)
}

// dispatch runs the job and returns its completion note (empty for most kinds) or error.
func (w *Worker) dispatch(ctx context.Context, job *models.Job) (string, error) {
	switch job.Kind {
	case models.JobKindFetchEvidence:
		return w.fetchEvidence(ctx, job)
	case models.JobKindResearchBrief:
		return w.researchBrief(ctx, job)
	case models.JobKindComputeRoute:
		var p models.ComputeRoutePayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return "", fmt.Errorf("decode payload: %w", err)
		}
		err := w.computeRoute(ctx, p)
		if errors.Is(err, repository.ErrRouteSuperseded) {
			return "route_superseded", nil
		}
		return "", err
	case models.JobKindGenerateNarration:
		var p models.GenerateNarrationPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return "", fmt.Errorf("decode payload: %w", err)
		}
		return "", w.generateNarration(ctx, p)
	case models.JobKindImportSource:
		var p models.ImportSourcePayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return "", fmt.Errorf("decode payload: %w", err)
		}
		return w.importSource(ctx, p)
	case models.JobKindFetchMedia:
		var p models.FetchMediaPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return "", fmt.Errorf("decode payload: %w", err)
		}
		return "", w.fetchMedia(ctx, p)
	case models.JobKindDraftCandidate:
		var p models.DraftCandidatePayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return "", fmt.Errorf("decode payload: %w", err)
		}
		return "", w.draftCandidate(ctx, p)
	default:
		return "", fmt.Errorf("unknown job kind %q", job.Kind)
	}
}

func (w *Worker) onFinalFailure(ctx context.Context, job *models.Job, runErr error) error {
	if job.Kind == models.JobKindFetchEvidence {
		var p models.FetchEvidencePayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return nil
		}
		err := w.store.CompleteSourceEvidence(ctx, p.EvidenceID, job, models.EvidenceResult{Outcome: "worker_failed"})
		if errors.Is(err, repository.ErrResearchInactive) {
			return nil
		}
		return err
	}
	if job.Kind == models.JobKindResearchBrief {
		var p models.ResearchBriefPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return nil
		}
		err := w.store.StopBriefResearch(ctx, p.RunID, job, "worker_retry_exhausted")
		if errors.Is(err, repository.ErrResearchInactive) {
			return nil
		}
		return err
	}
	if job.Kind != models.JobKindComputeRoute {
		return nil
	}
	var p models.ComputeRoutePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return nil
	}
	var inputFailure routeInputFailure
	if !errors.As(runErr, &inputFailure) {
		return nil
	}
	failedRoute := models.TourRoute{TourID: p.TourID, Mode: p.Mode, Status: models.RouteStatusFailed, Error: runErr.Error()}
	var err error
	if inputFailure.quality != nil {
		failedRoute.Provider = inputFailure.quality.Provider
		inputFailure.quality.RouteSHA256 = failedRoute.RoutingFingerprint()
		err = w.store.UpsertRouteQualified(ctx, failedRoute, p.RouteVersion, inputFailure.digest, *inputFailure.quality)
	} else {
		err = w.store.UpsertRouteVersioned(ctx, failedRoute, p.RouteVersion, inputFailure.digest)
	}
	if errors.Is(err, repository.ErrRouteSuperseded) || errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	return err
}
