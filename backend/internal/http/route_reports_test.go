package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func submitReport(t *testing.T, e *testEnv, token, key, body string) (int, http.Header, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/api/v1/me/route-reports", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Idempotency-Key", key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res.Header, data
}

func reportBody(t *testing.T, tour map[string]any) map[string]any {
	t.Helper()
	stops := tour["stops"].([]any)
	return map[string]any{"tour_id": tour["id"], "revision_id": tour["revision_id"], "mode": "walk", "reason": "blocked", "affected": map[string]any{"kind": "entrance", "stop_id": stops[0].(map[string]any)["id"]}}
}

func TestRouteReportHTTPPrivacyRetriesAndDisposition(t *testing.T) {
	e := newTestEnv(t)
	e.drain(t)
	owner := e.signIn(t, "reporter@example.com", "")
	other := e.signIn(t, "other@example.com", "")
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	tour := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, "").body
	input := reportBody(t, tour)
	input["details"] = "PRIVATE-OPERATIONAL-OBSERVATION"
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if status, _, _ := submitReport(t, e, "", "report", string(raw)); status != 401 {
		t.Fatalf("anonymous submission %d", status)
	}
	status, headers, first := submitReport(t, e, owner, "report", string(raw))
	if status != 201 || headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("minimal report %d %+v", status, first)
	}
	submitted := first["submitted"].(map[string]any)
	if submitted["duration"] != "unknown" || submitted["direction"] != "unknown" || submitted["observed_on"] != nil || submitted["alternative"] != nil || submitted["source_url"] != "" {
		t.Fatalf("minimal unknowns %+v", submitted)
	}
	status, _, again := submitReport(t, e, owner, "report", string(raw))
	if status != 200 || again["id"] != first["id"] {
		t.Fatalf("retry %d %+v", status, again)
	}
	id := first["id"].(string)
	for _, tc := range []struct {
		token, path string
		status      int
	}{
		{"", "/me/route-reports/" + id, 401},
		{owner, "/me/route-reports/" + id, 200},
		{other, "/me/route-reports/" + id, 404},
		{owner, "/admin/route-reports/" + id, 403},
		{curator, "/admin/route-reports/" + id, 200},
		{"", "/admin/route-reports", 401},
	} {
		if r := e.get(t, "/api/v1"+tc.path, tc.token); r.status != tc.status {
			t.Fatalf("privacy %s %d %s", tc.path, r.status, r.raw)
		}
	}
	if r := e.get(t, "/api/v1/me/route-reports", other); r.status != 200 || len(items(t, r)) != 0 {
		t.Fatalf("foreign list %s", r.raw)
	}
	state := map[string]any{"version": 1, "status": "accepted", "reason": "Private disposition only; independent route review still required."}
	path := "/api/v1/admin/route-reports/" + id + "/state"
	if r := e.do(t, http.MethodPost, path, owner, state); r.status != 403 {
		t.Fatalf("owner disposition %d", r.status)
	}
	if r := e.do(t, http.MethodPost, path, curator, state); r.status != 200 || r.body["version"] != float64(2) {
		t.Fatalf("curator disposition %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, path, curator, state); r.status != 409 {
		t.Fatalf("stale disposition %d", r.status)
	}
	if r := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, ""); r.body["revision_id"] != tour["revision_id"] || strings.Contains(string(r.raw), "PRIVATE-OPERATIONAL") {
		t.Fatal("report changed or leaked into the public tour")
	}
	input["reason"] = "unsafe"
	changed, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if status, _, _ := submitReport(t, e, owner, "report", string(changed)); status != 409 {
		t.Fatalf("conflicting retry %d", status)
	}
	for _, tc := range []struct {
		field  string
		value  any
		status int
	}{
		{"owner_account_id", "forged-owner", 400},
		{"observed_on", "not-a-date", 400},
		{"source_url", "javascript:alert(1)", 422},
		{"annotation", map[string]any{}, 422},
		{"alternative", map[string]any{"type": "LineString", "coordinates": [][]float64{{1, 2, 3}, {4, 5}}}, 422},
	} {
		var invalid map[string]any
		if err := json.Unmarshal(raw, &invalid); err != nil {
			t.Fatal(err)
		}
		invalid[tc.field] = tc.value
		if r := e.do(t, http.MethodPost, "/api/v1/me/route-reports", owner, invalid); r.status != tc.status {
			t.Fatalf("invalid %s %d %s", tc.field, r.status, r.raw)
		}
	}
	for range 9 {
		if status, _, _ := submitReport(t, e, owner, "", string(raw)); status != 201 {
			t.Fatalf("quota setup %d", status)
		}
	}
	if status, _, _ := submitReport(t, e, owner, "", string(raw)); status != 429 {
		t.Fatalf("quota %d", status)
	}
	if status, _, _ := submitReport(t, e, owner, "report", string(raw)); status != 200 {
		t.Fatalf("retry at quota %d", status)
	}
	if status, _, _ := submitReport(t, e, owner, "", strings.Repeat(" ", (32<<10)+1)); status != 413 {
		t.Fatalf("body limit %d", status)
	}
	if status, _, _ := submitReport(t, e, owner, strings.Repeat("x", 129), string(raw)); status != 422 {
		t.Fatalf("key limit %d", status)
	}
}

func TestRouteReportsRejectPrivateHistoricalRevisions(t *testing.T) {
	e := newTestEnv(t)
	owner := e.signIn(t, "private-report-owner@example.com", "")
	other := e.signIn(t, "private-report-other@example.com", "")
	in := userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID)
	in["visibility"] = "private"
	r := e.do(t, http.MethodPost, "/api/v1/me/tours", owner, in)
	if r.status != 201 {
		t.Fatal(r.raw)
	}
	id := r.body["id"].(string)
	e.drain(t)
	tour := e.get(t, "/api/v1/me/tours/"+id, owner).body
	input := reportBody(t, tour)
	if r := e.do(t, http.MethodPost, "/api/v1/me/route-reports", other, input); r.status != 404 {
		t.Fatalf("private route accepted %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/me/route-reports", owner, input); r.status != 201 {
		t.Fatalf("owner private report %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPatch, "/api/v1/me/tours/"+id, owner, map[string]any{"visibility": "unlisted"}); r.status != 200 {
		t.Fatalf("unlisted %s", r.raw)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/me/route-reports", other, input); r.status != 404 {
		t.Fatalf("historical private snapshot exposed %d %s", r.status, r.raw)
	}
	tour = e.get(t, "/api/v1/tours/"+id, "").body
	if r := e.do(t, http.MethodPost, "/api/v1/me/route-reports", other, reportBody(t, tour)); r.status != 201 {
		t.Fatalf("new unlisted report %d %s", r.status, r.raw)
	}
}
