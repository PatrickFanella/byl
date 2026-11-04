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
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
)

// Worker claims and runs jobs.
type Worker struct {
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
	if workers < 1 {
		workers = 1
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
	runErr := w.dispatch(ctx, job)
	return true, w.finish(ctx, job, runErr)
}

// finish records the outcome. It uses a context that survives cancellation so a shutdown
// mid-job still leaves the row queued or done rather than stuck in running.
func (w *Worker) finish(ctx context.Context, job *models.Job, runErr error) error {
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if runErr == nil {
		return w.store.CompleteJob(wctx, job.ID)
	}
	interrupted := ctx.Err() != nil
	final := job.Attempts >= job.MaxAttempts && !interrupted
	w.log.Warn("job failed", "id", job.ID, "kind", job.Kind, "attempt", job.Attempts, "final", final, "interrupted", interrupted, "error", runErr)
	if final {
		if err := w.onFinalFailure(wctx, job, runErr); err != nil {
			w.log.Error("record final failure", "id", job.ID, "error", err)
		}
		return w.store.FailJob(wctx, job.ID, runErr.Error(), nil)
	}
	retry := w.now().Add(w.backoff(job.Attempts))
	if interrupted {
		retry = w.now()
	}
	return w.store.FailJob(wctx, job.ID, runErr.Error(), &retry)
}

func (w *Worker) dispatch(ctx context.Context, job *models.Job) error {
	switch job.Kind {
	case models.JobKindComputeRoute:
		var p models.ComputeRoutePayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode payload: %w", err)
		}
		return w.computeRoute(ctx, p)
	case models.JobKindGenerateNarration:
		var p models.GenerateNarrationPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode payload: %w", err)
		}
		return w.generateNarration(ctx, p)
	default:
		return fmt.Errorf("unknown job kind %q", job.Kind)
	}
}

func (w *Worker) onFinalFailure(ctx context.Context, job *models.Job, runErr error) error {
	if job.Kind != models.JobKindComputeRoute {
		return nil
	}
	var p models.ComputeRoutePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return nil
	}
	return w.store.UpsertRoute(ctx, models.TourRoute{TourID: p.TourID, Mode: p.Mode, Status: models.RouteStatusFailed, Error: runErr.Error()})
}
