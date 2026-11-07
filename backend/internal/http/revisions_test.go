package httpapi

import (
	"net/http"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestRevisionHTTPRetainsLegacyTripAndExplicitStableProgress(t *testing.T) {
	e := newTestEnv(t)
	e.drain(t)
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	owner := e.signIn(t, "trip@example.com", "")
	other := e.signIn(t, "other@example.com", "")
	base := "/api/v1/tours/" + testdb.SeedTourID
	first := e.get(t, base, "")
	if first.status != 200 {
		t.Fatalf("first %s", first.raw)
	}
	id := first.body["revision_id"].(string)
	if r := e.get(t, base+"/revisions/"+id, ""); r.status != 200 || r.body["snapshot"].(map[string]any)["moments"] == nil {
		t.Fatalf("snapshot %d %s", r.status, r.raw)
	}
	legacy := "/api/v1/me/progress/" + testdb.SeedTourID
	if r := e.do(t, http.MethodPut, legacy, owner, map[string]any{"mode": "walk", "visited_positions": []int{1}}); r.status != 200 || r.body["revision_id"] != id {
		t.Fatalf("legacy %d %s", r.status, r.raw)
	}
	admin := e.get(t, "/api/v1/admin/tours/"+testdb.SeedTourID, curator)
	edit := map[string]any{"draft_version": admin.body["draft_version"], "stops": []map[string]any{{"place_id": testdb.SeedPlaceMuseumID}, {"place_id": testdb.SeedPlaceStationID}}}
	if r := e.do(t, http.MethodPut, "/api/v1/admin/tours/"+testdb.SeedTourID+"/stops", curator, edit); r.status != 200 {
		t.Fatalf("edit %d %s", r.status, r.raw)
	}
	if r := e.get(t, base, ""); r.body["revision_id"] != id || len(r.body["stops"].([]any)) != 4 {
		t.Fatalf("draft leaked %s", r.raw)
	}
	e.drain(t)
	admin = e.get(t, "/api/v1/admin/tours/"+testdb.SeedTourID, curator)
	release := map[string]any{"draft_version": admin.body["draft_version"], "current_revision_id": id}
	r := e.do(t, http.MethodPost, "/api/v1/admin/tours/"+testdb.SeedTourID+"/publish", curator, release)
	if r.status != 200 || r.body["revision_id"] == id {
		t.Fatalf("release %d %s", r.status, r.raw)
	}
	current := r.body["revision_id"].(string)
	if r := e.do(t, http.MethodPost, "/api/v1/admin/tours/"+testdb.SeedTourID+"/publish", curator, release); r.status != 409 {
		t.Fatalf("stale publish %d %s", r.status, r.raw)
	}
	if r := e.get(t, base, owner); r.body["revision_id"] != id || len(r.body["stops"].([]any)) != 4 {
		t.Fatalf("active trip changed %s", r.raw)
	}
	if r := e.get(t, base, ""); r.body["revision_id"] != current || len(r.body["stops"].([]any)) != 2 {
		t.Fatalf("latest view %s", r.raw)
	}
	progress := legacy + "/revisions/" + current
	if r := e.get(t, progress, other); r.status != 404 {
		t.Fatalf("other account progress %d", r.status)
	}
	stop := r.body["stops"].([]any)[0].(map[string]any)["id"].(string)
	if r := e.do(t, http.MethodPut, progress, owner, map[string]any{"mode": "walk", "visited_stop_ids": []string{stop}}); r.status != 200 || r.body["revision_id"] != current || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("stable progress %d %s", r.status, r.raw)
	}
	if r := e.get(t, progress, other); r.status != 404 {
		t.Fatalf("progress leaked %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPut, progress, owner, map[string]any{"mode": "walk", "visited_stop_ids": []string{testdb.SeedPlaceStationID}}); r.status != 422 {
		t.Fatalf("foreign stop %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPut, legacy, owner, map[string]any{"mode": "walk", "reset": true}); r.status != 200 || r.body["revision_id"] != current || len(r.body["visited_positions"].([]any)) != 0 {
		t.Fatalf("reset %d %s", r.status, r.raw)
	}
}

func TestUnreleasedPrivateRevisionDoesNotBecomePublicThroughVisibilityEdit(t *testing.T) {
	e := newTestEnv(t)
	owner := e.signIn(t, "private-owner@example.com", "")
	body := userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID)
	body["visibility"] = "private"
	r := e.do(t, http.MethodPost, "/api/v1/me/tours", owner, body)
	if r.status != 201 {
		t.Fatalf("create %s", r.raw)
	}
	id := r.body["id"].(string)
	e.drain(t)
	working := e.get(t, "/api/v1/me/tours/"+id, owner)
	privateRevision := working.body["revision_id"].(string)
	r = e.do(t, http.MethodPatch, "/api/v1/me/tours/"+id, owner, map[string]any{"visibility": "unlisted", "stops": []map[string]any{{"place_id": testdb.SeedPlaceMuseumID}, {"place_id": testdb.SeedPlaceStationID}}})
	if r.status != 200 {
		t.Fatalf("edit %d %s", r.status, r.raw)
	}
	base := "/api/v1/tours/" + id
	if r := e.get(t, base, ""); r.status != 404 {
		t.Fatalf("private release exposed %d %s", r.status, r.raw)
	}
	if r := e.get(t, base+"/revisions/"+privateRevision, ""); r.status != 404 {
		t.Fatalf("historical private release exposed %d %s", r.status, r.raw)
	}
	if r := e.get(t, base+"/revisions", ""); r.status != 200 || len(items(t, r)) != 0 {
		t.Fatalf("private metadata exposed %d %s", r.status, r.raw)
	}
	e.drain(t)
	if r := e.get(t, base, ""); r.status != 200 || len(r.body["stops"].([]any)) != 2 {
		t.Fatalf("new authorized public release %d %s", r.status, r.raw)
	}
	if r := e.get(t, base+"/revisions/"+privateRevision, ""); r.status != 404 {
		t.Fatalf("prior private content exposed after public release %d", r.status)
	}
}
