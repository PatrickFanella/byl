package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/jobs"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
)

func corridorRequest(t *testing.T, tour map[string]any, version int) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "fixtures", "corridors-v1", "intent.json"))
	if err != nil {
		t.Fatal(err)
	}
	var in map[string]any
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	stops := tour["stops"].([]any)
	coordinates := []any{}
	for i, stop := range stops {
		place := stop.(map[string]any)["place"].(map[string]any)
		point := []float64{place["longitude"].(float64), place["latitude"].(float64)}
		coordinates = append(coordinates, point)
		if i == 0 {
			next := stops[1].(map[string]any)["place"].(map[string]any)
			coordinates = append(coordinates, []float64{(point[0] + next["longitude"].(float64)) / 2, (point[1] + next["latitude"].(float64)) / 2})
		}
	}
	midpoint := coordinates[1].([]float64)
	in["segments"] = []any{map[string]any{"id": "synthetic-itinerary", "kind": "trail", "from_node": "start", "to_node": "end", "branches": []string{"fixture"}, "modes": []string{"walk", "bike"}, "geometry": map[string]any{"type": "LineString", "coordinates": coordinates}}}
	routes := []any{}
	for _, mode := range []string{"walk", "bike"} {
		routes = append(routes, map[string]any{"mode": mode, "branch": "fixture", "direction": "synthetic itinerary order", "path": []any{map[string]any{"segment_id": "synthetic-itinerary", "reverse": false}}, "allowed_connectors": []string{}, "shaping_points": []any{map[string]any{"id": "fixture-shape", "segment_id": "synthetic-itinerary", "traversal_index": 0, "after_stop_id": stops[0].(map[string]any)["id"], "before_stop_id": stops[1].(map[string]any)["id"], "latitude": midpoint[1], "longitude": midpoint[0], "note": "Synthetic control, not a story"}}})
	}
	in["routes"] = routes
	return map[string]any{"version": version, "intent": in}
}

func TestCorridorHTTPReviewAndExplicitPublication(t *testing.T) {
	e := newTestEnv(t)
	e.drain(t)
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	visitor := e.signIn(t, "visitor@example.com", "")
	public := "/api/v1/tours/" + testdb.SeedTourID
	admin := "/api/v1/admin/tours/" + testdb.SeedTourID
	tour := e.get(t, public, "").body
	e.approveCorridorFixtureArrivals(t, curator, tour)
	body := corridorRequest(t, tour, 0)
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {visitor, 403}} {
		if r := e.do(t, http.MethodPost, admin+"/corridors", tc.token, body); r.status != tc.status {
			t.Fatalf("role gate %d %s", r.status, r.raw)
		}
	}
	r := e.do(t, http.MethodPost, admin+"/corridors", curator, body)
	if r.status != 201 || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("import %d %s", r.status, r.raw)
	}
	proposal := r.body["id"].(string)
	review := admin + "/corridors/" + proposal + "/review"
	if r := e.do(t, http.MethodPost, review, curator, map[string]any{"version": 0, "action": "approve", "note": "PRIVATE CORRIDOR REVIEW"}); r.status != 200 {
		t.Fatalf("review %d %s", r.status, r.raw)
	}
	e.drain(t)
	if r := e.get(t, public, ""); r.body["revision_id"] != tour["revision_id"] || r.body["corridor"] != nil {
		t.Fatalf("review auto-published %s", r.raw)
	}
	working := e.get(t, admin, curator).body
	if r := e.do(t, http.MethodPost, admin+"/publish", curator, map[string]any{"draft_version": working["draft_version"], "current_revision_id": tour["revision_id"]}); r.status != 200 {
		t.Fatalf("publish %d %s", r.status, r.raw)
	}
	released := e.get(t, public, "")
	if released.body["corridor"].(map[string]any)["id"] != proposal || len(released.body["stops"].([]any)) != 4 || strings.Contains(string(released.raw), "PRIVATE CORRIDOR") || strings.Contains(string(released.raw), "reviewed_by") {
		t.Fatalf("public corridor %s", released.raw)
	}
	if r := e.get(t, admin+"/corridors/"+proposal, visitor); r.status != 403 {
		t.Fatalf("private review %d", r.status)
	}
	if r := e.get(t, admin+"/corridors/"+proposal, curator); r.status != 200 || r.body["review_note"] != "PRIVATE CORRIDOR REVIEW" {
		t.Fatalf("retained private review %d %s", r.status, r.raw)
	}
	// A proposal also compares the working draft, not just the intent pointer.
	r = e.do(t, http.MethodPost, admin+"/corridors", curator, corridorRequest(t, working, 1))
	if r.status != 201 {
		t.Fatalf("second proposal %s", r.raw)
	}
	second := r.body["id"].(string)
	if r := e.do(t, http.MethodPatch, admin, curator, map[string]any{"title": "Changed working title"}); r.status != 200 {
		t.Fatal(r.raw)
	}
	if r := e.do(t, http.MethodPost, admin+"/corridors/"+second+"/review", curator, map[string]any{"version": 1, "action": "approve", "note": "Old draft"}); r.status != 409 {
		t.Fatalf("stale draft approval %d %s", r.status, r.raw)
	}
	// Only an explicit curator import can reach the bounded body reader.
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+admin+"/corridors", strings.NewReader(strings.Repeat(" ", (256<<10)+1)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+curator)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, res.Body); err != nil {
		t.Error(err)
	}
	if err := res.Body.Close(); err != nil {
		t.Error(err)
	}
	if res.StatusCode != 413 {
		t.Fatalf("import body limit %d", res.StatusCode)
	}
}

func TestCorridorReviewDoesNotTriggerPersonalLegacyRelease(t *testing.T) {
	e := newTestEnv(t)
	owner := e.signIn(t, "owner@example.com", "")
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	r := e.do(t, http.MethodPost, "/api/v1/me/tours", owner, userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID))
	if r.status != 201 {
		t.Fatal(r.raw)
	}
	id := r.body["id"].(string)
	e.drain(t)
	tour := e.get(t, "/api/v1/me/tours/"+id, owner).body
	e.approveCorridorFixtureArrivals(t, curator, tour)
	admin := "/api/v1/admin/tours/" + id
	r = e.do(t, http.MethodPost, admin+"/corridors", curator, corridorRequest(t, tour, 0))
	if r.status != 201 {
		t.Fatal(r.raw)
	}
	proposal := r.body["id"].(string)
	if r := e.do(t, http.MethodPost, admin+"/corridors/"+proposal+"/review", curator, map[string]any{"version": 0, "action": "approve", "note": "Synthetic reviewed intent, pending explicit owner release"}); r.status != 200 {
		t.Fatal(r.raw)
	}
	e.drain(t)
	if r := e.get(t, "/api/v1/tours/"+id, owner); r.body["revision_id"] != tour["revision_id"] || r.body["corridor"] != nil {
		t.Fatalf("owner legacy release was implied by review %s", r.raw)
	}
	if r := e.do(t, http.MethodPost, admin+"/routes/recompute", curator, nil); r.status != 202 {
		t.Fatal(r.raw)
	}
	e.drain(t)
	if r := e.do(t, http.MethodPatch, "/api/v1/me/tours/"+id, owner, map[string]any{"title": "Edited pending corridor"}); r.status != 200 {
		t.Fatal(r.raw)
	}
	if r := e.get(t, "/api/v1/tours/"+id, owner); r.body["revision_id"] != tour["revision_id"] || r.body["corridor"] != nil {
		t.Fatalf("recompute/legacy save released a pending corridor %s", r.raw)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/me/tours/"+id+"/publish", owner, nil); r.status != 200 || r.body["corridor"] == nil {
		t.Fatalf("explicit owner release %d %s", r.status, r.raw)
	}
}

func TestCorridorPendingInitialViewRequiresExplicitRelease(t *testing.T) {
	e := newTestEnv(t)
	owner := e.signIn(t, "initial-owner@example.com", "")
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	r := e.do(t, http.MethodPost, "/api/v1/me/tours", owner, userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID))
	if r.status != 201 {
		t.Fatal(r.raw)
	}
	id := r.body["id"].(string)
	admin := "/api/v1/admin/tours/" + id
	tour := e.get(t, "/api/v1/me/tours/"+id, owner).body
	e.approveCorridorFixtureArrivals(t, curator, tour)
	r = e.do(t, http.MethodPost, admin+"/corridors", curator, corridorRequest(t, tour, 0))
	if r.status != 201 {
		t.Fatal(r.raw)
	}
	proposal := r.body["id"].(string)
	if r := e.do(t, http.MethodPost, admin+"/corridors/"+proposal+"/review", curator, map[string]any{"version": 0, "action": "approve", "note": "Pending first publication"}); r.status != 200 {
		t.Fatal(r.raw)
	}
	e.drain(t)
	if r := e.get(t, "/api/v1/tours/"+id, ""); r.status != 200 || r.body["revision_id"] != nil || r.body["corridor"] != nil {
		t.Fatalf("pending initial view exposed unpublished intent %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/me/tours/"+id+"/publish", owner, nil); r.status != 200 || r.body["revision_id"] == nil || r.body["corridor"] == nil {
		t.Fatalf("explicit first release %d %s", r.status, r.raw)
	}
}

// The HTTP review tests use a provider stub with explicit controlled capability;
// the development straight-line provider itself never receives that capability.
type corridorFixtureRouter struct{ routing.Router }

func (corridorFixtureRouter) RouteControlled(_ context.Context, _ models.TravelMode, request routing.ControlledRequest) (routing.Route, error) {
	controls := request.Controls
	result := routing.Route{Provider: "controlled-test-stub"}
	leg := routing.Leg{}
	for i, p := range controls {
		coordinate := [2]float64{p.Lng, p.Lat}
		result.Geometry = append(result.Geometry, coordinate)
		leg.Geometry = append(leg.Geometry, coordinate)
		if i > 0 {
			leg.DistanceM += geo.DistanceMeters(controls[i-1].Lat, controls[i-1].Lng, p.Lat, p.Lng)
		}
		if i > 0 && p.Break {
			leg.DurationS = int(math.Ceil(leg.DistanceM / 1.3))
			result.DistanceM += leg.DistanceM
			result.DurationS += leg.DurationS
			result.Legs = append(result.Legs, leg)
			leg = routing.Leg{Geometry: [][2]float64{coordinate}}
		}
	}
	return result, nil
}
func (e *testEnv) approveCorridorFixtureArrivals(t *testing.T, curator string, tour map[string]any) {
	t.Helper()
	for _, stop := range tour["stops"].([]any) {
		place := stop.(map[string]any)["place"].(map[string]any)
		base := "/api/v1/admin/places/" + place["id"].(string) + "/arrivals"
		lat, lng := place["latitude"].(float64), place["longitude"].(float64)
		in := models.ArrivalProposalRequest{Point: models.ArrivalPoint{Name: "Synthetic controlled-route entrance", Kind: "entrance", Latitude: &lat, Longitude: &lng}}
		for _, mode := range models.AllTravelModes {
			in.Point.Access = append(in.Point.Access, models.ArrivalAccess{Mode: mode, Status: "allowed", Surface: "unknown", SourceURL: "https://example.org/synthetic-access", SourceTitle: "Synthetic fixture access", RetrievedAt: "2026-11-07"})
		}
		proposal := e.do(t, http.MethodPost, base, curator, in)
		if proposal.status != 201 {
			t.Fatalf("arrival %d %s", proposal.status, proposal.raw)
		}
		if r := e.do(t, http.MethodPost, base+"/"+proposal.body["id"].(string)+"/review", curator, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic source context"}); r.status != 200 {
			t.Fatalf("arrival review %d %s", r.status, r.raw)
		}
	}
	e.worker = jobs.New(e.store, corridorFixtureRouter{routing.NewStraightLine()}, narration.None{}, e.blob, 1, slog.New(slog.NewTextHandler(io.Discard, nil)))
}
