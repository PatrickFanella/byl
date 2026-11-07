package repository

import (
	"errors"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func arrivalFixture() models.ArrivalProposalRequest {
	lat, lng := 41.805, -88.008
	return models.ArrivalProposalRequest{Point: models.ArrivalPoint{Name: "Downer burial entrance fixture (not field inspected)", Kind: "entrance", Latitude: &lat, Longitude: &lng, Access: []models.ArrivalAccess{
		{Mode: models.TravelModeWalk, Status: "allowed", Surface: "paved", SourceURL: "https://archive.example/fixture-access", SourceTitle: "Synthetic access fixture", RetrievedAt: "2026-01-01"},
		{Mode: models.TravelModeBike, Status: "restricted", Surface: "unknown", SourceURL: "https://archive.example/fixture-access", SourceTitle: "Synthetic bike restriction", RetrievedAt: "2026-01-02"},
	}}}
}

func TestArrivalReviewRetainsSiteAndIndependentModes(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "arrivals", "arrivals@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.GetPlace(ctx, testdb.SeedPlaceStationID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if before.ArrivalPoint != nil || before.ArrivalVersion != 0 || before.SelectArrival(models.TravelModeWalk).NavigationPoint != nil {
		t.Fatal("legacy site must not imply a checked entrance")
	}
	in := arrivalFixture()
	p, err := s.ProposeArrival(ctx, before.ID, a.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	place, err := s.GetPlace(ctx, before.ID, "", false)
	if err != nil || place.ArrivalPoint != nil {
		t.Fatalf("unreviewed proposal leaked %+v %v", place, err)
	}
	if _, err := s.ReviewArrival(ctx, before.ID, p.ID, a.ID, models.ArrivalReviewRequest{Action: "approve", Note: "Fixture source context reviewed; no on-site claim"}); err != nil {
		t.Fatal(err)
	}
	place, err = s.GetPlace(ctx, before.ID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if place.Latitude != before.Latitude || place.Longitude != before.Longitude || place.UpdatedAt != before.UpdatedAt {
		t.Fatal("historical place row changed")
	}
	walk, bike := place.SelectArrival(models.TravelModeWalk), place.SelectArrival(models.TravelModeBike)
	if place.ArrivalVersion != 1 || walk.NavigationPoint == nil || walk.NavigationPoint.Lat != *in.Point.Latitude || walk.Eligibility != "allowed" || bike.Eligibility != "restricted" || bike.NavigationPoint != nil {
		t.Fatalf("independent modes %+v %+v", walk, bike)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE place_arrival_proposals SET point='{}' WHERE id=$1`, p.ID); err == nil {
		t.Fatal("reviewed point was mutable")
	}
	// Proposing an unknown replacement leaves the approved entrance visible until review.
	unknown := in
	unknown.Version = 1
	unknown.Point.Access = []models.ArrivalAccess{{Mode: models.TravelModeWalk, Status: "unknown", Surface: "unknown"}, {Mode: models.TravelModeBike, Status: "unknown", Surface: "unknown"}}
	newPoint, err := s.ProposeArrival(ctx, before.ID, a.ID, unknown)
	if err != nil {
		t.Fatal(err)
	}
	place, err = s.GetPlace(ctx, before.ID, "", false)
	if err != nil || place.ArrivalPoint.ID != p.ID {
		t.Fatal("pending change replaced current point")
	}
	if _, err := s.ReviewArrival(ctx, before.ID, newPoint.ID, a.ID, models.ArrivalReviewRequest{Version: 1, Action: "approve", Note: "Access unresolved in fixture"}); err != nil {
		t.Fatal(err)
	}
	place, err = s.GetPlace(ctx, before.ID, "", false)
	if err != nil || place.ArrivalVersion != 2 || place.SelectArrival(models.TravelModeWalk).NavigationPoint != nil {
		t.Fatalf("unknown access not explicit %+v %v", place, err)
	}
	old, err := s.GetArrivalProposal(ctx, before.ID, p.ID)
	if err != nil || old.Status != "approved" || old.Point.Access[0].SourceURL != in.Point.Access[0].SourceURL {
		t.Fatalf("history lost %+v %v", old, err)
	}
	if _, err := s.ProposeArrival(ctx, before.ID, a.ID, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale edit %v", err)
	}
}

func TestArrivalConcurrentReviewsHaveOneWinner(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "arrival-race", "race@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	proposals := []*models.ArrivalProposal{}
	for range 2 {
		p, err := s.ProposeArrival(ctx, testdb.SeedPlaceStationID, a.ID, arrivalFixture())
		if err != nil {
			t.Fatal(err)
		}
		proposals = append(proposals, p)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, p := range proposals {
		go func(id string) {
			<-start
			_, err := s.ReviewArrival(ctx, testdb.SeedPlaceStationID, id, a.ID, models.ArrivalReviewRequest{Action: "approve", Note: "Fixture review"})
			results <- err
		}(p.ID)
	}
	close(start)
	wins, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("%d winners %d conflicts", wins, conflicts)
	}
	place, err := s.GetPlace(ctx, testdb.SeedPlaceStationID, "", false)
	if err != nil || place.ArrivalVersion != 1 {
		t.Fatalf("pointer %+v %v", place, err)
	}
	if _, err := s.ReviewArrival(ctx, testdb.SeedPlaceTivoliID, proposals[0].ID, a.ID, models.ArrivalReviewRequest{Action: "reject", Note: "Wrong place"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign proposal %v", err)
	}
}
