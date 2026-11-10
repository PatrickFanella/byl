package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestDevAuthAndSessions(t *testing.T) {
	env := newTestEnv(t)
	if r := env.get(t, "/api/v1/me", ""); r.status != 401 || errCode(r) != "unauthorized" {
		t.Fatalf("anonymous /me: %d %s", r.status, r.raw)
	}
	r := env.do(t, http.MethodPost, "/api/v1/auth/dev", "", map[string]string{"email": "not-an-email"})
	if r.status != 422 {
		t.Fatalf("bad email: %d %s", r.status, r.raw)
	}
	token := env.signIn(t, "admin@example.com", "")
	r = env.get(t, "/api/v1/me", token)
	if r.status != 200 || r.body["role"] != "admin" || r.body["email"] != "admin@example.com" {
		t.Fatalf("bootstrap admin: %d %s", r.status, r.raw)
	}
	long := env.do(t, http.MethodPost, "/api/v1/auth/dev", "", map[string]string{"email": "long@example.com", "display_name": strings.Repeat("é", 300)})
	if acct, _ := long.body["account"].(map[string]any); long.status != 200 || acct["display_name"] != strings.Repeat("é", maxDisplayNameRunes) {
		t.Fatalf("display names must be capped at %d runes: %d %s", maxDisplayNameRunes, long.status, long.raw)
	}
	visitor := env.signIn(t, "v@example.com", "")
	if r := env.get(t, "/api/v1/me", visitor); r.body["role"] != "visitor" {
		t.Fatalf("visitor: %s", r.raw)
	}
	if r := env.do(t, http.MethodDelete, "/api/v1/auth/session", visitor, nil); r.status != 204 {
		t.Fatalf("logout: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/me", visitor); r.status != 401 {
		t.Fatalf("after logout: %d", r.status)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/auth/apple", "", map[string]string{"identity_token": "x"}); r.status != 503 {
		t.Fatalf("apple without config should be 503, got %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/auth/google", "", map[string]string{"id_token": "x"}); r.status != 503 {
		t.Fatalf("google without config should be 503, got %d %s", r.status, r.raw)
	}
}

func TestDevAuthDisabled(t *testing.T) {
	env := newTestEnv(t)
	env.srv.Config.Handler = NewRouter(Deps{Store: env.store, Sessions: nil, DevAuth: false})
	if r := env.do(t, http.MethodPost, "/api/v1/auth/dev", "", map[string]string{"email": "a@b.c"}); r.status != 404 {
		t.Fatalf("disabled dev auth: %d", r.status)
	}
}

func userRoute(stops ...string) map[string]any {
	s := make([]map[string]any, 0, len(stops))
	for _, id := range stops {
		s = append(s, map[string]any{"place_id": id})
	}
	return map[string]any{"city": "downers-grove", "title": "My Loop", "stops": s}
}

func TestUserRoutesLifecycle(t *testing.T) {
	env := newTestEnv(t)
	me := env.signIn(t, "me@example.com", "")
	other := env.signIn(t, "other@example.com", "")

	if r := env.do(t, http.MethodPost, "/api/v1/me/tours", "", userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID)); r.status != 401 {
		t.Fatalf("anonymous create: %d", r.status)
	}
	r := env.do(t, http.MethodPost, "/api/v1/me/tours", me, userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID, testdb.SeedPlaceFishelID, testdb.SeedPlaceMuseumID))
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	slug := r.body["slug"].(string)
	id := r.body["id"].(string)
	if len(slug) != 10 || r.body["visibility"] != "unlisted" || r.body["status"] != "draft" || len(r.body["stops"].([]any)) != 4 {
		t.Fatalf("created route: %s", r.raw)
	}
	if r.body["stops"].([]any)[2].(map[string]any)["featured_moment"] == nil {
		t.Fatalf("user route stops should carry the place's primary moment: %s", r.raw)
	}
	queued, _ := env.store.ListJobs(context.Background(), models.JobStatusQueued, 10, 0)
	if len(queued) != 4 {
		t.Fatalf("expected 2 seed + 2 new route jobs queued, got %d", len(queued))
	}
	if r := env.get(t, "/api/v1/tours/"+slug, ""); r.status != 404 {
		t.Fatalf("unlisted route by link: %d", r.status)
	}
	if r := env.get(t, "/api/v1/cities/downers-grove/tours", ""); len(items(t, r)) != 1 {
		t.Fatalf("user routes must not appear in the public list: %s", r.raw)
	}
	if r := env.get(t, "/api/v1/me/tours", me); r.status != 200 || len(items(t, r)) != 1 {
		t.Fatalf("my tours: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/me/tours/"+id, other); r.status != 404 {
		t.Fatalf("other's GET must be 404: %d", r.status)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/me/tours/"+id, other, map[string]any{"title": "Hijacked"}); r.status != 404 {
		t.Fatalf("other's PATCH must be 404: %d", r.status)
	}
	r = env.do(t, http.MethodPatch, "/api/v1/me/tours/"+id, me, map[string]any{"title": "Renamed", "visibility": "private"})
	if r.status != 200 || r.body["title"] != "Renamed" || r.body["visibility"] != "private" {
		t.Fatalf("patch: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/tours/"+slug, ""); r.status != 404 {
		t.Fatalf("private route anonymous: %d", r.status)
	}
	if r := env.get(t, "/api/v1/tours/"+slug, me); r.status != 200 {
		t.Fatalf("private route owner: %d", r.status)
	}
	r = env.do(t, http.MethodPatch, "/api/v1/me/tours/"+id, me, map[string]any{"stops": []map[string]any{{"place_id": testdb.SeedPlaceMuseumID}, {"place_id": testdb.SeedPlaceStationID}}})
	if r.status != 200 || len(r.body["stops"].([]any)) != 2 || r.body["routes"].(map[string]any)["walk"].(map[string]any)["status"] != "pending" {
		t.Fatalf("replace stops: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodDelete, "/api/v1/me/tours/"+id, other, nil); r.status != 404 {
		t.Fatalf("other's DELETE must be 404: %d", r.status)
	}
	if r := env.do(t, http.MethodDelete, "/api/v1/me/tours/"+id, me, nil); r.status != 204 {
		t.Fatalf("delete: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/tours/"+slug, me); r.status != 404 {
		t.Fatalf("after delete: %d", r.status)
	}
}

func TestUserRouteValidation(t *testing.T) {
	env := newTestEnv(t)
	me := env.signIn(t, "me@example.com", "")
	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"one stop", userRoute(testdb.SeedPlaceStationID), "stops"},
		{"public visibility", func() map[string]any {
			b := userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID)
			b["visibility"] = "public"
			return b
		}(), "visibility"},
		{"blank title", func() map[string]any {
			b := userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID)
			b["title"] = " "
			return b
		}(), "title"},
	}
	for _, tc := range cases {
		r := env.do(t, http.MethodPost, "/api/v1/me/tours", me, tc.body)
		fields, _ := r.body["error"].(map[string]any)["fields"].(map[string]any)
		if r.status != 422 || fields[tc.field] == nil {
			t.Errorf("%s: %d %s", tc.name, r.status, r.raw)
		}
	}
	if err := env.store.SetPlaceStatus(context.Background(), testdb.SeedPlaceFishelID, models.EntityStatusDraft); err != nil {
		t.Fatal(err)
	}
	r := env.do(t, http.MethodPost, "/api/v1/me/tours", me, userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceFishelID))
	if r.status != 422 || errCode(r) != "validation_failed" {
		t.Fatalf("draft place stop: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPost, "/api/v1/me/tours", me, userRoute(testdb.SeedPlaceStationID, "00000000-0000-4000-8000-00000000ffff"))
	if r.status != 422 {
		t.Fatalf("unknown place stop: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPost, "/api/v1/me/tours", me, map[string]any{"city": "atlantis", "title": "x", "stops": []map[string]any{{"place_id": testdb.SeedPlaceStationID}, {"place_id": testdb.SeedPlaceTivoliID}}})
	if r.status != 404 {
		t.Fatalf("unknown city: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPost, "/api/v1/me/tours", me, map[string]any{"city": "downers-grove", "title": "x", "bogus": 1})
	if r.status != 400 || errCode(r) != "bad_request" {
		t.Fatalf("unknown field: %d %s", r.status, r.raw)
	}
}

func TestSavedAndProgressEndpoints(t *testing.T) {
	env := newTestEnv(t)
	me := env.signIn(t, "me@example.com", "")
	if r := env.do(t, http.MethodPut, "/api/v1/me/saved/downtown-memory-loop", me, nil); r.status != 204 {
		t.Fatalf("save: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/me/saved", me); r.status != 200 || len(items(t, r)) != 1 {
		t.Fatalf("saved list: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodDelete, "/api/v1/me/saved/downtown-memory-loop", me, nil); r.status != 204 {
		t.Fatalf("unsave: %d", r.status)
	}
	if r := env.get(t, "/api/v1/me/saved", me); len(items(t, r)) != 0 {
		t.Fatalf("after unsave: %s", r.raw)
	}
	if r := env.do(t, http.MethodPut, "/api/v1/me/saved/nope", me, nil); r.status != 404 {
		t.Fatalf("save unknown tour: %d", r.status)
	}

	r := env.do(t, http.MethodPut, "/api/v1/me/progress/downtown-memory-loop", me, map[string]any{"mode": "bike", "visited_positions": []int{1, 2}})
	if r.status != 200 || r.body["mode"] != "bike" || len(r.body["visited_positions"].([]any)) != 2 || r.body["completed_at"] != nil {
		t.Fatalf("put progress: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPut, "/api/v1/me/progress/downtown-memory-loop", me, map[string]any{"mode": "bike", "visited_positions": []int{1, 2, 3, 4}, "completed": true})
	if r.status != 200 || r.body["completed_at"] == nil {
		t.Fatalf("complete: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/me/progress/downtown-memory-loop", me); r.status != 200 || r.body["tour_id"] != testdb.SeedTourID {
		t.Fatalf("get progress: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/me/progress", me); len(items(t, r)) != 1 {
		t.Fatalf("list progress: %s", r.raw)
	}
	if r := env.do(t, http.MethodPut, "/api/v1/me/progress/downtown-memory-loop", me, map[string]any{"mode": "fly"}); r.status != 422 {
		t.Fatalf("bad mode: %d", r.status)
	}
	if r := env.get(t, "/api/v1/me/progress/nope", me); r.status != 404 {
		t.Fatalf("unknown tour progress: %d", r.status)
	}
}

func TestProgressUpdatesMergeAndReset(t *testing.T) {
	env := newTestEnv(t)
	me := env.signIn(t, "walker@example.com", "")
	path := "/api/v1/me/progress/downtown-memory-loop"
	put := func(body map[string]any) resp {
		return env.do(t, http.MethodPut, path, me, body)
	}
	first := put(map[string]any{"mode": "walk", "visited_positions": []int{1}, "completed": true})
	if first.status != 200 {
		t.Fatalf("first device: %d %s", first.status, first.raw)
	}
	second := put(map[string]any{"mode": "bike", "visited_positions": []int{2, 2}})
	if second.status != 200 || len(second.body["visited_positions"].([]any)) != 2 || second.body["completed_at"] != first.body["completed_at"] {
		t.Fatalf("second device must union stops and retain completion: %d %s", second.status, second.raw)
	}
	bad := put(map[string]any{"mode": "walk", "reset": true, "visited_positions": []int{1}})
	if bad.status != 422 {
		t.Fatalf("reset cannot also visit stops: %d %s", bad.status, bad.raw)
	}
	reset := put(map[string]any{"mode": "walk", "reset": true})
	if reset.status != 200 || len(reset.body["visited_positions"].([]any)) != 0 || reset.body["completed_at"] != nil {
		t.Fatalf("reset must clear stops and completion: %d %s", reset.status, reset.raw)
	}
	next := put(map[string]any{"mode": "walk", "visited_positions": []int{3}})
	if next.status != 200 || len(next.body["visited_positions"].([]any)) != 1 || next.body["completed_at"] != nil {
		t.Fatalf("new walk must not restore old stops: %d %s", next.status, next.raw)
	}
}
