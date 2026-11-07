package repository

import (
	"context"
	"errors"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func revisionFixture(t *testing.T) (*Store, context.Context, string, *models.TourRevision) {
	t.Helper()
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "revision-curator", "revision@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	readyFixtureRoutes(t, s, ctx, testdb.SeedTourID)
	tour, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.PublishTourRevision(ctx, tour.ID, a.ID, models.PublishRevisionRequest{DraftVersion: tour.DraftVersion, CurrentRevisionID: tour.RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	return s, ctx, a.ID, r
}

func readyFixtureRoutes(t *testing.T, s *Store, ctx context.Context, tour string) {
	t.Helper()
	for _, mode := range models.AllTravelModes {
		if err := s.UpsertRoute(ctx, models.TourRoute{TourID: tour, Mode: mode, Status: models.RouteStatusReady, Provider: "fixture", DistanceM: 1000, DurationS: 120, Geometry: &models.LineString{Type: "LineString", Coordinates: [][2]float64{{-88.01, 41.80}, {-88.02, 41.81}}}, Legs: []models.RouteLeg{}}); err != nil {
			t.Fatal(err)
		}
	}
}

func reversedRevisionStops(r *models.TourRevision) []models.StopInput {
	stops := []models.StopInput{}
	for i := len(r.Snapshot.Tour.Stops) - 1; i >= 0; i-- {
		p := r.Snapshot.Tour.Stops[i]
		stops = append(stops, models.StopInput{ID: p.ID, PlaceID: p.Place.ID, FeaturedMomentID: p.FeaturedMomentID, Note: p.Note})
	}
	return stops
}

func TestTourRevisionFreezesContentAndGeometryAcrossDraftEdits(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	body := original.Snapshot.Moments[testdb.SeedMomentBurlingtonID].Body
	changed := "A curator changed the current reviewed text."
	if _, err := s.UpdateMoment(ctx, testdb.SeedMomentBurlingtonID, models.UpdateMomentRequest{Body: &changed}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceStopsVersioned(ctx, original.TourID, reversedRevisionStops(original), &original.DraftVersion); err != nil {
		t.Fatal(err)
	}
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	if working.Stops[0].ID != original.Snapshot.Tour.Stops[3].ID || *working.RevisionID != original.ID || working.DraftVersion <= original.DraftVersion {
		t.Fatalf("draft %+v", working)
	}
	served, err := s.ServedTour(ctx, working, "")
	if err != nil || served.Stops[0].ID != original.Snapshot.Tour.Stops[0].ID {
		t.Fatalf("draft replaced published view %+v %v", served, err)
	}
	old, err := s.GetTourRevision(ctx, original.TourID, original.ID)
	if err != nil || old.Snapshot.Moments[testdb.SeedMomentBurlingtonID].Body != body || old.Snapshot.Routes[models.TravelModeWalk].Geometry.Coordinates[0][0] != -88.01 {
		t.Fatalf("snapshot changed %+v %v", old, err)
	}
	for _, sql := range []string{`UPDATE tour_revisions SET snapshot='{}' WHERE id=$1`, `DELETE FROM tour_revisions WHERE id=$1`} {
		if _, err := s.pool.Exec(ctx, sql, original.ID); err == nil {
			t.Fatal("published revision was mutable")
		}
	}
	if _, err := s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: original.DraftVersion, CurrentRevisionID: &original.ID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale publish %v", err)
	}
	readyFixtureRoutes(t, s, ctx, working.ID)
	next, err := s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &original.ID})
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == original.ID || next.Snapshot.Moments[testdb.SeedMomentBurlingtonID].Body != changed || next.Snapshot.Tour.Stops[0].ID != working.Stops[0].ID {
		t.Fatalf("new revision %+v", next)
	}
	if _, err := s.GetTourRevision(ctx, testdb.SeedPlaceStationID, original.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign revision %v", err)
	}
}

func TestRevisionProgressAndLegacyPinDoNotFollowReorderedPositions(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	p, err := s.PutProgress(ctx, models.TourProgress{AccountID: author, TourID: original.TourID, Mode: models.TravelModeWalk, VisitedPositions: []int{1}}, false)
	if err != nil || p.RevisionID != original.ID {
		t.Fatalf("legacy pin %+v %v", p, err)
	}
	if err := s.ReplaceStops(ctx, original.TourID, reversedRevisionStops(original)); err != nil {
		t.Fatal(err)
	}
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	readyFixtureRoutes(t, s, ctx, working.ID)
	next, err := s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &original.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRevisionProgress(ctx, author, next.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("new revision inherited progress %v", err)
	}
	working, err = s.GetTour(ctx, working.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	served, err := s.ServedTour(ctx, working, author)
	if err != nil || *served.RevisionID != original.ID || served.Stops[0].ID != original.Snapshot.Tour.Stops[0].ID {
		t.Fatalf("active legacy view %+v %v", served, err)
	}
	old, err := s.GetRevisionProgress(ctx, author, original.ID)
	if err != nil || len(old.VisitedStopIDs) != 1 || old.VisitedStopIDs[0] != original.Snapshot.Tour.Stops[0].ID {
		t.Fatalf("stable visits %+v %v", old, err)
	}
	reset, err := s.PutProgress(ctx, models.TourProgress{AccountID: author, TourID: original.TourID, Mode: models.TravelModeWalk}, true)
	if err != nil || reset.RevisionID != next.ID || len(reset.VisitedPositions) != 0 {
		t.Fatalf("reset %+v %v", reset, err)
	}
	if _, err := s.PutRevisionProgress(ctx, models.RevisionProgress{AccountID: author, TourID: next.TourID, RevisionID: next.ID, Mode: models.TravelModeBike, VisitedStopIDs: []string{testdb.SeedPlaceStationID}}, false); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("foreign stop %v", err)
	}
	old, err = s.GetRevisionProgress(ctx, author, original.ID)
	if err != nil || len(old.VisitedStopIDs) != 1 {
		t.Fatalf("old trip lost %+v %v", old, err)
	}
}

func TestConcurrentRevisionPublicationAndEditingRejectStaleVersions(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	errorsCh := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			_, err := s.PublishTourRevision(ctx, original.TourID, author, models.PublishRevisionRequest{DraftVersion: original.DraftVersion, CurrentRevisionID: &original.ID})
			errorsCh <- err
		}()
	}
	close(start)
	wins, conflicts := 0, 0
	for range 2 {
		err := <-errorsCh
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("publish race %d winners %d conflicts", wins, conflicts)
	}
	start = make(chan struct{})
	for range 2 {
		go func() {
			<-start
			title := "Edited draft"
			_, err := s.UpdateTour(ctx, original.TourID, UpdateTourInput{Title: &title, ExpectedVersion: &original.DraftVersion})
			errorsCh <- err
		}()
	}
	close(start)
	wins, conflicts = 0, 0
	for range 2 {
		err := <-errorsCh
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("edit race %d winners %d conflicts", wins, conflicts)
	}
	listed, _, err := s.ListTours(ctx, TourFilter{PublicOnly: true})
	if err != nil || len(listed) != 1 || listed[0].Title == "Edited draft" {
		t.Fatalf("draft discovery %+v %v", listed, err)
	}
}

func TestRouteVersionRejectsOldResultsAndRecomputeChangesIdentity(t *testing.T) {
	s, ctx, _, original := revisionFixture(t)
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.TourRouteInputs(ctx, working.ID, models.TravelModeWalk, working.RouteVersion); err != nil {
		t.Fatal(err)
	}
	if err := s.RecomputeRoutes(ctx, working.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.TourRouteInputs(ctx, working.ID, models.TravelModeWalk, working.RouteVersion); !errors.Is(err, ErrRouteSuperseded) {
		t.Fatalf("stale input read %v", err)
	}
	if err := s.UpsertRouteVersioned(ctx, models.TourRoute{TourID: working.ID, Mode: models.TravelModeWalk, Status: models.RouteStatusReady}, working.RouteVersion); !errors.Is(err, ErrRouteSuperseded) {
		t.Fatalf("stale geometry write %v", err)
	}
	route, err := s.GetTourRoute(ctx, working.ID, models.TravelModeWalk)
	if err != nil || route.Status != models.RouteStatusPending {
		t.Fatalf("stale result replaced pending route %+v %v", route, err)
	}
	r, err := s.GetTourRevision(ctx, working.ID, original.ID)
	if err != nil || r.Snapshot.Routes[models.TravelModeWalk].Status != models.RouteStatusReady {
		t.Fatal("recompute changed published geometry")
	}
}

func TestRouteDigestFencesIndependentPlaceCoordinateEdits(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	_, digest, err := s.TourRouteInputs(ctx, working.ID, models.TravelModeWalk, working.RouteVersion)
	if err != nil {
		t.Fatal(err)
	}
	latitude := 41.809
	if _, err := s.UpdatePlace(ctx, testdb.SeedPlaceStationID, models.UpdatePlaceRequest{Latitude: &latitude}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRouteVersioned(ctx, models.TourRoute{TourID: working.ID, Mode: models.TravelModeWalk, Status: models.RouteStatusReady}, working.RouteVersion, digest); !errors.Is(err, ErrRouteSuperseded) {
		t.Fatalf("stale coordinate geometry %v", err)
	}
	if _, err := s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &original.ID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("published mismatched coordinates/geometry %v", err)
	}
}

func TestModeExpansionReusesReadyUnchangedRoute(t *testing.T) {
	s, ctx, _, r := revisionFixture(t)
	if _, err := s.UpdateTour(ctx, r.TourID, UpdateTourInput{Modes: []models.TravelMode{models.TravelModeWalk}}); err != nil {
		t.Fatal(err)
	}
	before := countJobs(t, s, ctx, models.JobKindComputeRoute)
	if _, err := s.UpdateTour(ctx, r.TourID, UpdateTourInput{Modes: models.AllTravelModes}); err != nil {
		t.Fatal(err)
	}
	if countJobs(t, s, ctx, models.JobKindComputeRoute)-before != 1 {
		t.Fatal("ready unchanged walk geometry was unnecessarily recomputed")
	}
	walk, err := s.GetTourRoute(ctx, r.TourID, models.TravelModeWalk)
	if err != nil || walk.Status != models.RouteStatusReady {
		t.Fatalf("reused walk %+v %v", walk, err)
	}
}
