package repository

import (
	"encoding/json"
	"errors"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestApprovedArrivalFencesRoutesAndRetainsPublishedSnapshot(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	before, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	inputs, hash, err := s.TourNavigationInputs(ctx, before.ID, models.TravelModeWalk, before.RouteVersion)
	if err != nil || inputs.Stops[0].Place.ArrivalPoint != nil {
		t.Fatalf("legacy inputs %+v %v", inputs, err)
	}
	retained, err := json.Marshal(original.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	point := arrivalFixture()
	p, err := s.ProposeArrival(ctx, before.Stops[0].Place.ID, author, point)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewArrival(ctx, p.PlaceID, p.ID, author, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic input-fence fixture"}); err != nil {
		t.Fatal(err)
	}
	inputs, newHash, err := s.TourNavigationInputs(ctx, before.ID, models.TravelModeWalk, before.RouteVersion)
	if err != nil || newHash == hash || inputs.Stops[0].Place.ArrivalPoint.ID != p.ID {
		t.Fatalf("arrival/hash not captured consistently %+v %v", inputs, err)
	}
	if err := s.UpsertRouteVersioned(ctx, original.Snapshot.Routes[models.TravelModeWalk], before.RouteVersion, hash); !errors.Is(err, ErrRouteSuperseded) {
		t.Fatalf("stale arrival result committed %v", err)
	}
	if blockers, err := s.TourPublishBlockers(ctx, before.ID); err != nil || len(blockers) == 0 {
		t.Fatalf("stale routes can publish %v %v", blockers, err)
	}
	old, err := s.GetTourRevision(ctx, before.ID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(old.Snapshot)
	if err != nil || string(after) != string(retained) {
		t.Fatal("arrival review rewrote a published snapshot")
	}
}
