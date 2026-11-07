package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/sources"
)

func (w *Worker) discoverBrief(ctx context.Context, run *models.ResearchRun, job *models.Job) (models.ResearchOutput, error) {
	known, scope, err := w.store.DiscoveryCollection(ctx, run.Submitted)
	if err != nil {
		return models.ResearchOutput{}, err
	}
	retained, err := w.store.ListSourceEvidence(ctx, run.BriefID, 50, 0)
	if err != nil {
		return models.ResearchOutput{}, err
	}
	// BeginResearchStage already reserved one request and the conservative token allowance.
	first := true
	reserve := func(ctx context.Context) error {
		allowed, err := w.store.ReserveDiscoveryRequest(ctx, run.ID, job, first)
		first = false
		if err != nil {
			return err
		}
		if !allowed {
			return sources.ErrDiscoveryBudget
		}
		return nil
	}
	report, err := w.Discovery.Run(ctx, run.Submitted, scope, known, retained, reserve)
	timedOut := errors.Is(err, context.DeadlineExceeded) && errors.Is(ctx.Err(), context.DeadlineExceeded)
	if err != nil && !timedOut {
		return models.ResearchOutput{}, err
	}
	data, err := json.Marshal(report)
	if err != nil {
		return models.ResearchOutput{}, err
	}
	out := models.ResearchOutput{Data: data}
	if timedOut {
		out.NeedsResearch = "provider_unavailable"
	} else if len(report.Items) == 0 {
		out.NeedsResearch = "insufficient_evidence"
	}
	return out, nil
}
