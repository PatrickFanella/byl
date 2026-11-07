package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
)

func assemblySetup(t *testing.T) (*Worker, *repository.Store, context.Context, *models.ResearchRun) {
	t.Helper()
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "assembly", "assembly@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	minutes := 30
	b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Workers", Area: "Downers Grove", TripShape: "out_and_back", DurationMinutes: &minutes}, "")
	if err != nil {
		t.Fatal(err)
	}
	moments := []string{}
	for _, place := range []string{testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID} {
		m, err := s.CreateMoment(ctx, models.CreateMomentRequest{PlaceID: place, Title: "Workers at " + place, Body: "Workers organized at this synthetic location, according to the retained test record.", Sources: []models.Source{{Title: "Synthetic archive", Publisher: "Fixture", URL: "https://example.org/workers"}}})
		if err != nil {
			t.Fatal(err)
		}
		moments = append(moments, m.ID)
	}
	w.Assemble = true
	w.Research = researchFunc(func(ctx context.Context, task ResearchTask) (models.ResearchOutput, error) {
		out := models.ResearchOutput{Data: json.RawMessage(`{}`)}
		if task.Stage == "reconciliation" {
			out.Reconciliations = []models.ReconciliationProposal{}
			for _, id := range moments {
				p, err := s.PrepareReconciliation(ctx, task.Submitted, models.DiscoveryItem{Kind: models.CandidateKindMoment, EntityID: id, Evidence: "published_collection", Title: "Workers", Text: "Retained synthetic workers record", Area: "Downers Grove", Origins: []models.DiscoveryOrigin{}})
				if err != nil {
					return out, err
				}
				out.Reconciliations = append(out.Reconciliations, p)
			}
		} else if task.Stage != "research" {
			t.Fatal("local assembly called external processor")
		}
		return out, nil
	})
	run, err := s.StartBriefResearch(ctx, b.ID, b.Version)
	if err != nil {
		t.Fatal(err)
	}
	return w, s, ctx, run
}

func TestAssemblyResearchCheckpointsArePrivateUnpublishedAndReplayStable(t *testing.T) {
	w, s, ctx, run := assemblySetup(t)
	var toursBefore int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM tours`).Scan(&toursBefore); err != nil {
		t.Fatal(err)
	}
	drain(t, w, ctx)
	saved, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || saved.Status != "completed" || saved.RequestsUsed != 2 || saved.TokensReserved != 4000 {
		t.Fatalf("local stages spent external reservations or failed %+v %v", saved, err)
	}
	proposal, err := s.GetBriefItinerary(ctx, run.BriefID, "")
	if err != nil || proposal.Proposal == nil || proposal.Proposal.Status != "draft" || len(proposal.Proposal.Stops) != 2 || len(proposal.Proposal.TravelSequence) != 3 || proposal.Proposal.TravelMinutes != nil {
		t.Fatalf("assembled %+v %v", proposal, err)
	}
	if proposal.Proposal.Stops[0].Material.SourceURLs[0] != "https://example.org/workers" || proposal.Proposal.Stops[0].Material.RecordID == "" {
		t.Fatal("lost source/identity trail")
	}
	before, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	publicProgress, _ := json.Marshal(saved)
	if strings.Contains(string(publicProgress), "Workers organized") || strings.Contains(string(publicProgress), "example.org/workers") {
		t.Fatal("private material leaked to owner progress")
	}
	// A restarted worker reads the completed checkpoint and never creates another
	// proposal/tour or repeats source/model work for the same run.
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET status='queued',run_after=now() WHERE id=$1`, run.JobID); err != nil {
		t.Fatal(err)
	}
	w.store = repository.New(s.Pool())
	w.Research = researchFunc(func(context.Context, ResearchTask) (models.ResearchOutput, error) {
		t.Fatal("completed provider stage replayed")
		return models.ResearchOutput{}, nil
	})
	drain(t, w, ctx)
	after, err := s.GetBriefItinerary(ctx, run.BriefID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(after)
	if string(raw) != string(before) {
		t.Fatal("retained proposal changed after restart")
	}
	var toursAfter int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM tours`).Scan(&toursAfter); err != nil || toursBefore != toursAfter {
		t.Fatal("assembly materialized a tour")
	}
}

func TestAssemblyLocalStagesKeepAttemptAndCancellationFences(t *testing.T) {
	_, s, ctx, run := assemblySetup(t)
	claim, err := s.ClaimJob(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if started, err := s.BeginLocalResearchStage(ctx, run.ID, "assembly", claim); err != nil || !started {
		t.Fatal(err)
	}
	saved, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || saved.RequestsUsed != 0 || saved.TokensReserved != 0 {
		t.Fatal("local stage charged provider work")
	}
	brief, err := s.GetRouteBrief(ctx, run.BriefID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeBriefResearch(ctx, brief.ID, models.ResearchActionRequest{Version: brief.Version, RunID: run.ID}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitResearchStage(ctx, run.ID, "assembly", claim, models.ResearchOutput{Data: json.RawMessage(`{}`)}); !errors.Is(err, repository.ErrResearchInactive) {
		t.Fatalf("cancelled local work committed: %v", err)
	}
}

func TestItineraryCaptureUsesCurrentInspectedDraftAndLeavesEditsUntouched(t *testing.T) {
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	eid := inspectedDraftEvidence(t, w, ctx)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "assembly-drafts", "assembly-drafts@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Derailment", Area: "Downers Grove"}, "")
	if err != nil {
		t.Fatal(err)
	}
	place, err := s.GetPlace(ctx, testdb.SeedPlaceStationID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	item := models.DiscoveryItem{Kind: models.CandidateKindMoment, Title: "New derailment event", Text: "A sourced derailment event", URL: "https://archive.example/derailment", Area: "Downers Grove", Lat: &place.Latitude, Lng: &place.Longitude, Identity: &models.IdentityFacts{PlaceName: place.Name, Address: place.Address, CityID: place.CityID, MomentTitle: "New derailment event"}}
	record, err := s.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: b.Version, Item: item})
	if err != nil {
		t.Fatal(err)
	}
	if record.MomentCandidateID == nil {
		t.Fatalf("missing pending moment %+v", record)
	}
	id := *record.MomentCandidateID
	w.Drafter = &fakeDrafter{}
	if err := w.draftCandidate(ctx, inspectedDraftPayload(t, s, ctx, id, eid)); err != nil {
		t.Fatal(err)
	}
	c, err := s.GetCandidate(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	before := string(c.Payload)
	captured, err := s.CaptureItineraryMaterials(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Items) != 1 || captured.Items[0].CandidateID != id || captured.Items[0].EvidenceKind != "evidence_linked_draft" || captured.Items[0].EvidenceIDs[0] != eid || captured.Items[0].Narrative != "The archive records a derailment in 1940." {
		t.Fatalf("lost validated historical proposal %+v", captured)
	}
	c, err = s.GetCandidate(ctx, id)
	if err != nil || string(c.Payload) != before || c.Status != models.CandidateStatusPending {
		t.Fatal("assembly overwrote/approved curator input")
	}
	// A source URL changed inside the generated proposal cannot impersonate
	// the retained evidence snapshot, even if its historical quote still matches.
	var tampered map[string]any
	if err := json.Unmarshal(c.Payload, &tampered); err != nil {
		t.Fatal(err)
	}
	generated := tampered["generated_drafts"].([]any)[0].(map[string]any)
	generated["evidence_manifest"].([]any)[0].(map[string]any)["url"] = "https://example.org/forged-source"
	forged, _ := json.Marshal(tampered)
	if err := s.UpdateCandidatePayload(ctx, id, forged, nil); err != nil {
		t.Fatal(err)
	}
	rejected, err := s.CaptureItineraryMaterials(ctx, b.ID)
	if err != nil || len(rejected.Items) != 0 || len(rejected.Gaps) == 0 {
		t.Fatalf("forged source reference accepted %+v %v", rejected, err)
	}
	if err := s.UpdateCandidatePayload(ctx, id, json.RawMessage(before), nil); err != nil {
		t.Fatal(err)
	}
	var edited map[string]json.RawMessage
	if err := json.Unmarshal(c.Payload, &edited); err != nil {
		t.Fatal(err)
	}
	edited["title"] = json.RawMessage(`"Curator changed title"`)
	raw, _ := json.Marshal(edited)
	if err := s.UpdateCandidatePayload(ctx, id, raw, nil); err != nil {
		t.Fatal(err)
	}
	captured, err = s.CaptureItineraryMaterials(ctx, b.ID)
	if err != nil || len(captured.Items) != 0 || len(captured.Gaps) == 0 || captured.Gaps[0].Code != "candidate_needs_evidence" {
		t.Fatalf("changed input reused stale generated text %+v %v", captured, err)
	}
}

func TestNeedsResearchAssemblyResumeRetainsTheSamePartialProposal(t *testing.T) {
	w, s, ctx, run := assemblySetup(t)
	processor := w.Research
	w.Research = researchFunc(func(ctx context.Context, task ResearchTask) (models.ResearchOutput, error) {
		out, err := processor.Process(ctx, task)
		if task.Stage == "reconciliation" && len(out.Reconciliations) > 1 {
			out.Reconciliations = out.Reconciliations[:1]
		}
		return out, err
	})
	drain(t, w, ctx)
	before, err := s.GetBriefItinerary(ctx, run.BriefID, "")
	if err != nil || before.Proposal == nil || before.Proposal.Status != "needs_research" {
		t.Fatalf("missing partial %+v %v", before, err)
	}
	raw, _ := json.Marshal(before.Proposal)
	b, err := s.GetRouteBrief(ctx, run.BriefID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeBriefResearch(ctx, b.ID, models.ResearchActionRequest{Version: b.Version, RunID: run.ID}, true); err != nil {
		t.Fatal(err)
	}
	originalClock := w.now
	w.now = func() time.Time { return originalClock().Add(time.Minute) }
	w.Research = researchFunc(func(context.Context, ResearchTask) (models.ResearchOutput, error) {
		t.Fatal("completed research/capture replayed")
		return models.ResearchOutput{}, nil
	})
	drain(t, w, ctx)
	after, err := s.GetBriefItinerary(ctx, run.BriefID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(after.Proposal)
	if string(raw) != string(again) {
		t.Fatal("resumed partial proposal was rewritten despite identical captured material")
	}
}
