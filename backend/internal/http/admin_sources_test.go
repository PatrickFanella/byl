package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/sources"
	"invisible-museum/backend/internal/testdb"
)

type httpFakeDrafter struct {
	rank llm.Ranking
	last llm.RankRequest
}

func (f *httpFakeDrafter) DraftMoment(context.Context, llm.DraftRequest) (llm.MomentDraft, error) {
	return llm.MomentDraft{Title: "t", Body: "b", DatePrecision: "circa"}, nil
}

func (f *httpFakeDrafter) RankMoments(_ context.Context, req llm.RankRequest) (llm.Ranking, error) {
	f.last = req
	return f.rank, nil
}

type httpFakeAdapter struct{}

func (httpFakeAdapter) Name() string { return "fake" }
func (httpFakeAdapter) Discover(context.Context, sources.Request) ([]sources.Found, error) {
	return nil, nil
}

func withDrafter(d llm.Drafter) func(*Deps) {
	return func(deps *Deps) { deps.Drafter = d }
}

func TestAdminSourcesList(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	r := env.get(t, "/api/v1/admin/sources", curator)
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	names := map[string]map[string]any{}
	for _, it := range items(t, r) {
		m := it.(map[string]any)
		names[m["name"].(string)] = m
	}
	for _, n := range []string{"wikipedia", "wikimedia_commons", "internet_archive", "loc", "url"} {
		if names[n] == nil {
			t.Fatalf("missing %s in %v", n, names)
		}
	}
	if names["wikipedia"]["supports_near"] != true || names["loc"]["supports_query"] != true {
		t.Fatalf("flags %v", names)
	}
	if r := env.get(t, "/api/v1/admin/sources", ""); r.status != 401 {
		t.Fatalf("anonymous: %d", r.status)
	}
}

func TestAdminImportsEnqueue(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	r := env.do(t, http.MethodPost, "/api/v1/admin/imports", curator, map[string]any{"source": "wikipedia", "city": "downers-grove", "radius_m": 1200})
	if r.status != 202 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	job := r.body["job"].(map[string]any)
	if job["kind"] != "import_source" || job["status"] != "queued" {
		t.Fatalf("job %v", job)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(mustMarshal(job["payload"])), &payload)
	if payload["city_id"] != testdb.SeedCityID || payload["source"] != "wikipedia" || payload["radius_m"].(float64) != 1200 {
		t.Fatalf("payload %v", payload)
	}
	r = env.do(t, http.MethodPost, "/api/v1/admin/imports", curator, map[string]any{"source": "url", "city": "downers-grove", "place": "tivoli-theatre", "query": "marquee"})
	if r.status != 202 {
		t.Fatalf("with place: %d %s", r.status, r.raw)
	}
	_ = json.Unmarshal([]byte(mustMarshal(r.body["job"].(map[string]any)["payload"])), &payload)
	if payload["place_id"] != testdb.SeedPlaceTivoliID || payload["query"] != "marquee" {
		t.Fatalf("place payload %v", payload)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/imports", curator, map[string]any{"source": "nope", "city": "downers-grove"}); r.status != 422 {
		t.Fatalf("unknown source: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/imports", curator, map[string]any{"source": "wikipedia", "city": "atlantis"}); r.status != 404 {
		t.Fatalf("unknown city: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/imports", curator, map[string]any{"city": "downers-grove"}); r.status != 422 {
		t.Fatalf("missing source: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/imports", curator, map[string]any{"source": "wikipedia", "city": "downers-grove", "near": "91,0"}); r.status != 422 {
		t.Fatalf("bad near: %d %s", r.status, r.raw)
	}
}

func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func createMomentCandidate(t *testing.T, env *testEnv, token string) string {
	t.Helper()
	r := env.do(t, http.MethodPost, "/api/v1/admin/candidates", token, map[string]any{
		"kind": "moment", "origin": "import", "target_place_id": testdb.SeedPlaceStationID,
		"payload": map[string]any{"title": "Fire", "raw_text": "A fire damaged the depot.", "source_url": "https://x"},
	})
	if r.status != 201 {
		t.Fatalf("create candidate: %d %s", r.status, r.raw)
	}
	return r.body["id"].(string)
}

func TestAdminDraftCandidate(t *testing.T) {
	env := newTestEnvWith(t, withDrafter(&httpFakeDrafter{}))
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	id := createMomentCandidate(t, env, curator)
	r := env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+id+"/draft", curator, nil)
	if r.status != 202 || r.body["job"].(map[string]any)["kind"] != "draft_candidate" {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+id+"/reject", curator, map[string]any{"note": "no"}); r.status != 200 {
		t.Fatalf("reject: %d", r.status)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+id+"/draft", curator, nil); r.status != 409 {
		t.Fatalf("draft after review: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates", curator, map[string]any{"kind": "place", "payload": map[string]any{"city_id": testdb.SeedCityID, "name": "X", "latitude": 41.8, "longitude": -88}})
	if r := env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+r.body["id"].(string)+"/draft", curator, nil); r.status != 409 {
		t.Fatalf("draft place candidate: %d %s", r.status, r.raw)
	}

	noLLM := newTestEnv(t)
	tok := noLLM.signIn(t, "c@example.com", models.AccountRoleCurator)
	cid := createMomentCandidate(t, noLLM, tok)
	if r := noLLM.do(t, http.MethodPost, "/api/v1/admin/candidates/"+cid+"/draft", tok, nil); r.status != 503 || errCode(r) != "unavailable" {
		t.Fatalf("no llm: %d %s", r.status, r.raw)
	}
}

func TestAdminSuggestFeaturedMoment(t *testing.T) {
	fd := &httpFakeDrafter{rank: llm.Ranking{Ordered: []llm.RankedMoment{{ID: testdb.SeedMomentPlatformID, Reason: "daily rhythm"}, {ID: testdb.SeedMomentBurlingtonID, Reason: "origin story"}}}}
	env := newTestEnvWith(t, withDrafter(fd))
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	r := env.do(t, http.MethodPost, "/api/v1/admin/tours/downtown-memory-loop/stops/1/suggest", curator, nil)
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	ordered := r.body["ordered"].([]any)
	if len(ordered) != 2 || ordered[0].(map[string]any)["moment"].(map[string]any)["id"] != testdb.SeedMomentPlatformID || ordered[0].(map[string]any)["reason"] != "daily rhythm" {
		t.Fatalf("ordered %v", ordered)
	}
	if fd.last.TopicName != "Rail and the Commuter Town" || fd.last.PlaceName != "Main Street Station" || len(fd.last.Moments) != 2 {
		t.Fatalf("rank request %+v", fd.last)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/downtown-memory-loop/stops/9/suggest", curator, nil); r.status != 404 {
		t.Fatalf("bad position: %d", r.status)
	}
	noLLM := newTestEnv(t)
	tok := noLLM.signIn(t, "c@example.com", models.AccountRoleCurator)
	if r := noLLM.do(t, http.MethodPost, "/api/v1/admin/tours/downtown-memory-loop/stops/1/suggest", tok, nil); r.status != 503 {
		t.Fatalf("no llm: %d %s", r.status, r.raw)
	}
}

func TestAdminStats(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	createMomentCandidate(t, env, curator)
	r := env.get(t, "/api/v1/admin/stats", curator)
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	pending := r.body["pending_candidates"].(map[string]any)
	jobs := r.body["jobs"].(map[string]any)
	published := r.body["published"].(map[string]any)
	if pending["moment"].(float64) != 1 || jobs["queued"].(float64) != 2 || published["places"].(float64) != 4 || published["moments"].(float64) != 5 || published["tours"].(float64) != 1 {
		t.Fatalf("stats %s", r.raw)
	}
}

func TestMediaCandidateApprovalEnqueuesFetch(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	r := env.do(t, http.MethodPost, "/api/v1/admin/candidates", curator, map[string]any{
		"kind": "media", "origin": "import", "source_name": "wikimedia_commons", "external_id": "commons:1",
		"payload": map[string]any{"kind": "image", "media_url": "https://upload.wikimedia.org/x.jpg", "mime_type": "image/jpeg", "caption": "Depot", "credit": "Someone", "license": "CC-BY-2.0", "license_url": "https://creativecommons.org/licenses/by/2.0/"},
	})
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	before, _ := env.store.ListJobs(context.Background(), models.JobStatusQueued, 50, 0)
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+r.body["id"].(string)+"/approve", curator, nil)
	if r.status != 200 || r.body["approved_entity_id"] == nil {
		t.Fatalf("approve: %d %s", r.status, r.raw)
	}
	asset, err := env.store.GetMedia(context.Background(), r.body["approved_entity_id"].(string))
	if err != nil || asset.StorageKey != "" || asset.License != "CC-BY-2.0" || asset.Status != models.EntityStatusDraft {
		t.Fatalf("asset %v %+v", err, asset)
	}
	after, _ := env.store.ListJobs(context.Background(), models.JobStatusQueued, 50, 0)
	if len(after)-len(before) != 1 || after[0].Kind != models.JobKindFetchMedia {
		t.Fatalf("expected one fetch_media job, got %d new", len(after)-len(before))
	}
}
