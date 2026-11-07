package httpapi

import (
	"net/http"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestArrivalPublicUnknownAndCuratorReview(t *testing.T) {
	e := newTestEnv(t)
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	visitor := e.signIn(t, "visitor@example.com", "")
	public := "/api/v1/places/" + testdb.SeedPlaceStationID + "/arrival"
	if r := e.get(t, public, ""); r.status != 200 || r.body["eligibility"] != "unknown" || r.body["navigation_point"] != nil {
		t.Fatalf("legacy %d %s", r.status, r.raw)
	}
	if r := e.get(t, public+"?mode=drive", ""); r.status != 422 {
		t.Fatalf("mode %d", r.status)
	}
	base := "/api/v1/admin/places/" + testdb.SeedPlaceStationID + "/arrivals"
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {visitor, 403}} {
		for _, m := range []string{http.MethodGet, http.MethodPost} {
			r := e.do(t, m, base, tc.token, map[string]any{})
			if r.status != tc.status {
				t.Fatalf("private %d %s", r.status, r.raw)
			}
		}
	}
	lat, lng := 41.805, -88.008
	in := models.ArrivalProposalRequest{Point: models.ArrivalPoint{Name: "Synthetic burial entrance lead; no field inspection", Kind: "viewpoint", Latitude: &lat, Longitude: &lng, Access: []models.ArrivalAccess{
		{Mode: models.TravelModeWalk, Status: "allowed", Surface: "unknown", SourceURL: "https://archive.example/fixture", SourceTitle: "Fixture access record", RetrievedAt: "2026-01-01"},
		{Mode: models.TravelModeBike, Status: "closed", Surface: "unknown", SourceURL: "https://archive.example/fixture", SourceTitle: "Fixture bike restriction", RetrievedAt: "2026-01-01"},
	}}}
	r := e.do(t, http.MethodPost, base, curator, in)
	if r.status != 201 || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("propose %d %s", r.status, r.raw)
	}
	id := r.body["id"].(string)
	if r := e.get(t, public, ""); r.body["navigation_point"] != nil {
		t.Fatal("pending proposal public")
	}
	decision := models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic fixture review"}
	if r := e.do(t, http.MethodPost, base+"/"+id+"/review", visitor, decision); r.status != 403 {
		t.Fatalf("visitor review %d", r.status)
	}
	if r := e.do(t, http.MethodPost, base+"/"+id+"/review", curator, decision); r.status != 200 {
		t.Fatalf("review %d %s", r.status, r.raw)
	}
	if r := e.get(t, public+"?mode=walk", ""); r.status != 200 || r.body["navigation_point"] == nil || r.body["eligibility"] != "allowed" {
		t.Fatalf("walk %d %s", r.status, r.raw)
	}
	if r := e.get(t, public+"?mode=bike", ""); r.status != 200 || r.body["navigation_point"] != nil || r.body["eligibility"] != "closed" {
		t.Fatalf("bike %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, base+"/"+id+"/review", curator, decision); r.status != 409 {
		t.Fatalf("re-review %d", r.status)
	}
	if r := e.get(t, base, curator); r.status != 200 || len(items(t, r)) != 1 {
		t.Fatalf("history %d %s", r.status, r.raw)
	}
}
