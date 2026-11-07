package jobs

import (
	"context"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
)

type controlledJobRouter struct {
	routing.Router
	calls []models.TravelMode
}

func (r *controlledJobRouter) RouteControlled(ctx context.Context, mode models.TravelMode, request routing.ControlledRequest) (routing.Route, error) {
	r.calls = append(r.calls, mode)
	var stops []routing.Point
	for _, point := range request.Controls {
		if point.Break {
			stops = append(stops, point.Point)
		}
	}
	result, err := r.Route(ctx, mode, stops)
	result.Provider = "controlled-job-stub"
	return result, err
}

func approveJobArrivals(t *testing.T, s *repository.Store, ctx context.Context, bikeStatus string) {
	t.Helper()
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "controlled-route-reviewer", "route-reviewer@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	tour, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, stop := range tour.Stops {
		lat, lng := stop.Place.Latitude+0.0001, stop.Place.Longitude+0.0001
		in := models.ArrivalProposalRequest{Point: models.ArrivalPoint{Name: "Synthetic approved entrance", Kind: "entrance", Latitude: &lat, Longitude: &lng}}
		for _, mode := range models.AllTravelModes {
			status := "allowed"
			if mode == models.TravelModeBike {
				status = bikeStatus
			}
			in.Point.Access = append(in.Point.Access, models.ArrivalAccess{Mode: mode, Status: status, Surface: "unknown", SourceURL: "https://example.org/route-fixture", SourceTitle: "Synthetic access", RetrievedAt: "2026-11-07"})
		}
		proposal, err := s.ProposeArrival(ctx, stop.Place.ID, a.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReviewArrival(ctx, stop.Place.ID, proposal.ID, a.ID, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic access review"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestControlledRouteWorkerSeparatesModeFailures(t *testing.T) {
	router := &controlledJobRouter{Router: routing.NewStraightLine()}
	w, s, ctx := newWorker(t, router, narration.None{})
	drain(t, w, ctx)
	approveJobArrivals(t, s, ctx, "restricted")
	if err := s.RecomputeRoutes(ctx, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	if n := drain(t, w, ctx); n != 2 {
		t.Fatalf("expected two independent attempts, got%d", n)
	}
	walk, err := s.GetTourRoute(ctx, testdb.SeedTourID, models.TravelModeWalk)
	if err != nil || walk.Status != models.RouteStatusReady || len(walk.Legs) != 3 || walk.Legs[0].FromStopID == "" || walk.Legs[0].NavigationSource != "approved_navigation" {
		t.Fatalf("walk %+v %v", walk, err)
	}
	bike, err := s.GetTourRoute(ctx, testdb.SeedTourID, models.TravelModeBike)
	if err != nil || bike.Status != models.RouteStatusFailed || !strings.Contains(bike.Error, "arrival_restricted") {
		t.Fatalf("bike %+v %v", bike, err)
	}
	if len(router.calls) != 1 || router.calls[0] != models.TravelModeWalk {
		t.Fatalf("provider received restricted mode %v", router.calls)
	}
}

func TestControlledRouteWorkerRejectsDevelopmentProvider(t *testing.T) {
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	approveJobArrivals(t, s, ctx, "allowed")
	if err := s.RecomputeRoutes(ctx, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	if n := drain(t, w, ctx); n != 2 {
		t.Fatalf("deterministic failures retried %d times", n)
	}
	for _, mode := range models.AllTravelModes {
		route, err := s.GetTourRoute(ctx, testdb.SeedTourID, mode)
		if err != nil || route.Status != models.RouteStatusFailed || !strings.Contains(route.Error, "provider_does_not_support_approved_navigation") || route.Geometry != nil {
			t.Fatalf("development fallback %+v %v", route, err)
		}
	}
}

func TestControlledSuccessRequiresExplicitPublicationAndPinsCoordinates(t *testing.T) {
	w, s, ctx := newWorker(t, &controlledJobRouter{Router: routing.NewStraightLine()}, narration.None{})
	drain(t, w, ctx)
	before, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	approveJobArrivals(t, s, ctx, "allowed")
	if err := s.RecomputeRoutes(ctx, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	// Even durable legacy release intent must not convert provider HTTP success
	// into approval of the new navigation inputs.
	pool := testdbPool(t, s)
	if _, err := pool.Exec(ctx, `UPDATE tour_revision_state SET release_requested_version=draft_version WHERE tour_id=$1`, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	drain(t, w, ctx)
	after, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil || *after.RevisionID != *before.RevisionID {
		t.Fatalf("worker auto-published %+v %v", after, err)
	}
	released, err := s.PublishTourRevision(ctx, after.ID, "", models.PublishRevisionRequest{DraftVersion: after.DraftVersion, CurrentRevisionID: before.RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	intent := string(released.Snapshot.RoutingIntent)
	if !strings.Contains(intent, `"coordinate_source": "approved_navigation"`) || !strings.Contains(intent, `"site_latitude"`) {
		t.Fatalf("incorrect frozen routing inputs %s", intent)
	}
	first := released.Snapshot.Tour.Stops[0]
	point := released.Snapshot.Routes[models.TravelModeWalk].Legs[0].Geometry[0]
	if point[1] != *first.Place.ArrivalPoint.Point.Latitude || point[1] == first.Place.Latitude {
		t.Fatal("route used historical site rather than reviewed entrance")
	}
}
