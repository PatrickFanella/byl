package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/testdb"
)

// Finding 1: unpublished featured moments must not leak through a public tour read.
func TestPublicTourHidesUnpublishedFeaturedMoment(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	stops := []models.StopInput{
		{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: testdb.SeedPlaceTivoliID},
		{PlaceID: testdb.SeedPlaceFishelID}, {PlaceID: testdb.SeedPlaceMuseumID, FeaturedMomentID: ptrS(testdb.SeedMomentDownerID)},
	}
	if err := env.store.ReplaceStops(ctx, testdb.SeedTourID, stops); err != nil {
		t.Fatal(err)
	}
	r := env.get(t, "/api/v1/tours/downtown-memory-loop", "")
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	last := r.body["stops"].([]any)[3].(map[string]any)
	fm, _ := last["featured_moment"].(map[string]any)
	if fm == nil || fm["id"] == testdb.SeedMomentDownerID || fm["status"] != "published" {
		t.Fatalf("public read must fall back to the published primary moment: %v", last)
	}
	blockers, _ := env.store.TourPublishBlockers(ctx, testdb.SeedTourID)
	if !strings.Contains(strings.Join(blockers, ";"), "featured moment") {
		t.Fatalf("publish blockers must name the unpublished featured moment: %v", blockers)
	}
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	r = env.get(t, "/api/v1/admin/tours/downtown-memory-loop", curator)
	if r.body["stops"].([]any)[3].(map[string]any)["featured_moment"].(map[string]any)["id"] != testdb.SeedMomentDownerID {
		t.Fatalf("admin read keeps the curator's choice: %s", r.raw)
	}
}

// Finding 1 and 2: a place on a published curated tour cannot be unpublished or archived.
func TestUnpublishPlaceOnPublishedTourRefused(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	for _, path := range []string{"/api/v1/admin/places/" + testdb.SeedPlaceFishelID + "/unpublish"} {
		r := env.do(t, http.MethodPost, path, curator, nil)
		if r.status != 409 || !strings.Contains(errMessage(r), "downtown-memory-loop") {
			t.Fatalf("%s: %d %s", path, r.status, r.raw)
		}
	}
	if r := env.do(t, http.MethodDelete, "/api/v1/admin/places/"+testdb.SeedPlaceFishelID, curator, nil); r.status != 409 {
		t.Fatalf("archive: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/"+testdb.SeedTourID+"/unpublish", curator, nil); r.status != 200 {
		t.Fatalf("unpublish tour: %d", r.status)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/places/"+testdb.SeedPlaceFishelID+"/unpublish", curator, nil); r.status != 200 {
		t.Fatalf("unpublish place after tour is draft: %d %s", r.status, r.raw)
	}
}

// Published working-copy edits cannot replace the released geometry or stops.
func TestStopChangesKeepPublishedCuratedSnapshot(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	env.drain(t)
	body := map[string]any{"stops": []map[string]any{{"place_id": testdb.SeedPlaceStationID}, {"place_id": testdb.SeedPlaceMuseumID}}}
	if r := env.do(t, http.MethodPut, "/api/v1/admin/tours/"+testdb.SeedTourID+"/stops", curator, body); r.status != 200 {
		t.Fatalf("stops on published tour: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/"+testdb.SeedTourID+"/routes/recompute", curator, nil); r.status != 202 {
		t.Fatalf("recompute on published tour: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/tours/"+testdb.SeedTourID, curator, map[string]any{"modes": []string{"walk"}}); r.status != 200 {
		t.Fatalf("mode change on published tour: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/tours/downtown-memory-loop", ""); len(r.body["stops"].([]any)) != 4 || r.body["routes"].(map[string]any)["walk"].(map[string]any)["status"] != "ready" {
		t.Fatalf("published tour must keep its geometry: %s", r.raw)
	}
	env.do(t, http.MethodPost, "/api/v1/admin/tours/"+testdb.SeedTourID+"/unpublish", curator, nil)
	if r := env.do(t, http.MethodPut, "/api/v1/admin/tours/"+testdb.SeedTourID+"/stops", curator, body); r.status != 200 {
		t.Fatalf("stops on draft tour: %d %s", r.status, r.raw)
	}
	me := env.signIn(t, "me@example.com", "")
	r := env.do(t, http.MethodPost, "/api/v1/me/tours", me, userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID))
	id := r.body["id"].(string)
	if r := env.do(t, http.MethodPatch, "/api/v1/me/tours/"+id, me, map[string]any{"stops": []map[string]any{{"place_id": testdb.SeedPlaceTivoliID}, {"place_id": testdb.SeedPlaceMuseumID}}}); r.status != 200 {
		t.Fatalf("user routes stay editable: %d %s", r.status, r.raw)
	}
}

// Finding 2: a published moment cannot lose its media licence.
func TestLicenceRemovalOnPublishedMomentRefused(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	licensedAsset := uploadMedia(t, env, curator, "a.jpg", "image/jpeg", []byte("\xff\xd8\xffjpeg"), map[string]string{"license": "CC0-1.0", "caption": "Archive photograph", "credit": "Archive", "source_url": "https://example.org/photo", "license_url": "https://creativecommons.org/publicdomain/zero/1.0/"})
	unlicensedAsset := uploadMedia(t, env, curator, "b.jpg", "image/jpeg", []byte("\xff\xd8\xffjpeg"), nil)
	licensed, unlicensed := licensedAsset["id"].(string), unlicensedAsset["id"].(string)
	moment := "/api/v1/admin/moments/" + testdb.SeedMomentTalkiesID
	if r := env.do(t, http.MethodPut, moment+"/media", curator, map[string]any{"media_ids": []string{licensed}}); r.status != 200 {
		t.Fatalf("attach licensed: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPut, moment+"/media", curator, map[string]any{"media_ids": []string{licensed, unlicensed}}); r.status != 409 {
		t.Fatalf("attach unlicensed to published moment: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/media/"+licensed, curator, map[string]any{"license": ""}); r.status != 409 {
		t.Fatalf("clearing licence on attached asset: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/media/"+unlicensed, curator, map[string]any{"license": ""}); r.status != 200 {
		t.Fatalf("clearing licence on an unattached asset is fine: %d %s", r.status, r.raw)
	}
}

// Finding 3: saved tours respect visibility.
func TestSavedHidesPrivateAndUnpublishedTours(t *testing.T) {
	env := newTestEnv(t)
	alice := env.signIn(t, "alice@example.com", "")
	bob := env.signIn(t, "bob@example.com", "")
	r := env.do(t, http.MethodPost, "/api/v1/me/tours", alice, userRoute(testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID))
	slug, id := r.body["slug"].(string), r.body["id"].(string)
	if r := env.do(t, http.MethodPut, "/api/v1/me/saved/"+slug, bob, nil); r.status != 204 {
		t.Fatalf("save: %d", r.status)
	}
	if r := env.get(t, "/api/v1/me/saved", bob); len(items(t, r)) != 1 {
		t.Fatalf("saved: %s", r.raw)
	}
	env.do(t, http.MethodPatch, "/api/v1/me/tours/"+id, alice, map[string]any{"visibility": "private", "title": "Alice secret plans"})
	if r := env.get(t, "/api/v1/me/saved", bob); len(items(t, r)) != 0 {
		t.Fatalf("private route must vanish from Bob's saved list: %s", r.raw)
	}
	if r := env.get(t, "/api/v1/me/saved", alice); len(items(t, r)) != 0 {
		t.Fatalf("alice never saved it: %s", r.raw)
	}
	_ = env.do(t, http.MethodPut, "/api/v1/me/saved/"+slug, alice, nil)
	if r := env.get(t, "/api/v1/me/saved", alice); len(items(t, r)) != 1 {
		t.Fatalf("owner sees own private route in saved: %s", r.raw)
	}
	if err := env.store.SetTourStatus(context.Background(), testdb.SeedTourID, models.EntityStatusDraft); err != nil {
		t.Fatal(err)
	}
	_ = env.do(t, http.MethodPut, "/api/v1/me/saved/downtown-memory-loop", bob, nil)
	if r := env.get(t, "/api/v1/me/saved", bob); len(items(t, r)) != 0 {
		t.Fatalf("unpublished curated tour must not show in saved: %s", r.raw)
	}
}

// Finding 5: admin mutations accept slugs and never 500 on them.
func TestAdminMutationsAcceptSlugs(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/downtown-memory-loop/publish", curator, nil); r.status != 200 && r.status != 409 {
		t.Fatalf("publish by slug: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/downtown-memory-loop/unpublish", curator, nil); r.status != 200 {
		t.Fatalf("unpublish by slug: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/downtown-memory-loop/routes/recompute", curator, nil); r.status != 202 {
		t.Fatalf("recompute by slug: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/tours/downtown-memory-loop", curator, map[string]any{"summary": "x"}); r.status != 200 {
		t.Fatalf("patch by slug: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/places/fishel-park/unpublish", curator, nil); r.status != 200 {
		t.Fatalf("place by slug: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/places/fishel-park", curator, map[string]any{"summary": "y"}); r.status != 200 {
		t.Fatalf("patch place by slug: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPost, "/api/v1/admin/tours/nope/publish", curator, nil); r.status != 404 {
		t.Fatalf("unknown slug: %d %s", r.status, r.raw)
	}
}

// Finding 8: stops in another city are rejected.
func TestUserRouteRejectsStopsFromAnotherCity(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	city, err := env.store.CreateCity(ctx, models.CreateCityRequest{Name: "Naperville", CenterLat: 41.77, CenterLng: -88.15, BBoxMinLat: 41.7, BBoxMinLng: -88.2, BBoxMaxLat: 41.8, BBoxMaxLng: -88.1})
	if err != nil {
		t.Fatal(err)
	}
	place, err := env.store.CreatePlace(ctx, models.CreatePlaceRequest{CityID: city.ID, Name: "Riverwalk", Latitude: 41.77, Longitude: -88.15})
	if err != nil {
		t.Fatal(err)
	}
	_ = env.store.SetPlaceStatus(ctx, place.ID, models.EntityStatusPublished)
	me := env.signIn(t, "me@example.com", "")
	r := env.do(t, http.MethodPost, "/api/v1/me/tours", me, userRoute(testdb.SeedPlaceStationID, place.ID))
	if r.status != 422 {
		t.Fatalf("cross-city stops: %d %s", r.status, r.raw)
	}
	fields, _ := r.body["error"].(map[string]any)["fields"].(map[string]any)
	if fields["stops"] == nil {
		t.Fatalf("expected stops field error: %s", r.raw)
	}
}

// Minors promoted: negative offsets must not 500; media directories must not list.
func TestNegativeOffsetAndDirectoryListing(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	for _, p := range []string{"/api/v1/admin/jobs?offset=-1", "/api/v1/admin/media?offset=-5", "/api/v1/admin/accounts?offset=-1"} {
		tok := curator
		if strings.Contains(p, "accounts") {
			tok = env.signIn(t, "admin@example.com", "")
		}
		if r := env.get(t, p, tok); r.status != 200 {
			t.Fatalf("%s: %d %s", p, r.status, r.raw)
		}
	}
	if err := env.blob.Put(context.Background(), "images/x.txt", "text/plain", stringsReader("hello"), 5); err != nil {
		t.Fatal(err)
	}
	if r := env.get(t, "/media/files/images/", ""); r.status != 404 {
		t.Fatalf("directory listing: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/media/files/images/x.txt", ""); r.status != 200 {
		t.Fatalf("file still served: %d", r.status)
	}
}

func ptrS(s string) *string { return &s }

var _ = repository.ErrNotFound
