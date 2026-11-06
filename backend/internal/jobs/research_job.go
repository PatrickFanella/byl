package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

// ResearchTask grants one bounded provider operation. Implementations must honor
// ctx, perform at most one external request, and cap total model tokens at
// MaxTokens. They return proposed data only; persistence belongs to the worker.
// Source discovery/adapters are intentionally implemented in later roadmap issues.
type ResearchTask struct {
	RunID     string
	Stage     string
	Submitted models.RouteBriefInput
	Previous  map[string]json.RawMessage
	MaxTokens int
}

// ResearchProcessor performs one stage without writing application records.
type ResearchProcessor interface {
	Process(context.Context, ResearchTask) (models.ResearchOutput, error)
}

// ResearchFailure carries a safe code, never a raw provider response or URL.
type ResearchFailure struct{ Code string }

func (e ResearchFailure) Error() string { return researchFailureCode(e.Code) }

func researchFailureCode(code string) string {
	switch code {
	case "source_unavailable", "provider_unavailable", "insufficient_evidence", "invalid_output":
		return code
	}
	return "provider_failure"
}

func (w *Worker) researchBrief(ctx context.Context, job *models.Job) (string, error) {
	var p models.ResearchBriefPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return "", fmt.Errorf("%w: invalid research job payload", ErrNoRetry)
	}
	for _, stage := range models.ResearchStages {
		run, err := w.store.GetResearchRun(ctx, p.RunID)
		if err != nil {
			return "", errors.New("research checkpoint unavailable")
		}
		if run.Status != "active" || run.JobID != job.ID {
			return "research_stopped", nil
		}
		var completed bool
		previous := map[string]json.RawMessage{}
		for _, checkpoint := range run.Stages {
			if checkpoint.Status == "completed" {
				previous[checkpoint.Stage] = checkpoint.Output
				if checkpoint.Stage == stage {
					completed = true
				}
			}
		}
		if completed {
			continue
		}
		if w.Research == nil {
			if err := w.store.StopBriefResearch(ctx, run.ID, job, "processor_unavailable"); err != nil && !errors.Is(err, repository.ErrResearchInactive) {
				return "", errors.New("research checkpoint unavailable")
			}
			return "needs_research: processor_unavailable", nil
		}
		started, err := w.store.BeginResearchStage(ctx, run.ID, stage, job)
		if errors.Is(err, repository.ErrResearchInactive) {
			return "research_stopped", nil
		}
		if err != nil {
			return "", errors.New("research checkpoint unavailable")
		}
		if !started {
			continue
		}
		deadline := time.Now().Add(30 * time.Second)
		if run.DeadlineAt.Before(deadline) {
			deadline = run.DeadlineAt
		}
		stageCtx, cancel := context.WithDeadline(ctx, deadline)
		out, runErr := w.Research.Process(stageCtx, ResearchTask{RunID: run.ID, Stage: stage, Submitted: run.Submitted, Previous: previous, MaxTokens: repository.ResearchTokenReservation})
		stageErr := stageCtx.Err()
		cancel()
		if runErr != nil || stageErr != nil {
			if ctx.Err() != nil {
				return "", errors.New("research interrupted")
			}
			code := "provider_failure"
			var failure ResearchFailure
			if errors.As(runErr, &failure) {
				code = researchFailureCode(failure.Code)
			}
			if errors.Is(stageErr, context.DeadlineExceeded) {
				code = "provider_unavailable"
			}
			if err := w.store.RecordResearchFailure(ctx, run.ID, stage, job, code); err != nil {
				if errors.Is(err, repository.ErrResearchInactive) {
					return "research_stopped", nil
				}
				return "", errors.New("research checkpoint unavailable")
			}
			// Retry exhaustion is enforced by persisted stage attempts before another call.
			return "", ResearchFailure{Code: code}
		}
		if err := w.store.CommitResearchStage(ctx, run.ID, stage, job, out); err != nil {
			if errors.Is(err, repository.ErrResearchInactive) {
				return "research_stopped", nil
			}
			return "", errors.New("research checkpoint unavailable")
		}
	}
	return "research_checkpointed", nil
}
