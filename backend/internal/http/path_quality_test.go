package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

// HTTP history/authorization tests inject synthetic quality checkpoints as a
// precondition. Real assessment, source resolution and provider identity checks
// are exercised independently through the worker integration test.
func (e *testEnv) qualifyFixtureRoutes(t *testing.T, id string) {
	t.Helper()
	ctx := context.Background()
	tour, err := e.store.GetTour(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range tour.Modes {
		route, err := e.store.GetTourRoute(ctx, id, mode)
		if err != nil {
			t.Fatal(err)
		}
		if route.Status != models.RouteStatusReady {
			t.Fatalf("fixture needs computed geometry, got %s: %s", route.Status, route.Error)
		}
		_, digest, err := e.store.TourNavigationInputs(ctx, id, mode, tour.RouteVersion)
		if err != nil {
			t.Fatal(err)
		}
		route.Provider = "valhalla"
		attr := &models.PathAttribution{Provider: "valhalla", Method: "trace_attributes/edge_walk", Legs: []models.AttributedLeg{}}
		for _, leg := range route.Legs {
			attr.Legs = append(attr.Legs, models.AttributedLeg{Geometry: leg.Geometry})
		}
		report := models.PathQualityReport{PathQualitySummary: models.PathQualitySummary{Version: 1, Status: "pass", Issues: []models.PathQualityIssue{}, InputCurrent: true, CheckedAt: time.Now().UTC()}, InputHash: digest, Mode: mode, Provider: route.Provider, RouteSHA256: route.RoutingFingerprint(), Attribution: attr, Inspections: []models.PathInspection{{SegmentID: "synthetic-http", Purpose: "network", EvidenceID: "synthetic-http-inspection", URL: "https://example.org/http-test", ContentSHA256: strings.Repeat("a", 64), RetrievedAt: time.Now().UTC(), Reference: "fixture", Quote: "Private synthetic HTTP persistence fixture."}}}
		if err := e.store.UpsertRouteQualified(ctx, *route, tour.RouteVersion, digest, report); err != nil {
			t.Fatal(err)
		}
	}
}
func (e *testEnv) publishFixture(t *testing.T, id string) {
	t.Helper()
	e.qualifyFixtureRoutes(t, id)
	ctx := context.Background()
	tour, err := e.store.GetTour(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.PublishTourRevision(ctx, id, "", models.PublishRevisionRequest{DraftVersion: tour.DraftVersion, CurrentRevisionID: tour.RevisionID}); err != nil {
		t.Fatal(err)
	}
}

func TestPathQualityHTTPIsPrivateCurrentAndRetained(t *testing.T) {
	e := newTestEnv(t)
	e.drain(t)
	curator := e.signIn(t, "quality-curator@example.com", models.AccountRoleCurator)
	visitor := e.signIn(t, "quality-visitor@example.com", "")
	base := "/api/v1/admin/tours/" + testdb.SeedTourID + "/routes/quality?mode=walk"
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {visitor, 403}} {
		if r := e.get(t, base, tc.token); r.status != tc.status {
			t.Fatalf("private gate %d %s", r.status, r.raw)
		}
	}
	r := e.get(t, base, curator)
	if r.status != 200 || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("checkpoint %d %s", r.status, r.raw)
	}
	report := r.body["report"].(map[string]any)
	if report["status"] != "blocked" || report["input_current"] != true {
		t.Fatalf("development report %s", r.raw)
	}
	retained := report["report_id"].(string)
	if r := e.get(t, base+"&report_id=bad", curator); r.status != 422 {
		t.Fatalf("invalid report ID %d", r.status)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/admin/tours/"+testdb.SeedTourID+"/routes/recompute", curator, nil); r.status != 202 {
		t.Fatal(r.status)
	}
	r = e.get(t, base+"&report_id="+retained, curator)
	if r.status != 200 || r.body["report"].(map[string]any)["input_current"] != false {
		t.Fatalf("stale checkpoint %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/admin/tours/"+testdb.SeedTourID+"/publish", curator, nil); r.status != 409 {
		t.Fatalf("stale publication %d %s", r.status, r.raw)
	}
	e.drain(t)
	e.qualifyFixtureRoutes(t, testdb.SeedTourID)
	r = e.get(t, base, curator)
	if r.status != 200 || !strings.Contains(string(r.raw), "Private synthetic HTTP") {
		t.Fatalf("private inspection absent %s", r.raw)
	}
	e.publishFixture(t, testdb.SeedTourID)
	public := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, "")
	if public.status != 200 || strings.Contains(string(public.raw), "synthetic-http-inspection") || strings.Contains(string(public.raw), "Private synthetic HTTP") {
		t.Fatal("inspection leaked publicly")
	}
}
