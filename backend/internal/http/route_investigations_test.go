package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestRouteInvestigationHTTPRequiresCuratorAndKeepsPrivateCheckpoint(t *testing.T) {
	e := newTestEnv(t)
	e.drain(t)
	e.publishFixture(t, testdb.SeedTourID)
	owner := e.signIn(t, "investigation-reporter@example.com", "")
	curator := e.signIn(t, "investigation-curator@example.com", models.AccountRoleCurator)
	tour := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, "").body
	r := e.do(t, http.MethodPost, "/api/v1/me/route-reports", owner, reportBody(t, tour))
	if r.status != 201 {
		t.Fatal(r.raw)
	}
	base := "/api/v1/admin/route-reports/" + r.body["id"].(string)
	request := map[string]any{"version": 1, "research": map[string]any{"topic": "PRIVATE HISTORY LEAD", "area": "Test area", "event_leads": []string{"A claim needs source inspection"}}}
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {owner, 403}} {
		for _, suffix := range []string{"/related", "/investigations"} {
			if r := e.get(t, base+suffix, tc.token); r.status != tc.status {
				t.Fatalf("private GET %d %s", r.status, r.raw)
			}
		}
		if r := e.do(t, http.MethodPost, base+"/investigations", tc.token, request); r.status != tc.status {
			t.Fatalf("private queue %d %s", r.status, r.raw)
		}
	}
	if r := e.do(t, http.MethodPost, base+"/investigations", curator, map[string]any{"version": 0}); r.status != 422 {
		t.Fatalf("validation %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, base+"/investigations", curator, map[string]any{"version": 99}); r.status != 409 {
		t.Fatalf("stale %d %s", r.status, r.raw)
	}
	r = e.do(t, http.MethodPost, base+"/investigations", curator, request)
	if r.status != 202 {
		t.Fatalf("queue %d %s", r.status, r.raw)
	}
	path := base + "/investigations/" + r.body["id"].(string)
	if r := e.get(t, path, owner); r.status != 403 {
		t.Fatal("reporter could read private research")
	}
	e.drain(t)
	r = e.get(t, path, curator)
	if r.status != 200 || r.body["result"] == nil || !strings.Contains(string(r.raw), "historical_research_required") || !strings.Contains(string(r.raw), "PRIVATE HISTORY LEAD") {
		t.Fatalf("checkpoint %d %s", r.status, r.raw)
	}
	if r := e.get(t, base+"/investigations", curator); r.status != 200 || len(items(t, r)) != 1 {
		t.Fatal("missing investigation list")
	}
	if r := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, ""); strings.Contains(string(r.raw), "PRIVATE HISTORY LEAD") || strings.Contains(string(r.raw), "independent_corroboration") {
		t.Fatal("private investigation leaked to public tour")
	}
	res, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Header.Set("Authorization", "Bearer "+curator)
	response, err := http.DefaultClient.Do(res)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.Header.Get("Cache-Control") != "no-store" || !strings.Contains(response.Header.Get("Vary"), "Authorization") {
		t.Fatal("private caching headers missing")
	}
}
