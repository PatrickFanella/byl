package jobs

import (
	"encoding/json"
	"errors"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
)

func TestStaleRouteFailureCannotReplaceNewInputState(t *testing.T) {
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	_, digest, err := s.TourRouteInputs(ctx, testdb.SeedTourID, models.TravelModeWalk, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload := models.ComputeRoutePayload{TourID: testdb.SeedTourID, Mode: models.TravelModeWalk, RouteVersion: 0}
	if err := s.RecomputeRoutes(ctx, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	if err := w.computeRoute(ctx, payload); !errors.Is(err, repository.ErrRouteSuperseded) {
		t.Fatalf("stale job %v", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	job := &models.Job{Kind: models.JobKindComputeRoute, Payload: raw}
	if err := w.onFinalFailure(ctx, job, routeInputFailure{cause: errors.New("fixture old routing failure"), digest: digest}); err != nil {
		t.Fatal(err)
	}
	route, err := s.GetTourRoute(ctx, testdb.SeedTourID, models.TravelModeWalk)
	if err != nil || route.Status != models.RouteStatusPending || route.Error != "" {
		t.Fatalf("old failure replaced current state %+v %v", route, err)
	}
	latitude := 41.809
	current, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	_, digest, err = s.TourRouteInputs(ctx, current.ID, models.TravelModeWalk, current.RouteVersion)
	if err != nil {
		t.Fatal(err)
	}
	payload.RouteVersion = current.RouteVersion
	raw, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	job.Payload = raw
	if _, err := s.UpdatePlace(ctx, testdb.SeedPlaceStationID, models.UpdatePlaceRequest{Latitude: &latitude}); err != nil {
		t.Fatal(err)
	}
	if err := w.onFinalFailure(ctx, job, routeInputFailure{cause: errors.New("fixture coordinate race"), digest: digest}); err != nil {
		t.Fatal(err)
	}
	route, err = s.GetTourRoute(ctx, current.ID, models.TravelModeWalk)
	if err != nil || route.Status != models.RouteStatusPending {
		t.Fatalf("coordinate-race failure replaced state %+v %v", route, err)
	}
}
