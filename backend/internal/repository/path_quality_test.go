package repository

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestQualityCheckpointsBindResultAndRetainPrivateHistory(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	walk := models.TravelModeWalk
	route, err := s.GetTourRoute(ctx, original.TourID, walk)
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.GetPathQualityReport(ctx, original.TourID, walk, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "pass" || !report.InputCurrent || report.ReportID == "" || route.Quality.ReportID != report.ReportID || report.RouteSHA256 != route.RoutingFingerprint() {
		t.Fatalf("checkpoint %+v route%+v", report, route)
	}
	public, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "persistence-fixture") || strings.Contains(string(public), "Synthetic persistence checkpoint") || !strings.Contains(string(public), report.ReportID) {
		t.Fatal("public summary leaked private inspection data or lost checkpoint identity")
	}
	// A different result for the same source inputs cannot borrow the old stamp.
	changed := *route
	changed.DistanceM++
	if err := s.UpsertRouteQualified(ctx, changed, original.Snapshot.Tour.RouteVersion, report.InputHash, *report); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("quality was detached from geometry/metrics: %v", err)
	}
	// Plain writers, including earlier APIs, invalidate current quality.
	if err := s.UpsertRoute(ctx, *route); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPathQualityReport(ctx, route.TourID, walk, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("plain write retained passing quality %v", err)
	}
	retained, err := s.GetPathQualityReport(ctx, route.TourID, walk, report.ReportID)
	if err != nil || retained.InputCurrent {
		t.Fatalf("old checkpoint lost or still current %+v %v", retained, err)
	}
	if _, err := s.PublishTourRevision(ctx, route.TourID, author, models.PublishRevisionRequest{DraftVersion: original.DraftVersion, CurrentRevisionID: &original.ID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing quality published: %v", err)
	}
	// The database itself rejects publication by an older application.
	if _, err := s.pool.Exec(ctx, `INSERT INTO tour_revisions(tour_id,number,draft_version,snapshot) VALUES($1,999,$2,$3)`, route.TourID, original.DraftVersion, public); err == nil {
		t.Fatal("direct revision insert bypassed the quality gate")
	}
	if _, err := s.pool.Exec(ctx, `UPDATE tour_path_quality_reports SET report='{}' WHERE id=$1`, report.ReportID); err == nil {
		t.Fatal("quality history was mutable")
	}
	old, err := s.GetTourRevision(ctx, route.TourID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(old)
	if err != nil || string(after) != string(public) {
		t.Fatal("working recomputation changed the published snapshot")
	}
}

func TestQualityResultIsFencedByChangedArrivalAndReportsStayReadable(t *testing.T) {
	s, ctx, author, original := revisionFixture(t)
	route, err := s.GetTourRoute(ctx, original.TourID, models.TravelModeWalk)
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.GetPathQualityReport(ctx, original.TourID, route.Mode, "")
	if err != nil {
		t.Fatal(err)
	}
	working, err := s.GetTour(ctx, route.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.ProposeArrival(ctx, testdb.SeedPlaceStationID, author, arrivalFixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewArrival(ctx, p.PlaceID, p.ID, author, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic access input change"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRouteQualified(ctx, *route, working.RouteVersion, report.InputHash, *report); !errors.Is(err, ErrRouteSuperseded) {
		t.Fatalf("stale quality committed %v", err)
	}
	current, err := s.GetPathQualityReport(ctx, route.TourID, route.Mode, "")
	if err != nil || current.InputCurrent {
		t.Fatalf("stale checkpoint presented as current %+v %v", current, err)
	}
	if err := s.DeleteTour(ctx, route.TourID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPathQualityReport(ctx, route.TourID, route.Mode, report.ReportID); err != nil {
		t.Fatalf("retained decision history was erased: %v", err)
	}
}
