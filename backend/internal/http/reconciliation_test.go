package httpapi

import (
	"net/http"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestReconciliationPrivateContract(t *testing.T) {
	e := newTestEnv(t)
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	owner := e.signIn(t, "owner@example.com", "")
	b := e.do(t, http.MethodPost, "/api/v1/admin/route-briefs", curator, map[string]any{"topic": "Railroad history", "area": "Downers Grove"})
	if b.status != 201 {
		t.Fatalf("brief %s", b.raw)
	}
	base := "/api/v1/admin/route-briefs/" + b.body["id"].(string) + "/reconciliation"
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {owner, 403}} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
			path := base
			if method == http.MethodPut {
				path += "/00000000-0000-4000-8000-000000000099/decision"
			}
			r := e.do(t, method, path, tc.token, map[string]any{})
			if r.status != tc.status {
				t.Fatalf("privacy %s %d %s", method, r.status, r.raw)
			}
		}
	}
	in := models.ReconciliationRequest{Version: 1, Item: models.DiscoveryItem{Kind: models.CandidateKindPlace, EntityID: testdb.SeedPlaceStationID, Title: "Main Street Station"}}
	r := e.do(t, http.MethodPost, base, curator, in)
	if r.status != 200 || r.header.Get("Cache-Control") != "no-store" || r.body["proposal"].(map[string]any)["outcome"] != "existing_place" {
		t.Fatalf("identity %d %s", r.status, r.raw)
	}
	id := r.body["id"].(string)
	if r := e.get(t, base+"/"+id+"/sources", curator); r.status != 200 || len(items(t, r)) != 1 || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("sources %d %s", r.status, r.raw)
	}
	if r := e.get(t, base+"/"+id+"/sources", owner); r.status != 403 {
		t.Fatalf("source privacy %d", r.status)
	}
	if r := e.get(t, base, curator); r.status != 200 || len(items(t, r)) != 1 {
		t.Fatalf("list %d %s", r.status, r.raw)
	}
	in.Version = 2
	if r := e.do(t, http.MethodPost, base, curator, in); r.status != 409 {
		t.Fatalf("stale %d %s", r.status, r.raw)
	}
	in.Version = 1
	in.Item.URL = "https://user:secret@example.org/"
	if r := e.do(t, http.MethodPost, base, curator, in); r.status != 422 {
		t.Fatalf("URL %d %s", r.status, r.raw)
	}
	decision := models.IdentityDecisionRequest{Version: 0, Outcome: "existing_place", PlaceID: testdb.SeedPlaceStationID, Note: "Reviewed fixture identity"}
	if r := e.do(t, http.MethodPut, base+"/"+id+"/decision", curator, decision); r.status != 200 || r.body["review_version"] != float64(1) {
		t.Fatalf("decision %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPut, base+"/"+id+"/decision", curator, decision); r.status != 409 {
		t.Fatalf("stale decision %d %s", r.status, r.raw)
	}
	other := e.do(t, http.MethodPost, "/api/v1/admin/route-briefs", curator, map[string]any{"topic": "Other", "area": "Downers Grove"})
	otherPath := "/api/v1/admin/route-briefs/" + other.body["id"].(string) + "/reconciliation/" + id + "/decision"
	decision.Version = 1
	if r := e.do(t, http.MethodPut, otherPath, curator, decision); r.status != 404 {
		t.Fatalf("unlinked %d %s", r.status, r.raw)
	}
}
