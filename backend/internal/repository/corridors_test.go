package repository

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
)

func corridorInput(t *testing.T, tour *models.TourDetail) *models.CorridorIntent {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "fixtures", "corridors-v1", "intent.json"))
	if err != nil {
		t.Fatal(err)
	}
	var in models.CorridorIntent
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	in.Routes[0].ShapingPoints[0].AfterStopID = tour.Stops[0].ID
	in.Routes[0].ShapingPoints[0].BeforeStopID = tour.Stops[1].ID
	return &in
}

func TestCorridorReviewPublicationAndRecomputeRetainChoices(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	input := corridorInput(t, &original.Snapshot.Tour)
	p, err := s.ProposeCorridor(ctx, original.TourID, author, models.CorridorProposalRequest{Intent: input})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.ProposeCorridor(ctx, original.TourID, author, models.CorridorProposalRequest{Intent: input})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.GetTour(ctx, original.TourID, false)
	if err != nil || before.Corridor != nil {
		t.Fatalf("pending import leaked: %+v %v", before, err)
	}
	_, oldHash, err := s.TourRouteInputs(ctx, original.TourID, models.TravelModeWalk, before.RouteVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCorridor(ctx, original.TourID, p.ID, author, models.ArrivalReviewRequest{Action: "approve", Note: "PRIVATE REVIEW REASON"}); err != nil {
		t.Fatal(err)
	}
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil || working.Corridor == nil || working.Corridor.Version != 1 || working.DraftVersion <= before.DraftVersion || working.RouteVersion <= before.RouteVersion || len(working.Stops) != len(before.Stops) {
		t.Fatalf("approval %+v %v", working, err)
	}
	_, hash, err := s.TourRouteInputs(ctx, working.ID, models.TravelModeWalk, working.RouteVersion)
	if err != nil || hash == oldHash {
		t.Fatal("corridor approval did not fence route inputs")
	}
	if _, err := s.ReviewCorridor(ctx, original.TourID, other.ID, author, models.ArrivalReviewRequest{Action: "approve", Note: "Stale proposal"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale review %v", err)
	}
	if err := s.UpsertRouteVersioned(ctx, original.Snapshot.Routes[models.TravelModeWalk], before.RouteVersion, oldHash); !errors.Is(err, ErrRouteSuperseded) {
		t.Fatalf("superseded route committed %v", err)
	}
	for _, query := range []string{`UPDATE tour_corridor_proposals SET intent='{}' WHERE id=$1`, `UPDATE tour_corridor_proposals SET review_note='changed' WHERE id=$1`} {
		if _, err := s.pool.Exec(ctx, query, p.ID); err == nil {
			t.Fatal("proposal or completed review was mutable")
		}
	}
	served, err := s.ServedTour(ctx, working, "")
	if err != nil || served.Corridor != nil || *served.RevisionID != original.ID {
		t.Fatalf("approval released an unpublished draft %+v %v", served, err)
	}
	readyFixtureRoutes(t, s, ctx, working.ID)
	released, err := s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &original.ID})
	if err != nil || released.Snapshot.Tour.Corridor.ID != p.ID || len(released.Snapshot.Tour.Stops) != 4 || len(released.Snapshot.Moments) != 4 {
		t.Fatalf("snapshot %+v %v", released, err)
	}
	raw, err := json.Marshal(released)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE REVIEW") || strings.Contains(string(raw), "created_by") || !strings.Contains(string(raw), "elgin-shape") {
		t.Fatal("public snapshot leaked private review or lost shaping intent")
	}
	if err := s.RecomputeRoutes(ctx, working.ID); err != nil {
		t.Fatal(err)
	}
	working, err = s.GetTour(ctx, working.ID, false)
	if err != nil || working.Corridor.ID != p.ID || working.Corridor.Intent.Routes[0].Branch != "elgin" {
		t.Fatalf("recompute lost intent %+v %v", working, err)
	}
	old, err := s.GetTourRevision(ctx, working.ID, released.ID)
	if err != nil || old.Snapshot.Tour.Corridor.ID != p.ID || old.Snapshot.Routes[models.TravelModeWalk].Status != models.RouteStatusReady {
		t.Fatal("recompute changed a retained published revision")
	}
}

func TestCorridorChangedStopsNeedReviewedReplacementOrRemoval(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	p, err := s.ProposeCorridor(ctx, original.TourID, author, models.CorridorProposalRequest{Intent: corridorInput(t, &original.Snapshot.Tour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCorridor(ctx, original.TourID, p.ID, author, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic reference review"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceStops(ctx, original.TourID, reversedRevisionStops(original)); err != nil {
		t.Fatal(err)
	}
	readyFixtureRoutes(t, s, ctx, original.TourID)
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &original.ID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("dangling shaping stop references published %v", err)
	}
	removal, err := s.ProposeCorridor(ctx, working.ID, author, models.CorridorProposalRequest{Version: 1, Remove: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCorridor(ctx, working.ID, removal.ID, author, models.ArrivalReviewRequest{Version: 1, Action: "approve", Note: "Explicitly remove obsolete intent"}); err != nil {
		t.Fatal(err)
	}
	working, err = s.GetTour(ctx, working.ID, false)
	if err != nil || working.Corridor.Version != 2 || working.Corridor.Intent != nil {
		t.Fatalf("removal %+v %v", working, err)
	}
	readyFixtureRoutes(t, s, ctx, working.ID)
	if _, err := s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &original.ID}); err != nil {
		t.Fatalf("reviewed removal did not unblock publication: %v", err)
	}
}
