package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/testdb"
)

func TestLocalReconciliationCheckpointsBeforeMissingDrafter(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	w.Reconcile = true
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		if task.Stage != "research" {
			return models.ResearchOutput{}, ResearchFailure{Code: "provider_unavailable"}
		}
		raw, err := json.Marshal(models.DiscoveryReport{Items: []models.DiscoveryItem{{Kind: models.CandidateKindPlace, EntityID: testdb.SeedPlaceStationID, Title: "Main Street Station"}}})
		return models.ResearchOutput{Data: raw}, err
	})
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || r.Stages[1].Status != "completed" {
		t.Fatalf("checkpoint %+v %v", r, err)
	}
	var report models.ReconciliationReport
	if err := json.Unmarshal(r.Stages[1].Output, &report); err != nil || len(report.Records) != 1 || report.Records[0].Assessment.Outcome != "existing_place" {
		t.Fatalf("report %+v %v", report, err)
	}
	w.Research = nil
	drain(t, w, ctx)
	r, err = s.GetResearchRun(ctx, run.ID)
	if err != nil || r.Reason != "processor_unavailable" || r.Stages[1].Status != "completed" {
		t.Fatalf("missing draft stage %+v %v", r, err)
	}
	// The owner progress representation excludes both source payloads and identities.
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var progress map[string]any
	if err := json.Unmarshal(raw, &progress); err != nil {
		t.Fatal(err)
	}
	stage := progress["stages"].([]any)[1].(map[string]any)
	if stage["output"] != nil {
		t.Fatal("private checkpoint leaked")
	}
}

func TestReconciliationCommitIsClaimFenced(t *testing.T) {
	_, s, ctx, run := researchSetup(t)
	job, err := s.ClaimJob(ctx, run.CreatedAt.AddDate(0, 0, 1))
	if err != nil || job == nil {
		t.Fatalf("claim %v", err)
	}
	if ok, err := s.BeginResearchStage(ctx, run.ID, "reconciliation", job); err != nil || !ok {
		t.Fatalf("begin %v %v", ok, err)
	}
	item := models.DiscoveryItem{Kind: models.CandidateKindPlace, EntityID: testdb.SeedPlaceStationID, Title: "Main Street Station"}
	p, err := s.PrepareReconciliation(ctx, run.Submitted, item)
	if err != nil {
		t.Fatal(err)
	}
	stale := *job
	stale.Attempts--
	err = s.CommitResearchStage(ctx, run.ID, "reconciliation", &stale, models.ResearchOutput{Reconciliations: []models.ReconciliationProposal{p}})
	if !errors.Is(err, repository.ErrResearchInactive) {
		t.Fatalf("stale claim %v", err)
	}
	var count int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM reconciliation_records`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale writes %d %v", count, err)
	}
}
