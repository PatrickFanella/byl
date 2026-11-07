package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/itinerary"
	"invisible-museum/backend/internal/models"
)

func TestBriefItineraryIsCuratorOnlyAndBoundToItsRetainedRun(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	curator := e.signIn(t, "itinerary-curator@example.com", models.AccountRoleCurator)
	owner := e.signIn(t, "itinerary-owner@example.com", "")
	ownerID := e.get(t, "/api/v1/me", owner).body["id"].(string)
	b, _, err := e.store.CreateRouteBrief(ctx, ownerID, models.RouteBriefInput{Topic: "Workers", Area: "Chicago", TripShape: "out_and_back"}, "")
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/admin/route-briefs/" + b.ID
	path := base + "/itinerary"
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {owner, 403}} {
		if r := e.get(t, path, tc.token); r.status != tc.status {
			t.Fatalf("private boundary %d", r.status)
		}
	}
	if r := e.get(t, path, curator); r.status != 404 {
		t.Fatalf("no run %d %s", r.status, r.raw)
	}
	r := e.do(t, http.MethodPost, base+"/research", curator, map[string]any{"version": b.Version})
	if r.status != 202 {
		t.Fatal(r.status)
	}
	id := r.body["id"].(string)
	if r := e.get(t, path, curator); r.status != 200 || r.body["proposal"] != nil || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("pending proposal %d %s", r.status, r.raw)
	}
	// This transport fixture supplies a retained private checkpoint. The real
	// worker/capture pipeline and replay behavior have independent PG coverage.
	proposal := itinerary.Assemble(b.Submitted, models.ItineraryMaterials{Version: 1, Items: []models.ItineraryMaterial{}, Gaps: []models.ItineraryGap{{Code: "identity_unresolved", Detail: "PRIVATE-SOURCE-AND-CANDIDATE-GAP"}}}, time.Now().UTC())
	raw, _ := json.Marshal(proposal)
	if _, err := e.store.Pool().Exec(ctx, `UPDATE brief_research_stages SET output=$2 WHERE run_id=$1 AND stage='assembly'`, id, raw); err != nil {
		t.Fatal(err)
	}
	r = e.get(t, path+"?run_id="+id, curator)
	if r.status != 200 || r.body["source_version"] != float64(1) || !strings.Contains(string(r.raw), "PRIVATE-SOURCE-AND-CANDIDATE-GAP") {
		t.Fatalf("retained proposal %d %s", r.status, r.raw)
	}
	if r := e.get(t, "/api/v1/me/route-briefs/"+b.ID+"/research", owner); r.status != 200 || strings.Contains(string(r.raw), "PRIVATE-SOURCE-AND-CANDIDATE-GAP") {
		t.Fatal("private proposal leaked to owner progress")
	}
	other, _, err := e.store.CreateRouteBrief(ctx, ownerID, models.RouteBriefInput{Topic: "Other", Area: "Chicago"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := e.get(t, "/api/v1/admin/route-briefs/"+other.ID+"/itinerary?run_id="+id, curator); r.status != 404 {
		t.Fatalf("foreign retained run %d", r.status)
	}
	if r := e.get(t, path+"?run_id=bad", curator); r.status != 404 {
		t.Fatalf("invalid retained run %d", r.status)
	}
}
