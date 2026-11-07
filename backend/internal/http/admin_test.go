package httpapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestAdminRoleGating(t *testing.T) {
	env := newTestEnv(t)
	visitor := env.signIn(t, "v@example.com", "")
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	if r := env.get(t, "/api/v1/admin/places", ""); r.status != 401 {
		t.Fatalf("anonymous: %d", r.status)
	}
	if r := env.get(t, "/api/v1/admin/places", visitor); r.status != 403 || errCode(r) != "forbidden" {
		t.Fatalf("visitor: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/admin/places", curator); r.status != 200 || len(items(t, r)) != 4 {
		t.Fatalf("curator: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/admin/accounts", curator); r.status != 403 {
		t.Fatalf("curator on accounts: %d", r.status)
	}
}

func TestAdminContentLifecycle(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	seedMediaCount := len(items(t, env.get(t, "/api/v1/admin/media", curator)))

	r := env.do(t, http.MethodPost, "/api/v1/admin/places", curator, map[string]any{"city_id": testdb.SeedCityID, "name": "Lyman Woods", "latitude": 41.82, "longitude": -87.99, "kind": "park"})
	if r.status != 201 || r.body["status"] != "draft" || r.body["slug"] != "lyman-woods" {
		t.Fatalf("create place: %d %s", r.status, r.raw)
	}
	placeID := r.body["id"].(string)
	if r := env.get(t, "/api/v1/places/"+placeID, ""); r.status != 404 {
		t.Fatalf("draft place public: %d", r.status)
	}
	if r := env.get(t, "/api/v1/admin/places/"+placeID, curator); r.status != 200 {
		t.Fatalf("draft place admin: %d", r.status)
	}
	r = env.do(t, http.MethodPatch, "/api/v1/admin/places/"+placeID, curator, map[string]any{"summary": "A preserve.", "arrival_radius_m": 80})
	if r.status != 200 || r.body["arrival_radius_m"].(float64) != 80 {
		t.Fatalf("patch place: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/places/"+placeID, curator, map[string]any{"latitude": 999}); r.status != 422 {
		t.Fatalf("invalid patch: %d", r.status)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/places/"+placeID+"/publish", curator, nil); r.status != 409 {
		t.Fatalf("incomplete place must stay a draft: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/places/"+placeID, ""); r.status != 404 {
		t.Fatalf("incomplete place must stay hidden: %d", r.status)
	}

	r = env.do(t, http.MethodPost, "/api/v1/admin/moments", curator, map[string]any{
		"place_id": placeID, "title": "Preserve Established", "body": strings.Repeat("This is a sourced historical account of the preserve and its creation. ", 10) + "\n\n" + strings.Repeat("The local records explain its context and the people who established it. ", 10) + "\n\n" + strings.Repeat("The surviving landscape connects that history with the present day. ", 10), "date_start": "1981-01-01", "date_precision": "year", "date_display": "1981",
		"topic_ids": []string{testdb.SeedTopicParksID}, "sources": []map[string]any{{"title": "Forest Preserve District", "publisher": "Forest Preserve District", "url": "https://example.org"}},
	})
	if r.status != 201 || r.body["status"] != "draft" || len(r.body["sources"].([]any)) != 1 {
		t.Fatalf("create moment: %d %s", r.status, r.raw)
	}
	momentID := r.body["id"].(string)

	// Upload an image without a licence, attach it, and watch publish refuse.
	asset := uploadMedia(t, env, curator, "photo.jpg", "image/jpeg", []byte("\xff\xd8\xffjpeg"), map[string]string{"caption": "The lawn"})
	if asset["kind"] != "image" || asset["public_url"] == "" {
		t.Fatalf("asset: %v", asset)
	}
	assetID := asset["id"].(string)
	if _, err := os.Stat(filepath.Join(env.fsDir, asset["storage_key"].(string))); err != nil {
		t.Fatalf("uploaded file missing: %v", err)
	}
	if r := env.get(t, "/api/v1/media/"+assetID, ""); r.status != 200 || r.body["public_url"] != asset["public_url"] {
		t.Fatalf("media metadata: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPut, "/api/v1/admin/moments/"+momentID+"/media", curator, map[string]any{"media_ids": []string{assetID}}); r.status != 200 {
		t.Fatalf("attach media: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPost, "/api/v1/admin/moments/"+momentID+"/publish", curator, nil)
	if r.status != 409 || errCode(r) != "conflict" || !strings.Contains(errMessage(r), "license") {
		t.Fatalf("publish without licence: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/media/"+assetID, curator, map[string]any{"license": "CC-BY-4.0", "credit": "Someone", "license_url": "https://creativecommons.org/licenses/by/4.0/", "source_url": "https://example.org/photo"}); r.status != 200 || r.body["license"] != "CC-BY-4.0" {
		t.Fatalf("patch media: %d %s", r.status, r.raw)
	}
	before, _ := env.store.ListJobs(context.Background(), models.JobStatusQueued, 50, 0)
	r = env.do(t, http.MethodPost, "/api/v1/admin/moments/"+momentID+"/publish", curator, nil)
	if r.status != 200 || r.body["status"] != "published" {
		t.Fatalf("publish moment: %d %s", r.status, r.raw)
	}
	after, _ := env.store.ListJobs(context.Background(), models.JobStatusQueued, 50, 0)
	if len(after)-len(before) != 1 || after[0].Kind != models.JobKindGenerateNarration {
		t.Fatalf("publishing a moment must enqueue narration: %d → %d", len(before), len(after))
	}
	if r := env.do(t, http.MethodPut, "/api/v1/admin/moments/"+momentID+"/topics", curator, map[string]any{"topic_ids": []string{testdb.SeedTopicParksID, testdb.SeedTopicHousesID}}); r.status != 200 || len(r.body["topics"].([]any)) != 2 {
		t.Fatalf("topics: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPut, "/api/v1/admin/moments/"+momentID+"/sources", curator, map[string]any{"sources": []map[string]any{{"title": ""}}}); r.status != 422 {
		t.Fatalf("sources validation: %d", r.status)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/places/"+placeID, curator, map[string]any{"primary_moment_id": momentID}); r.status != 200 || r.body["primary_moment"] == nil {
		t.Fatalf("primary moment: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/places/"+placeID+"/publish", curator, nil); r.status != 200 {
		t.Fatalf("publish complete place: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/places/"+placeID, ""); r.status != 200 {
		t.Fatalf("complete place public: %d", r.status)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/places/"+placeID, curator, map[string]any{"primary_moment_id": testdb.SeedMomentTalkiesID}); r.status != 422 {
		t.Fatalf("primary moment from another place: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/moments/"+momentID, ""); r.status != 200 || len(r.body["media"].([]any)) != 1 {
		t.Fatalf("public moment: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/moments/"+momentID+"/unpublish", curator, nil); r.status != 200 || r.body["status"] != "draft" {
		t.Fatalf("unpublish: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodDelete, "/api/v1/admin/places/"+placeID, curator, nil); r.status != 200 || r.body["status"] != "archived" {
		t.Fatalf("archive place: %d %s", r.status, r.raw)
	}
	// Topics and cities.
	if r := env.do(t, http.MethodPost, "/api/v1/admin/topics", curator, map[string]any{"city_id": testdb.SeedCityID, "name": "Schools"}); r.status != 201 || r.body["slug"] != "schools" {
		t.Fatalf("create topic: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/cities", curator, map[string]any{"name": "Naperville", "center_lat": 41.77, "center_lng": -88.15, "bbox_min_lat": 41.7, "bbox_min_lng": -88.2, "bbox_max_lat": 41.8, "bbox_max_lng": -88.1}); r.status != 201 || r.body["slug"] != "naperville" {
		t.Fatalf("create city: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/admin/media", curator); r.status != 200 || len(items(t, r)) != seedMediaCount+1 {
		t.Fatalf("list media: %d %s", r.status, r.raw)
	}
}

func uploadMedia(t *testing.T, env *testEnv, token, filename, contentType string, data []byte, fields map[string]string) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	part, _ := mw.CreateFormFile("file", filename)
	_, _ = part.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/api/v1/admin/media", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 201 {
		t.Fatalf("upload: %d %s", res.StatusCode, raw)
	}
	var out map[string]any
	_ = jsonUnmarshal(raw, &out)
	_ = contentType
	return out
}

func TestAdminTourPublishing(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	r := env.do(t, http.MethodPost, "/api/v1/admin/tours", curator, map[string]any{"city_id": testdb.SeedCityID, "title": "Night Walk", "topic_id": testdb.SeedTopicAfterDarkID, "modes": []string{"walk"}})
	if r.status != 201 || r.body["status"] != "draft" || r.body["stop_count"].(float64) != 0 {
		t.Fatalf("create tour: %d %s", r.status, r.raw)
	}
	tourID := r.body["id"].(string)
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/"+tourID+"/publish", curator, nil); r.status != 409 || !strings.Contains(errMessage(r), "no stops") {
		t.Fatalf("publish empty tour: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPut, "/api/v1/admin/tours/"+tourID+"/stops", curator, map[string]any{"stops": []map[string]any{
		{"place_id": testdb.SeedPlaceTivoliID, "featured_moment_id": testdb.SeedMomentTalkiesID, "note": "Start under the marquee."},
		{"place_id": testdb.SeedPlaceFishelID},
	}})
	if r.status != 200 || len(r.body["stops"].([]any)) != 2 {
		t.Fatalf("set stops: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPost, "/api/v1/admin/tours/"+tourID+"/publish", curator, nil)
	if r.status != 409 || !strings.Contains(errMessage(r), "walk") || !strings.Contains(errMessage(r), "pending") {
		t.Fatalf("publish before routes: %d %s", r.status, r.raw)
	}
	env.drain(t)
	if failed := env.do(t, http.MethodPost, "/api/v1/admin/tours/"+tourID+"/publish", curator, nil); failed.status != 409 || !strings.Contains(errMessage(failed), "development_geometry") {
		t.Fatalf("development path published: %d %s", failed.status, failed.raw)
	}
	env.qualifyFixtureRoutes(t, tourID)
	r = env.do(t, http.MethodPost, "/api/v1/admin/tours/"+tourID+"/publish", curator, nil)
	if r.status != 200 || r.body["status"] != "published" {
		t.Fatalf("publish: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/cities/downers-grove/tours?topic=downtown-after-dark", ""); len(items(t, r)) != 1 {
		t.Fatalf("public listing after publish: %s", r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/"+tourID+"/routes/recompute", curator, nil); r.status != 202 {
		t.Fatalf("published working-copy recompute: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/tours/"+tourID, curator, map[string]any{"title": "Night Walk II"}); r.status != 200 || r.body["title"] != "Night Walk II" {
		t.Fatalf("title change on a published tour is fine: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/"+tourID+"/unpublish", curator, nil); r.status != 200 || r.body["status"] != "draft" {
		t.Fatalf("unpublish: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/"+tourID+"/routes/recompute", curator, nil); r.status != 202 {
		t.Fatalf("recompute: %d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/tours/"+tourID, curator)
	if r.body["routes"].(map[string]any)["walk"].(map[string]any)["status"] != "pending" {
		t.Fatalf("recompute should mark pending: %s", r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/tours/"+tourID, curator, map[string]any{"modes": []string{"walk", "bike"}}); r.status != 200 || len(r.body["modes"].([]any)) != 2 {
		t.Fatalf("patch modes on draft tour: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/admin/tours?city="+testdb.SeedCityID, curator); r.status != 200 || len(items(t, r)) != 2 {
		t.Fatalf("admin tour list: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodDelete, "/api/v1/admin/tours/"+tourID, curator, nil); r.status != 204 {
		t.Fatalf("delete tour: %d", r.status)
	}
}

func TestAdminReviewQueue(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	r := env.do(t, http.MethodPost, "/api/v1/admin/candidates", curator, map[string]any{
		"kind": "moment", "origin": "import", "source_name": "wikipedia", "external_id": "Q1", "target_place_id": testdb.SeedPlaceStationID,
		"payload": map[string]any{"title": "Station Fire", "body": "A fire damaged the depot.", "date_display": "1910"},
	})
	if r.status != 201 || r.body["status"] != "pending" {
		t.Fatalf("create candidate: %d %s", r.status, r.raw)
	}
	candID := r.body["id"].(string)
	if r := env.do(t, http.MethodPost, "/api/v1/admin/candidates", curator, map[string]any{"kind": "moment", "source_name": "wikipedia", "external_id": "Q1", "payload": map[string]any{}}); r.status != 409 {
		t.Fatalf("duplicate candidate: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/admin/candidates?status=pending&kind=moment", curator); len(items(t, r)) != 1 {
		t.Fatalf("list: %s", r.raw)
	}
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+candID+"/approve", curator, map[string]any{"note": "ok", "payload": map[string]any{"title": "Station Fire of 1910"}})
	if r.status != 200 || r.body["status"] != "approved" || r.body["approved_entity_id"] == nil {
		t.Fatalf("approve: %d %s", r.status, r.raw)
	}
	entityID := r.body["approved_entity_id"].(string)
	m, err := env.store.GetMoment(context.Background(), entityID, true)
	if err != nil || m.Title != "Station Fire of 1910" || m.Status != models.EntityStatusDraft || m.PlaceID != testdb.SeedPlaceStationID || m.Body != "A fire damaged the depot." {
		t.Fatalf("approved moment: %v %+v", err, m)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+candID+"/reject", curator, map[string]any{"note": "no"}); r.status != 409 {
		t.Fatalf("re-review: %d", r.status)
	}
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates", curator, map[string]any{"kind": "place", "payload": map[string]any{"city_id": testdb.SeedCityID, "name": "Prince Pond", "latitude": 41.8, "longitude": -88.0}})
	placeCand := r.body["id"].(string)
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+placeCand+"/reject", curator, map[string]any{"note": "duplicate of existing park"})
	if r.status != 200 || r.body["status"] != "rejected" || r.body["review_note"] != "duplicate of existing park" {
		t.Fatalf("reject: %d %s", r.status, r.raw)
	}
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates", curator, map[string]any{"kind": "place", "payload": map[string]any{"city_id": testdb.SeedCityID, "name": "Prince Pond", "latitude": 41.8, "longitude": -88.0}})
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+r.body["id"].(string)+"/approve", curator, nil)
	if r.status != 200 {
		t.Fatalf("approve place: %d %s", r.status, r.raw)
	}
	if p, err := env.store.GetPlace(context.Background(), r.body["approved_entity_id"].(string), "", true); err != nil || p.Slug != "prince-pond" {
		t.Fatalf("approved place: %v %+v", err, p)
	}
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates", curator, map[string]any{"kind": "moment", "payload": map[string]any{"title": "No place"}})
	r = env.do(t, http.MethodPost, "/api/v1/admin/candidates/"+r.body["id"].(string)+"/approve", curator, nil)
	if r.status != 422 {
		t.Fatalf("approving a moment candidate without a target place must fail validation: %d %s", r.status, r.raw)
	}
}

func TestAdminJobsAndAccounts(t *testing.T) {
	env := newTestEnv(t)
	admin := env.signIn(t, "admin@example.com", "")
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	r := env.get(t, "/api/v1/admin/jobs?status=queued", curator)
	if r.status != 200 || len(items(t, r)) != 2 {
		t.Fatalf("queued jobs: %d %s", r.status, r.raw)
	}
	jobID := items(t, r)[0].(map[string]any)["id"].(string)
	if r := env.do(t, http.MethodPost, "/api/v1/admin/jobs/"+jobID+"/retry", curator, nil); r.status != 404 {
		t.Fatalf("retrying a queued job: %d", r.status)
	}
	if err := env.store.FailJob(context.Background(), jobID, "boom", nil); err != nil {
		t.Fatal(err)
	}
	if r := env.get(t, "/api/v1/admin/jobs?status=failed", curator); len(items(t, r)) != 1 {
		t.Fatalf("failed jobs: %s", r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/jobs/"+jobID+"/retry", curator, nil); r.status != 200 || r.body["status"] != "queued" {
		t.Fatalf("retry: %d %s", r.status, r.raw)
	}

	r = env.get(t, "/api/v1/admin/accounts", admin)
	if r.status != 200 || len(items(t, r)) != 2 {
		t.Fatalf("accounts: %d %s", r.status, r.raw)
	}
	var curatorID string
	for _, a := range items(t, r) {
		if a.(map[string]any)["email"] == "c@example.com" {
			curatorID = a.(map[string]any)["id"].(string)
		}
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/accounts/"+curatorID+"/role", curator, map[string]any{"role": "admin"}); r.status != 403 {
		t.Fatalf("curator promoting self: %d", r.status)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/accounts/"+curatorID+"/role", admin, map[string]any{"role": "god"}); r.status != 422 {
		t.Fatalf("bad role: %d", r.status)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/accounts/"+curatorID+"/role", admin, map[string]any{"role": "visitor"}); r.status != 200 || r.body["role"] != "visitor" {
		t.Fatalf("demote: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/admin/places", curator); r.status != 403 {
		t.Fatalf("demoted curator should lose access: %d", r.status)
	}
}
