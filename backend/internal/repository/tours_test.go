package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func countJobs(t *testing.T, s *Store, ctx context.Context, kind string) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind = $1 AND status = 'queued'`, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestGetSeedTourResolvesFeaturedMoments(t *testing.T) {
	s, ctx := newStore(t)
	tour, err := s.GetTour(ctx, "downtown-memory-loop")
	if err != nil {
		t.Fatal(err)
	}
	if tour.StopCount != 4 || len(tour.Stops) != 4 || tour.Topic == nil || tour.Topic.Slug != "rail-and-the-commuter-town" {
		t.Fatalf("tour: %+v", tour.TourSummary)
	}
	if tour.Stops[0].FeaturedMoment == nil || tour.Stops[0].FeaturedMoment.ID != testdb.SeedMomentBurlingtonID {
		t.Fatalf("stop 1 featured: %+v", tour.Stops[0])
	}
	fishel := tour.Stops[2]
	if fishel.FeaturedMomentID != nil || fishel.FeaturedMoment == nil || fishel.FeaturedMoment.ID != testdb.SeedMomentConcertID {
		t.Fatalf("stop 3 should fall back to the place's primary moment: %+v", fishel)
	}
	if len(tour.Modes) != 2 || tour.Routes[models.TravelModeWalk].Status != models.RouteStatusPending {
		t.Fatalf("routes before worker: %+v modes %v", tour.Routes, tour.Modes)
	}
	if tour.Routes[models.TravelModeWalk].EstimatedMinutes != nil {
		t.Fatal("pending route must not carry an estimate")
	}
}

func TestCreateTourQueuesRouteJobsPerMode(t *testing.T) {
	s, ctx := newStore(t)
	before := countJobs(t, s, ctx, models.JobKindComputeRoute)
	tour, err := s.CreateTour(ctx, CreateTourInput{
		CityID: testdb.SeedCityID, Title: "Three Stops", Modes: []models.TravelMode{models.TravelModeWalk, models.TravelModeBike},
		Visibility: models.TourVisibilityPublic,
		Stops:      []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: testdb.SeedPlaceTivoliID}, {PlaceID: testdb.SeedPlaceFishelID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tour.Slug != "three-stops" || tour.Status != models.EntityStatusDraft || len(tour.Stops) != 3 {
		t.Fatalf("created: %+v", tour.TourSummary)
	}
	if got := countJobs(t, s, ctx, models.JobKindComputeRoute) - before; got != 2 {
		t.Fatalf("expected 2 route jobs, got %d", got)
	}
	if len(tour.Routes) != 2 || tour.Routes[models.TravelModeBike].Status != models.RouteStatusPending {
		t.Fatalf("routes: %+v", tour.Routes)
	}
}

func TestReplaceStopsValidatesAndRequeues(t *testing.T) {
	s, ctx := newStore(t)
	if err := s.UpsertRoute(ctx, models.TourRoute{TourID: testdb.SeedTourID, Mode: models.TravelModeWalk, Status: models.RouteStatusReady, Provider: "test", DistanceM: 1000, DurationS: 800,
		Geometry: &models.LineString{Type: "LineString", Coordinates: [][2]float64{{-88.0092, 41.7959}, {-88.0099, 41.7952}}}}); err != nil {
		t.Fatal(err)
	}
	tour, _ := s.GetTour(ctx, testdb.SeedTourID)
	if tour.Routes[models.TravelModeWalk].Status != models.RouteStatusReady || *tour.Routes[models.TravelModeWalk].EstimatedMinutes != 14 {
		t.Fatalf("route after upsert: %+v", tour.Routes[models.TravelModeWalk])
	}
	before := countJobs(t, s, ctx, models.JobKindComputeRoute)
	err := s.ReplaceStops(ctx, testdb.SeedTourID, []models.StopInput{{PlaceID: testdb.SeedPlaceMuseumID}, {PlaceID: testdb.SeedPlaceStationID, FeaturedMomentID: ptr(testdb.SeedMomentPlatformID)}})
	if err != nil {
		t.Fatal(err)
	}
	tour, _ = s.GetTour(ctx, testdb.SeedTourID)
	if len(tour.Stops) != 2 || tour.Stops[1].FeaturedMoment.ID != testdb.SeedMomentPlatformID {
		t.Fatalf("stops: %+v", tour.Stops)
	}
	if tour.Routes[models.TravelModeWalk].Status != models.RouteStatusPending {
		t.Fatal("replacing stops must mark routes pending")
	}
	if got := countJobs(t, s, ctx, models.JobKindComputeRoute) - before; got != 2 {
		t.Fatalf("expected 2 new route jobs, got %d", got)
	}

	// Featured moment from a different place is rejected.
	err = s.ReplaceStops(ctx, testdb.SeedTourID, []models.StopInput{{PlaceID: testdb.SeedPlaceMuseumID, FeaturedMomentID: ptr(testdb.SeedMomentTalkiesID)}})
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("expected ErrInvalidReference, got %v", err)
	}
	// Unknown place is rejected.
	err = s.ReplaceStops(ctx, testdb.SeedTourID, []models.StopInput{{PlaceID: "00000000-0000-4000-8000-00000000ffff"}})
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("expected ErrInvalidReference for unknown place, got %v", err)
	}
}

func TestTourPublishBlockers(t *testing.T) {
	s, ctx := newStore(t)
	blockers, err := s.TourPublishBlockers(ctx, testdb.SeedTourID)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(blockers, "; ")
	if len(blockers) != 2 || !strings.Contains(joined, "walk") || !strings.Contains(joined, "bike") {
		t.Fatalf("expected pending walk and bike blockers, got %v", blockers)
	}
	for _, m := range models.AllTravelModes {
		if err := s.UpsertRoute(ctx, models.TourRoute{TourID: testdb.SeedTourID, Mode: m, Status: models.RouteStatusReady, DistanceM: 1, DurationS: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetPlaceStatus(ctx, testdb.SeedPlaceFishelID, models.EntityStatusDraft); err != nil {
		t.Fatal(err)
	}
	blockers, _ = s.TourPublishBlockers(ctx, testdb.SeedTourID)
	if len(blockers) != 1 || !strings.Contains(blockers[0], "fishel-park") {
		t.Fatalf("expected an unpublished-place blocker, got %v", blockers)
	}
}

func TestListToursVisibilityAndOwner(t *testing.T) {
	s, ctx := newStore(t)
	owner, _ := s.UpsertAccount(ctx, models.AuthProviderDev, "u1", "u1@example.com", "U One", false)
	_, err := s.CreateTour(ctx, CreateTourInput{CityID: testdb.SeedCityID, Title: "Mine", Modes: models.AllTravelModes, Status: models.EntityStatusPublished,
		Visibility: models.TourVisibilityUnlisted, OwnerAccountID: &owner.ID, RandomSlug: true,
		Stops: []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: testdb.SeedPlaceTivoliID}}})
	if err != nil {
		t.Fatal(err)
	}
	public, _, err := s.ListTours(ctx, TourFilter{CityID: testdb.SeedCityID, PublicOnly: true})
	if err != nil || len(public) != 1 || public[0].Slug != "downtown-memory-loop" {
		t.Fatalf("public list: %v %+v", err, public)
	}
	mine, _, err := s.ListTours(ctx, TourFilter{OwnerID: owner.ID})
	if err != nil || len(mine) != 1 || mine[0].Title != "Mine" || len(mine[0].Slug) != 10 {
		t.Fatalf("owner list: %v %+v", err, mine)
	}
	byTopic, _, _ := s.ListTours(ctx, TourFilter{CityID: testdb.SeedCityID, TopicID: testdb.SeedTopicParksID, PublicOnly: true})
	if len(byTopic) != 0 {
		t.Fatalf("topic filter should exclude the rail tour, got %d", len(byTopic))
	}
	if err := s.DeleteTour(ctx, mine[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTour(ctx, mine[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestUpdateTourModesAdjustsRoutes(t *testing.T) {
	s, ctx := newStore(t)
	tour, err := s.UpdateTour(ctx, testdb.SeedTourID, UpdateTourInput{Modes: []models.TravelMode{models.TravelModeWalk}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tour.Modes) != 1 || len(tour.Routes) != 1 {
		t.Fatalf("modes/routes after narrowing: %v %+v", tour.Modes, tour.Routes)
	}
	before := countJobs(t, s, ctx, models.JobKindComputeRoute)
	tour, err = s.UpdateTour(ctx, testdb.SeedTourID, UpdateTourInput{Modes: models.AllTravelModes})
	if err != nil || len(tour.Routes) != 2 {
		t.Fatalf("widening: %v %+v", err, tour.Routes)
	}
	if countJobs(t, s, ctx, models.JobKindComputeRoute)-before != 1 {
		t.Fatal("widening should enqueue a job for the new mode only")
	}
	if _, err := s.GetTourRoute(ctx, testdb.SeedTourID, models.TravelModeBike); err != nil {
		t.Fatalf("route row for bike should exist pending: %v", err)
	}
	_ = time.Now
}

func ptr(s string) *string { return &s }
