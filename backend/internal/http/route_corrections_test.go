package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestRouteCorrectionHTTPPrivateComparisonAndOwnerOutcome(t *testing.T) {
	e := newTestEnv(t)
	e.drain(t)
	e.publishFixture(t, testdb.SeedTourID)
	owner := e.signIn(t, "correction-owner@example.com", "")
	foreign := e.signIn(t, "correction-foreign@example.com", "")
	curator := e.signIn(t, "correction-curator@example.com", models.AccountRoleCurator)
	tour := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, "").body
	input := reportBody(t, tour)
	input["details"] = "PRIVATE CORRECTION OBSERVATION"
	input["direction"] = "forward"
	submitted := e.do(t, "POST", "/api/v1/me/route-reports", owner, input)
	if submitted.status != 201 {
		t.Fatal(submitted.raw)
	}
	id := submitted.body["id"].(string)
	base := "/api/v1/admin/route-reports/" + id
	queued := e.do(t, "POST", base+"/investigations", curator, map[string]any{"version": 1})
	if queued.status != 202 {
		t.Fatal(queued.raw)
	}
	investigation := queued.body["id"].(string)
	e.drain(t)
	path := base + "/correction-comparison?investigation_id=" + investigation
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {owner, 403}} {
		if response := e.get(t, path, tc.token); response.status != tc.status {
			t.Fatalf("private comparison %d %s", response.status, response.raw)
		}
		if response := e.do(t, "POST", base+"/corrections", tc.token, map[string]any{}); response.status != tc.status {
			t.Fatalf("private release %d", response.status)
		}
		if response := e.get(t, "/api/v1/admin/route-corrections?due=true", tc.token); response.status != tc.status {
			t.Fatalf("private due queue %d", response.status)
		}
	}
	working := e.get(t, "/api/v1/admin/tours/"+testdb.SeedTourID, curator).body
	e.approveCorridorFixtureArrivals(t, curator, working)
	proposal := e.do(t, "POST", "/api/v1/admin/tours/"+testdb.SeedTourID+"/corridors", curator, corridorRequest(t, working, 0))
	if proposal.status != 201 {
		t.Fatal(proposal.raw)
	}
	review := e.do(t, "POST", "/api/v1/admin/tours/"+testdb.SeedTourID+"/corridors/"+proposal.body["id"].(string)+"/review", curator, map[string]any{"version": 0, "action": "approve", "note": "PRIVATE CURATOR CORRIDOR REASON"})
	if review.status != 200 {
		t.Fatal(review.raw)
	}
	e.drain(t)
	e.qualifyFixtureRoutes(t, testdb.SeedTourID)
	response := e.get(t, path, curator)
	if response.status != 200 || response.header.Get("Cache-Control") != "no-store" || !strings.Contains(response.header.Get("Vary"), "Authorization") {
		t.Fatalf("comparison/cache %d %s", response.status, response.raw)
	}
	var comparison models.RouteCorrectionComparison
	if err := json.Unmarshal(response.raw, &comparison); err != nil {
		t.Fatal(err)
	}
	if !comparison.ReadyToPublish {
		t.Fatalf("prepared correction blocked %+v", comparison.Blockers)
	}
	approve := models.ApproveRouteCorrectionRequest{Version: comparison.Version, InvestigationID: investigation, Note: "Reviewed correction released with retained routing intent."}
	malformed := e.do(t, "POST", base+"/corrections", curator, map[string]any{"version": comparison.Version, "investigation_id": investigation, "note": "Review", "temporary": true})
	if malformed.status != 422 {
		t.Fatalf("missing detour review date %d", malformed.status)
	}
	stale := approve
	stale.Version = strings.Repeat("0", 64)
	if res := e.do(t, "POST", base+"/corrections", curator, stale); res.status != 409 {
		t.Fatalf("stale comparison %d %s", res.status, res.raw)
	}
	released := e.do(t, "POST", base+"/corrections", curator, approve)
	if released.status != 200 {
		t.Fatalf("release %d %s", released.status, released.raw)
	}
	outcome := "/api/v1/me/route-reports/" + id + "/correction"
	if res := e.get(t, outcome, foreign); res.status != 404 {
		t.Fatal("foreign outcome visible")
	}
	if res := e.get(t, outcome, ""); res.status != 401 {
		t.Fatal("anonymous outcome visible")
	}
	own := e.get(t, outcome, owner)
	if own.status != 200 || own.body["revision_id"] != released.body["revision_id"] || own.body["previous_revision_id"] != tour["revision_id"] || strings.Contains(string(own.raw), "reviewed_by") || strings.Contains(string(own.raw), "potentially_affected_tours") || strings.Contains(string(own.raw), "investigation_id") {
		t.Fatalf("owner outcome %d %s", own.status, own.raw)
	}
	public := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, "")
	if public.status != 200 || strings.Contains(string(public.raw), "PRIVATE CORRECTION") || strings.Contains(string(public.raw), "PRIVATE CURATOR") {
		t.Fatal("private correction leaked publicly")
	}
	status := e.get(t, "/api/v1/me/route-reports/"+id, owner)
	if status.body["status"] != "accepted" || status.body["reason"] != approve.Note {
		t.Fatal("owner not notified through report status")
	}
	if res := e.get(t, "/api/v1/admin/route-corrections?due=invalid", curator); res.status != 422 {
		t.Fatal("malformed due filter accepted")
	}
}
