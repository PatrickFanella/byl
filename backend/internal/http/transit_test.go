package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
	"invisible-museum/backend/internal/transit"
)

// Synthetic provider clock/points isolate persistence and client negotiation.
// Recorded official timetable rows are tested separately in internal/transit.
type transitFixturePlanner struct {
	mu    sync.Mutex
	state string
	drift bool
}

func (p *transitFixturePlanner) set(state string, drift bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state, p.drift = state, drift
}
func (p *transitFixturePlanner) Connect(_ context.Context, r transit.Request, now time.Time) transit.Connection {
	p.mu.Lock()
	defer p.mu.Unlock()
	source := &transit.Source{URL: transit.FeedURL, Publisher: "Chicago Transit Authority", SHA256: strings.Repeat("a", 64), RetrievedAt: now, LicenseURL: transit.LicenseURL, Timezone: "America/Chicago"}
	board := &transit.Stop{ID: "30374", Name: "Clark/Lake (Subway)", ParentID: "40380", Latitude: 41.885737, Longitude: -87.630886}
	alight := &transit.Stop{ID: "30077", Name: "Forest Park", ParentID: "40390", Latitude: 41.874257, Longitude: -87.817318}
	if p.drift {
		board.Latitude += .01
	}
	c := transit.Connection{State: p.state, Scheduled: true, Source: source, Board: board, Alight: alight, Issues: []string{"synthetic_provider_fixture"}}
	if p.state == "scheduled" {
		dep := r.DepartureAt.Add(5 * time.Minute).UTC()
		arr := dep.Add(40 * time.Minute)
		c.DepartureAt, c.ArrivalAt = &dep, &arr
		c.RouteID = "Blue"
		c.TripID = "synthetic-api-trip"
		c.Headsign = "Forest Park"
	}
	return c
}

func (e *testEnv) transitRequest(t *testing.T, method, path, token string, capable bool, payload any) resp {
	t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if capable {
		req.Header.Set("X-Byl-Capabilities", transitCapability)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Body.Close() }()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	out := resp{status: r.StatusCode, raw: raw, header: r.Header}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.body); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

type transitHTTPFixture struct {
	e                                        *testEnv
	curator, owner, oldRevision, newRevision string
	planner                                  *transitFixturePlanner
	intent                                   models.TransitIntent
}

func newTransitHTTPFixture(t *testing.T, enabled bool) transitHTTPFixture {
	t.Helper()
	planner := &transitFixturePlanner{state: "scheduled"}
	e := newTestEnvWith(t, func(d *Deps) { d.TransitPlanner = planner; d.TransitEnabled = enabled })
	e.drain(t)
	e.publishFixture(t, testdb.SeedTourID)
	curator := e.signIn(t, "transit-curator@example.com", models.AccountRoleCurator)
	owner := e.signIn(t, "transit-owner@example.com", "")
	base := "/api/v1/tours/" + testdb.SeedTourID
	original := e.get(t, base, "")
	if original.status != 200 {
		t.Fatal(string(original.raw))
	}
	old := original.body["revision_id"].(string)
	if r := e.do(t, "PUT", "/api/v1/me/progress/"+testdb.SeedTourID, owner, map[string]any{"mode": "walk", "visited_positions": []int{1}}); r.status != 200 {
		t.Fatal(string(r.raw))
	}
	working, err := e.store.GetTour(context.Background(), testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	intent := models.TransitIntent{Version: 1, Provider: "cta_gtfs_blue_v1", Mode: models.TravelModeWalk, FromStopID: working.Stops[0].ID, ToStopID: working.Stops[1].ID, BoardStopID: "30374", AlightStopID: "30077", WalkToStopSeconds: 300, WalkFromStopSeconds: 600, BoardingBufferSeconds: 180, WalkingAssumptions: "Explicit synthetic time assumptions; not evidence of real station access."}
	choice := models.TransitSelectionRequest{DraftVersion: working.DraftVersion, Intent: &intent, ReviewNote: "Explicit synthetic curator approval"}
	if r := e.do(t, "PUT", "/api/v1/admin/tours/"+working.ID+"/transit", curator, choice); r.status != 200 {
		t.Fatalf("selection: %d %s", r.status, r.raw)
	}
	e.publishFixture(t, working.ID)
	current, err := e.store.GetTour(context.Background(), working.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	return transitHTTPFixture{e, curator, owner, old, *current.RevisionID, planner, intent}
}

func TestTransitHTTPDefaultGateAndLegacyRevisionCompatibility(t *testing.T) {
	f := newTransitHTTPFixture(t, false)
	e := f.e
	base := "/api/v1/tours/" + testdb.SeedTourID
	if r := e.get(t, "/api/v1/itinerary-capabilities", ""); r.status != 200 || r.body["scheduled_transit_v1"] != false {
		t.Fatal("default gate")
	}
	for _, path := range []string{base, base + "/route?mode=walk", base + "/revisions/" + f.newRevision} {
		for _, capable := range []bool{false, true} {
			r := e.transitRequest(t, "GET", path, "", capable, nil)
			if r.status != 403 || r.body["error"].(map[string]any)["code"] != "unsupported_itinerary" || bytes.Contains(r.raw, []byte("coordinates")) {
				t.Fatalf("legacy/disabled route leaked: %d %s", r.status, r.raw)
			}
		}
	}
	if r := e.get(t, base+"/revisions/"+f.oldRevision, ""); r.status != 200 {
		t.Fatal("older revision blocked")
	}
	if r := e.get(t, base, f.owner); r.status != 200 || r.body["revision_id"] != f.oldRevision {
		t.Fatal("active legacy pin changed")
	}
	if r := e.get(t, base+"/route?mode=walk", f.owner); r.status != 200 || r.body["geometry"] == nil {
		t.Fatal("old pinned walking route lost")
	}
	if r := e.get(t, "/api/v1/me/progress/"+testdb.SeedTourID, f.owner); r.status != 200 || r.body["revision_id"] != f.oldRevision {
		t.Fatal("ordinary pinned progress changed")
	}
	if r := e.do(t, "PUT", "/api/v1/me/progress/"+testdb.SeedTourID, f.owner, map[string]any{"mode": "walk", "reset": true}); r.status != 403 || r.body["error"].(map[string]any)["code"] != "unsupported_itinerary" {
		t.Fatalf("legacy reset adopted disabled mixed revision: %d %s", r.status, r.raw)
	}
	if r := e.get(t, base, f.owner); r.status != 200 || r.body["revision_id"] != f.oldRevision {
		t.Fatal("rejected reset changed the active pin")
	}
	listed := e.get(t, "/api/v1/cities/downers-grove/tours", "")
	for _, item := range items(t, listed) {
		if item.(map[string]any)["id"] == testdb.SeedTourID {
			t.Fatal("unsupported mixed tour discovered")
		}
	}
}

func TestTransitHTTPSupportedPlanContainsExplicitSegmentsAndNoRailWalkingLine(t *testing.T) {
	f := newTransitHTTPFixture(t, true)
	e := f.e
	base := "/api/v1/tours/" + testdb.SeedTourID
	if r := e.get(t, base, ""); r.status != 403 {
		t.Fatal("legacy client received mixed itinerary")
	}
	if r := e.transitRequest(t, "GET", base, "", true, nil); r.status != 200 || r.body["transit"] == nil {
		t.Fatalf("capable detail: %d %s", r.status, r.raw)
	}
	listed := e.transitRequest(t, "GET", "/api/v1/cities/downers-grove/tours", "", true, nil)
	found := false
	for _, item := range items(t, listed) {
		found = found || item.(map[string]any)["id"] == testdb.SeedTourID
	}
	if !found {
		t.Fatal("qualified client could not discover mixed tour")
	}
	if r := e.transitRequest(t, "GET", base+"/route?mode=walk", "", true, nil); r.status != 409 || r.body["error"].(map[string]any)["code"] != "transit_plan_required" {
		t.Fatal("continuous walking route exposed")
	}
	r := e.transitRequest(t, "POST", base+"/revisions/"+f.newRevision+"/transit-plan", "", true, map[string]any{"version": 1, "departure_at": time.Now().Add(time.Hour).UTC()})
	if r.status != 200 || r.body["state"] != "scheduled" || r.body["departure_context_stop_id"] != f.intent.FromStopID {
		t.Fatalf("plan: %d %s", r.status, r.raw)
	}
	if r.header.Get("Cache-Control") != "no-store" || !strings.Contains(r.header.Get("Vary"), "X-Byl-Capabilities") {
		t.Fatal("plan cache negotiation")
	}
	counts := map[string]int{}
	for _, raw := range r.body["segments"].([]any) {
		segment := raw.(map[string]any)
		kind := segment["kind"].(string)
		counts[kind]++
		if kind != "walk" && segment["geometry"] != nil {
			t.Fatal("station/transit connector fabricated walking geometry")
		}
		if kind == "walk" && segment["from_stop_id"] == f.intent.FromStopID && segment["to_stop_id"] == f.intent.ToStopID {
			t.Fatal("replaced walking leg returned")
		}
	}
	for _, kind := range []string{"walk_to_stop", "boarding_wait", "transit", "walk_from_stop"} {
		if counts[kind] != 1 {
			t.Fatalf("missing explicit segment %s", kind)
		}
	}
	if counts["walk"] != 2 {
		t.Fatalf("ordinary walking sections lost: %v", counts)
	}
	service := r.body["service"].(map[string]any)
	if service["live"] != false || service["scheduled"] != true {
		t.Fatal("schedule presented as live")
	}
	progress := "/api/v1/me/progress/" + testdb.SeedTourID + "/revisions/" + f.newRevision
	if r := e.do(t, "PUT", progress, f.owner, map[string]any{"mode": "walk", "visited_stop_ids": []string{f.intent.FromStopID}}); r.status != 403 {
		t.Fatal("legacy progress entered mixed revision")
	}
	if r := e.transitRequest(t, "PUT", progress, f.owner, true, map[string]any{"mode": "walk", "visited_stop_ids": []string{f.intent.FromStopID}}); r.status != 200 || r.body["revision_id"] != f.newRevision {
		t.Fatalf("explicit mixed progress: %d %s", r.status, r.raw)
	}
	working := e.get(t, "/api/v1/admin/tours/"+testdb.SeedTourID, f.curator)
	changed := f.intent
	changed.WalkToStopSeconds = 1200
	if r := e.do(t, "PUT", "/api/v1/admin/tours/"+testdb.SeedTourID+"/transit", f.curator, map[string]any{"draft_version": working.body["draft_version"], "intent": changed, "review_note": "Explicit revised synthetic walking assumption, unpublished"}); r.status != 200 {
		t.Fatal(string(r.raw))
	}
	retained := e.transitRequest(t, "POST", base+"/revisions/"+f.newRevision+"/transit-plan", "", true, map[string]any{"version": 1, "departure_at": time.Now().Add(time.Hour).UTC()})
	if retained.status != 200 || retained.body["connector_version"] != float64(1) || retained.body["segments"].([]any)[0].(map[string]any)["duration_s"] != float64(300) {
		t.Fatalf("working selection leaked into frozen revision: %d %s", retained.status, retained.raw)
	}
}

func TestTransitHTTPUnavailableAndChangedPlatformNeverReturnUsableSegments(t *testing.T) {
	f := newTransitHTTPFixture(t, true)
	path := "/api/v1/tours/" + testdb.SeedTourID + "/revisions/" + f.newRevision + "/transit-plan"
	for _, state := range []string{"no_service", "stale", "provider_unavailable"} {
		f.planner.set(state, false)
		r := f.e.transitRequest(t, "POST", path, "", true, map[string]any{"version": 1, "departure_at": time.Now().Add(time.Hour).UTC()})
		if r.status != 200 || r.body["state"] != state || len(r.body["segments"].([]any)) != 0 {
			t.Fatalf("invented unavailable service: %d %s", r.status, r.raw)
		}
	}
	f.planner.set("scheduled", true)
	r := f.e.transitRequest(t, "POST", path, "", true, map[string]any{"version": 1, "departure_at": time.Now().Add(time.Hour).UTC()})
	if r.status != 200 || r.body["state"] != "needs_review" || len(r.body["segments"].([]any)) != 0 || r.body["service"].(map[string]any)["departure_at"] != nil {
		t.Fatalf("moved platform silently adopted: %d %s", r.status, r.raw)
	}
}

func TestTransitHTTPCuratorBoundaryStrictRequestsAndDisabledRemoval(t *testing.T) {
	f := newTransitHTTPFixture(t, false)
	e := f.e
	admin := "/api/v1/admin/tours/" + testdb.SeedTourID
	for _, token := range []string{"", f.owner} {
		if r := e.do(t, "PUT", admin+"/transit", token, map[string]any{}); r.status != 401 && r.status != 403 {
			t.Fatal("unprivileged transit edit")
		}
	}
	for _, body := range []any{map[string]any{"version": 1, "departure_at": time.Now().Add(time.Hour), "transfers": []string{"invented"}}, map[string]any{"version": 1, "departure_at": "2026-11-08"}, map[string]any{"version": 1, "departure_at": time.Now().Add(-time.Hour)}} {
		r := e.do(t, "POST", admin+"/transit-plan", f.curator, body)
		if r.status != 400 && r.status != 422 {
			t.Fatalf("invalid plan input: %d %s", r.status, r.raw)
		}
	}
	working := e.get(t, admin, f.curator)
	version := working.body["draft_version"]
	if r := e.do(t, "POST", admin+"/transit-plan", f.curator, map[string]any{"version": 1, "draft_version": version, "departure_at": time.Now().Add(time.Hour)}); r.status != 200 || r.body["state"] != "scheduled" {
		t.Fatalf("private preview while public gate off: %d %s", r.status, r.raw)
	} else if r.body["revision_id"] != nil || r.body["draft_version"] != version {
		t.Fatalf("working preview mislabeled as published revision: %s", r.raw)
	}
	if r := e.do(t, "PUT", admin+"/transit", f.curator, map[string]any{"draft_version": version, "remove": true, "review_note": "Explicit removal while public transit remains disabled"}); r.status != 200 || r.body["transit"] != nil {
		t.Fatalf("disabled removal: %d %s", r.status, r.raw)
	}
	if r := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, ""); r.status != 403 {
		t.Fatal("unpublished removal changed retained revision")
	}
	// A client downgrade can leave a mixed legacy pin. An explicit reset back to
	// a newly released ordinary revision must remain possible with transit off.
	account := e.get(t, "/api/v1/me", f.owner).body["id"].(string)
	if _, err := e.store.PutProgress(context.Background(), models.TourProgress{AccountID: account, TourID: testdb.SeedTourID, RevisionID: f.newRevision, Mode: models.TravelModeWalk}, true); err != nil {
		t.Fatal(err)
	}
	e.publishFixture(t, testdb.SeedTourID)
	if r := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, f.owner); r.status != 403 {
		t.Fatal("old mixed pin was silently replaced")
	}
	if r := e.do(t, "PUT", "/api/v1/me/progress/"+testdb.SeedTourID, f.owner, map[string]any{"mode": "walk", "reset": true}); r.status != 200 || r.body["revision_id"] == f.newRevision {
		t.Fatalf("safe explicit reset to ordinary revision blocked: %d %s", r.status, r.raw)
	}
}
