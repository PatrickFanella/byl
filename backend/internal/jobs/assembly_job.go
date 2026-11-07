package jobs

import (
	"context"
	"encoding/json"

	"invisible-museum/backend/internal/itinerary"
	"invisible-museum/backend/internal/models"
)

func (w *Worker) assembleBrief(ctx context.Context, run *models.ResearchRun, stage string, previous json.RawMessage) (models.ResearchOutput, error) {
	out := models.ResearchOutput{}
	if stage == "drafting" {
		material, err := w.store.CaptureItineraryMaterials(ctx, run.BriefID)
		if err != nil {
			return out, err
		}
		material.CapturedAt = w.now()
		out.Data, err = json.Marshal(material)
		return out, err
	}
	var material models.ItineraryMaterials
	if json.Unmarshal(previous, &material) != nil || material.Version != 1 || len(material.Items) > 20 {
		return out, ResearchFailure{Code: "invalid_output"}
	}
	captured := material.CapturedAt
	if captured.IsZero() {
		captured = run.CreatedAt
	}
	proposal := itinerary.Assemble(run.Submitted, material, captured)
	raw, err := json.Marshal(proposal)
	if err != nil {
		return out, err
	}
	if len(raw) > 262144 {
		return out, ResearchFailure{Code: "invalid_output"}
	}
	out.Data = raw
	if proposal.Status == "needs_research" {
		out.NeedsResearch = "insufficient_evidence"
	}
	return out, nil
}
