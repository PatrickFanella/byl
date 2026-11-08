package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func reportInput(revision *models.TourRevision) models.RouteReportInput {
	return models.RouteReportInput{TourID: revision.TourID, RevisionID: revision.ID, Mode: models.TravelModeWalk, Reason: "blocked", Affected: models.ReportLocation{Kind: "entrance", StopID: revision.Snapshot.Tour.Stops[0].ID}}
}

func reportPoint(lat, lng float64) *models.ReportPoint {
	return &models.ReportPoint{Lat: &lat, Lng: &lng}
}

func TestRouteReportsRetainRevisionAndPrivateLifecycle(t *testing.T) {
	s, ctx, author, revision := revisionFixture(t)
	in := reportInput(revision)
	first, created, err := s.CreateRouteReport(ctx, author, in, "repeat")
	if err != nil || !created || first.Status != models.BriefReview || first.Submitted.Duration != "unknown" || first.Submitted.ObservedOn != nil {
		t.Fatalf("minimal report %+v %v", first, err)
	}
	other, err := s.UpsertAccount(ctx, models.AuthProviderDev, "report-other", "other@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRouteReport(ctx, first.ID, other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign read %v", err)
	}
	if list, err := s.ListRouteReports(ctx, other.ID, "", 20, 0); err != nil || len(list) != 0 {
		t.Fatalf("foreign list %+v %v", list, err)
	}
	if err := s.ReplaceStops(ctx, revision.TourID, reversedRevisionStops(revision)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateRouteReport(ctx, author, in, "old-trip"); err != nil {
		t.Fatalf("working reorder invalidated the original trip: %v", err)
	}
	accepted, err := s.SetRouteReportState(ctx, first.ID, models.BriefStateRequest{Version: 1, Status: models.BriefAccepted, Reason: "Private disposition; independently review before editing a route."})
	if err != nil || accepted.Version != 2 || accepted.Submitted.RevisionID != revision.ID {
		t.Fatalf("disposition %+v %v", accepted, err)
	}
	if _, err := s.SetRouteReportState(ctx, first.ID, models.BriefStateRequest{Version: 1, Status: models.BriefDeclined, Reason: "Stale review"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale disposition %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE route_reports SET submitted='{}' WHERE id=$1`, first.ID); err == nil {
		t.Fatal("submitted observation was mutable")
	}
	if err := s.DeleteTour(ctx, revision.TourID); err != nil {
		t.Fatal(err)
	}
	again, created, err := s.CreateRouteReport(ctx, author, in, "repeat")
	if err != nil || created || again.ID != first.ID || again.Status != models.BriefAccepted {
		t.Fatalf("retry after route withdrawal %+v %v", again, err)
	}
	if _, _, err := s.CreateRouteReport(ctx, author, in, "new-after-delete"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("new report on deleted route %v", err)
	}
	in.Details = "Changed input"
	if _, _, err := s.CreateRouteReport(ctx, author, in, "repeat"); !errors.Is(err, ErrConflict) {
		t.Fatalf("key conflict %v", err)
	}
}

func TestRouteReportsValidateFrozenTargetsAndOptionalAlternatives(t *testing.T) {
	s, ctx, author, r := revisionFixture(t)
	in := reportInput(r)
	a, b := r.Snapshot.Tour.Stops[0], r.Snapshot.Tour.Stops[1]
	in.Affected = models.ReportLocation{Kind: "segment", FromStopID: a.ID, ToStopID: b.ID}
	in.Direction = "forward"
	in.Reason, in.Better, in.Duration = "improvement", "A smoother walking surface", "permanent"
	in.Annotation = reportPoint(a.Place.Latitude, a.Place.Longitude)
	in.Alternative = &models.ReportAlternative{Type: "LineString", Coordinates: [][]float64{{a.Place.Longitude, a.Place.Latitude}, {b.Place.Longitude, b.Place.Latitude}}}
	if _, _, err := s.CreateRouteReport(ctx, author, in, "alternative"); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*models.RouteReportInput){
		"far annotation":       func(v *models.RouteReportInput) { v.Annotation = reportPoint(0, 0) },
		"wrong direction":      func(v *models.RouteReportInput) { v.Direction = "reverse" },
		"non-adjacent segment": func(v *models.RouteReportInput) { v.Affected.ToStopID = r.Snapshot.Tour.Stops[3].ID },
		"foreign stop":         func(v *models.RouteReportInput) { v.Affected.ToStopID = r.TourID },
		"disconnected alternative": func(v *models.RouteReportInput) {
			v.Alternative = &models.ReportAlternative{Type: "LineString", Coordinates: [][]float64{{0, 0}, {1, 1}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := in
			change(&bad)
			if _, _, err := s.CreateRouteReport(ctx, author, bad, name); !errors.Is(err, ErrInvalidReference) {
				t.Fatalf("invalid target %v", err)
			}
		})
	}
}

func TestRouteReportConcurrentRetriesAndQuotas(t *testing.T) {
	s, ctx, author, r := revisionFixture(t)
	in := reportInput(r)
	ch := make(chan error, 16)
	ids := make(chan string, 16)
	for range 16 {
		go func() {
			report, _, err := s.CreateRouteReport(ctx, author, in, "concurrent")
			if err == nil {
				ids <- report.ID
			}
			ch <- err
		}()
	}
	for range 16 {
		if err := <-ch; err != nil {
			t.Fatal(err)
		}
	}
	first := <-ids
	for range 15 {
		if <-ids != first {
			t.Fatal("concurrent retry created another report")
		}
	}
	for i := range 16 {
		go func() { _, _, err := s.CreateRouteReport(ctx, author, in, fmt.Sprintf("quota-%d", i)); ch <- err }()
	}
	wins, limited := 0, 0
	for range 16 {
		err := <-ch
		if err == nil {
			wins++
		} else if errors.Is(err, ErrSubmissionLimit) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 9 || limited != 7 {
		t.Fatalf("quota race %d accepted, %d limited", wins, limited)
	}
	if _, _, err := s.CreateRouteReport(ctx, author, in, "concurrent"); err != nil {
		t.Fatalf("retry at quota %v", err)
	}
	older, err := s.UpsertAccount(ctx, models.AuthProviderDev, "older-reporter", "older@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	in.Normalize()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := s.pool.Exec(ctx, `INSERT INTO route_reports(owner_account_id,tour_id,revision_id,submitted,submission_hash,created_at) VALUES($1,$2,$3,$4,'fixture',now()-interval '2 days')`, older.ID, r.TourID, r.ID, raw); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.CreateRouteReport(ctx, older.ID, in, "unresolved-limit"); !errors.Is(err, ErrSubmissionLimit) {
		t.Fatalf("unresolved quota %v", err)
	}
}

func TestRouteReportConcurrentPublicationDoesNotDeadlock(t *testing.T) {
	s, parent, author, r := revisionFixture(t)
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, _, err := s.CreateRouteReport(ctx, author, reportInput(r), "while-publishing")
		results <- err
	}()
	go func() {
		<-start
		_, err := s.PublishTourRevision(ctx, r.TourID, author, models.PublishRevisionRequest{DraftVersion: r.DraftVersion, CurrentRevisionID: &r.ID})
		results <- err
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestRouteReportUsesCapturedEntranceAfterArrivalChanges(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	in := arrivalFixture()
	lat, lng := 41.9, -88.2 // Deliberately distant synthetic anchor, not a real entrance.
	in.Point.Latitude, in.Point.Longitude = &lat, &lng
	proposal, err := s.ProposeArrival(ctx, testdb.SeedPlaceStationID, author, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewArrival(ctx, testdb.SeedPlaceStationID, proposal.ID, author, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic snapshot fixture"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishTourRevision(ctx, original.TourID, author, models.PublishRevisionRequest{DraftVersion: original.DraftVersion, CurrentRevisionID: &original.ID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed entrance must invalidate the old route cache: %v", err)
	}
	// This test isolates frozen report targeting using the repository's explicit
	// ready-route fixture; controlled provider behavior has separate worker tests.
	readyFixtureRoutes(t, s, ctx, original.TourID)
	r, err := s.PublishTourRevision(ctx, original.TourID, author, models.PublishRevisionRequest{DraftVersion: original.DraftVersion, CurrentRevisionID: &original.ID})
	if err != nil {
		t.Fatal(err)
	}
	in.Version = 1
	newLat := 41.7
	in.Point.Latitude = &newLat
	newProposal, err := s.ProposeArrival(ctx, testdb.SeedPlaceStationID, author, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewArrival(ctx, testdb.SeedPlaceStationID, newProposal.ID, author, models.ArrivalReviewRequest{Version: 1, Action: "approve", Note: "Changed synthetic anchor"}); err != nil {
		t.Fatal(err)
	}
	report := reportInput(r)
	report.Affected.ArrivalID = proposal.ID
	report.Annotation = reportPoint(lat, lng)
	if _, _, err := s.CreateRouteReport(ctx, author, report, "frozen-entrance"); err != nil {
		t.Fatalf("current entrance replaced the frozen report target: %v", err)
	}
	report.Affected.ArrivalID = newProposal.ID
	if _, _, err := s.CreateRouteReport(ctx, author, report, "wrong-entrance"); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("current entrance incorrectly accepted for old revision: %v", err)
	}
}

func TestRouteReportAnnotationDistanceUsesSegmentInterior(t *testing.T) {
	path := [][2]float64{{-88, 41}, {-87.9, 41}}
	if !geo.NearPolyline([2]float64{-87.95, 41.005}, path, 1000) || geo.NearPolyline([2]float64{-87.95, 41.02}, path, 1000) {
		t.Fatal("segment interior proximity did not bound the affected area")
	}
	if !geo.NearPolyline([2]float64{180, 0.005}, [][2]float64{{179.9, 0}, {-179.9, 0}}, 1000) {
		t.Fatal("longitude wrapping broke a local annotation")
	}
}

func TestRouteReportCanTargetDeclaredClosingLeg(t *testing.T) {
	stops := []models.TourStop{{ID: "first", Position: 1}, {ID: "middle", Position: 2}, {ID: "last", Position: 3}}
	leg := models.RouteLeg{FromPosition: 3, ToPosition: 1, FromStopID: "last", ToStopID: "first", Return: true, NavigationSource: "approved_navigation", Geometry: [][2]float64{{-88, 41}, {-88, 41.001}}}
	snapshot := &models.TourSnapshot{Tour: models.TourDetail{TourSummary: models.TourSummary{Modes: []models.TravelMode{models.TravelModeWalk}}, Stops: stops}}
	snapshot.Routes = map[models.TravelMode]models.TourRoute{models.TravelModeWalk: {Status: models.RouteStatusReady, Legs: []models.RouteLeg{leg}}}
	revision := &models.TourRevision{Snapshot: snapshot}
	in := models.RouteReportInput{Mode: models.TravelModeWalk, Direction: "forward", Affected: models.ReportLocation{Kind: "segment", FromStopID: "last", ToStopID: "first"}, Annotation: reportPoint(41.0005, -88)}
	if err := validateReportLocation(revision, in); err != nil {
		t.Fatalf("closing leg cannot be reported: %v", err)
	}
	in.Direction = "reverse"
	if err := validateReportLocation(revision, in); err == nil {
		t.Fatal("closing leg direction was ignored")
	}
	in.Affected.FromStopID, in.Affected.ToStopID = "first", "last"
	if err := validateReportLocation(revision, in); err != nil {
		t.Fatalf("reverse closing leg: %v", err)
	}
	// Two-story loops have a forward leg and a closing leg between the same
	// pair. Unknown direction should prefer the submitted endpoint order.
	revision.Snapshot.Tour.Stops = []models.TourStop{stops[0], {ID: "last", Position: 2}}
	leg.FromPosition = 2
	revision.Snapshot.Routes[models.TravelModeWalk] = models.TourRoute{Status: models.RouteStatusReady, Legs: []models.RouteLeg{
		{FromPosition: 1, ToPosition: 2, Geometry: [][2]float64{{0, 0}, {0, 0.001}}}, leg,
	}}
	in.Direction, in.Affected.FromStopID, in.Affected.ToStopID = "unknown", "last", "first"
	if err := validateReportLocation(revision, in); err != nil {
		t.Fatalf("two-story return selected the reversed outgoing leg: %v", err)
	}
}

func TestRouteReportQueuePrioritizesClosuresAndKeepsOwnerChronology(t *testing.T) {
	s, ctx, author, revision := revisionFixture(t)
	closed := reportInput(revision)
	closed.Reason = "closed"
	first, _, err := s.CreateRouteReport(ctx, author, closed, "closure")
	if err != nil {
		t.Fatal(err)
	}
	improvement := reportInput(revision)
	improvement.Reason = "improvement"
	improvement.Better = "Synthetic alternative surface"
	second, _, err := s.CreateRouteReport(ctx, author, improvement, "improvement")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := s.ListRouteReports(ctx, "", models.BriefReview, 20, 0)
	if err != nil || len(admin) != 2 || admin[0].ID != first.ID {
		t.Fatalf("closure priority %+v %v", admin, err)
	}
	owner, err := s.ListRouteReports(ctx, author, "", 20, 0)
	if err != nil || len(owner) != 2 || owner[0].ID != second.ID {
		t.Fatalf("owner chronology %+v %v", owner, err)
	}
	if _, err = s.SetRouteReportState(ctx, first.ID, models.BriefStateRequest{Version: 1, Status: models.BriefDeclined, Reason: "Already assessed; not a current closure"}); err != nil {
		t.Fatal(err)
	}
	admin, err = s.ListRouteReports(ctx, "", "", 20, 0)
	if err != nil || len(admin) != 2 || admin[0].ID != second.ID {
		t.Fatal("completed closure hid an unresolved report")
	}
}
