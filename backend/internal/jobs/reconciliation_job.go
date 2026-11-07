package jobs

import (
	"context"
	"encoding/json"

	"invisible-museum/backend/internal/models"
)

func (w *Worker) reconcileBrief(ctx context.Context, run *models.ResearchRun, previous json.RawMessage) (models.ResearchOutput, error) {
	var report models.DiscoveryReport
	if err := json.Unmarshal(previous, &report); err != nil {
		return models.ResearchOutput{}, ResearchFailure{Code: "invalid_output"}
	}
	if len(report.Items) > 20 {
		return models.ResearchOutput{}, ResearchFailure{Code: "invalid_output"}
	}
	out := models.ResearchOutput{Data: json.RawMessage(`{}`), Reconciliations: []models.ReconciliationProposal{}}
	for _, item := range report.Items {
		p, err := w.store.PrepareReconciliation(ctx, run.Submitted, item)
		if err != nil {
			return models.ResearchOutput{}, err
		}
		out.Reconciliations = append(out.Reconciliations, p)
	}
	if len(report.Items) == 0 {
		out.NeedsResearch = "insufficient_evidence"
	}
	return out, nil
}
