package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func (w *Worker) fetchEvidence(ctx context.Context, job *models.Job) (string, error) {
	var payload models.FetchEvidencePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return "", fmt.Errorf("%w: invalid evidence job payload", ErrNoRetry)
	}
	e, err := w.store.GetSourceEvidence(ctx, payload.EvidenceID)
	if err != nil {
		return "", errors.New("evidence checkpoint unavailable")
	}
	if e.Result != nil {
		return "evidence_" + e.Result.Outcome, nil
	}
	result := models.EvidenceResult{Outcome: "worker_failed"}
	if job.Attempts <= job.MaxAttempts {
		retriever := w.Evidence
		if retriever == nil && w.Sources != nil {
			retriever = w.Sources.Client()
		}
		if retriever != nil {
			result, err = retriever.RetrieveEvidence(ctx, e.URL, e.SourceKind)
			if err != nil {
				return "", errors.New("evidence retrieval interrupted")
			}
		} else {
			result.Outcome = "unavailable"
		}
	}
	if err := w.store.CompleteSourceEvidence(ctx, e.ID, job, result); err != nil {
		if errors.Is(err, repository.ErrResearchInactive) {
			return "evidence_claim_replaced", nil
		}
		return "", errors.New("evidence checkpoint unavailable")
	}
	return "evidence_" + result.Outcome, nil
}
