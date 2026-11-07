package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
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
	shape := in["routes"].([]any)[0].(map[string]any)["shaping_points"].([]any)[0].(map[string]any)
	stops := tour["stops"].([]any)
	shape["after_stop_id"] = stops[0].(map[string]any)["id"]
	shape["before_stop_id"] = stops[1].(map[string]any)["id"]
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
