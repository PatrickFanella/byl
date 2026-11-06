package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestHealthz(t *testing.T) {
	env := newTestEnv(t)
	if r := env.get(t, "/healthz", ""); r.status != 200 || r.body["status"] != "ok" {
		t.Fatalf("%d %s", r.status, r.raw)
	}
}

func TestPilotPlacesHaveIllustratedSourcedHistories(t *testing.T) {
	env := newTestEnv(t)
	for _, id := range []string{testdb.SeedPlaceStationID, testdb.SeedPlaceTivoliID, testdb.SeedPlaceFishelID, testdb.SeedPlaceMuseumID} {
		place, err := env.store.GetPlace(context.Background(), id, "", false)
		if err != nil || place.PrimaryMoment == nil {
			t.Fatalf("place history missing: %s %v", id, err)
		}
		for _, summary := range place.Moments {
			moment, err := env.store.GetMoment(context.Background(), summary.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			media := append([]models.MediaAsset{}, moment.Media...)
			if moment.HeroMedia != nil {
				media = append(media, *moment.HeroMedia)
			}
			if gaps := models.HistoryReviewGaps(moment.Body, moment.Sources, media); len(gaps) != 0 {
				t.Fatalf("%s is incomplete: %v", moment.Title, gaps)
			}
		}
	}
	for _, file := range []string{"main-street-station", "tivoli-theatre", "fishel-school", "blodgett-family"} {
		r := env.get(t, "/media/files/library/images/"+file+".jpg", "")
		if r.status != 200 || len(r.raw) < 10000 || r.raw[0] != 0xff || r.raw[1] != 0xd8 {
			t.Fatalf("packaged image unavailable: %s status=%d bytes=%d", file, r.status, len(r.raw))
		}
	}
	if r := env.get(t, "/media/files/library/images/", ""); r.status != 404 {
		t.Fatal("image directory listing exposed")
	}
}

func TestCityEndpoints(t *testing.T) {
	env := newTestEnv(t)
	r := env.get(t, "/api/v1/cities", "")
	if r.status != 200 || len(items(t, r)) != 1 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/cities/downers-grove", "")
	if r.status != 200 || len(r.body["topics"].([]any)) != 4 || r.body["slug"] != "downers-grove" {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/cities/atlantis", ""); r.status != 404 || errCode(r) != "not_found" {
		t.Fatalf("%d %s", r.status, r.raw)
	}
}

func TestPlacesNearAndFilters(t *testing.T) {
	env := newTestEnv(t)
	r := env.get(t, "/api/v1/cities/downers-grove/places?near=41.7959,-88.0092&radius_m=300", "")
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	list := items(t, r)
	if len(list) != 3 {
		t.Fatalf("expected 3 places, got %d", len(list))
	}
	prev := -1.0
	for _, it := range list {
		d := it.(map[string]any)["distance_m"].(float64)
		if d < prev {
			t.Fatalf("not sorted by distance: %s", r.raw)
		}
		prev = d
	}
	if r := env.get(t, "/api/v1/cities/downers-grove/places?near=bogus", ""); r.status != 422 || errCode(r) != "validation_failed" {
		t.Fatalf("bad near: %d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/cities/downers-grove/places?topic=houses-and-builders", "")
	if r.status != 200 || len(items(t, r)) != 1 {
		t.Fatalf("topic slug filter: %d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/cities/downers-grove/places?bbox=-88.02,41.79,-88.00,41.80", "")
	if r.status != 200 || len(items(t, r)) != 3 {
		t.Fatalf("bbox: %d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/cities/downers-grove/places?limit=2", "")
	if r.status != 200 || len(items(t, r)) != 2 || r.body["next_cursor"] == "" {
		t.Fatalf("pagination: %d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/cities/downers-grove/places?limit=2&cursor="+r.body["next_cursor"].(string), "")
	if r.status != 200 || len(items(t, r)) != 2 || r.body["next_cursor"] != "" {
		t.Fatalf("second page: %d %s", r.status, r.raw)
	}
}

func TestPlaceAndMomentDetail(t *testing.T) {
	env := newTestEnv(t)
	r := env.get(t, "/api/v1/places/"+testdb.SeedPlaceMuseumID, "")
	if r.status != 200 || len(r.body["moments"].([]any)) != 2 || r.body["primary_moment"] == nil {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/places/downers-grove-museum?city=downers-grove", "")
	if r.status != 200 || r.body["id"] != testdb.SeedPlaceMuseumID {
		t.Fatalf("by slug: %d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/places/"+testdb.SeedPlaceStationID+"/moments", "")
	if r.status != 200 || len(items(t, r)) != 3 {
		t.Fatalf("moments: %d %s", r.status, r.raw)
	}
	r = env.get(t, "/api/v1/moments/"+testdb.SeedMomentTalkiesID, "")
	if r.status != 200 || len(r.body["sources"].([]any)) != 2 || r.body["body"] == "" || r.body["narration"] != nil {
		t.Fatalf("moment: %d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/moments/"+testdb.SeedMomentDownerID, ""); r.status != 404 {
		t.Fatalf("in_review moment must be hidden: %d", r.status)
	}
	if err := env.store.SetPlaceStatus(context.Background(), testdb.SeedPlaceFishelID, models.EntityStatusDraft); err != nil {
		t.Fatal(err)
	}
	if r := env.get(t, "/api/v1/places/"+testdb.SeedPlaceFishelID, ""); r.status != 404 {
		t.Fatalf("draft place must be hidden: %d", r.status)
	}
	if r := env.get(t, "/api/v1/places/"+testdb.SeedPlaceFishelID+"/moments", ""); r.status != 404 {
		t.Fatalf("draft place moments must be hidden: %d", r.status)
	}
	r = env.get(t, "/api/v1/topics/"+testdb.SeedTopicRailID, "")
	if r.status != 200 || r.body["moment_count"].(float64) != 4 {
		t.Fatalf("topic: %d %s", r.status, r.raw)
	}
}

func TestTourEndpoints(t *testing.T) {
	env := newTestEnv(t)
	r := env.get(t, "/api/v1/cities/downers-grove/tours", "")
	if r.status != 200 || len(items(t, r)) != 1 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	if r := env.get(t, "/api/v1/cities/downers-grove/tours?topic=parks-and-public-life", ""); len(items(t, r)) != 0 {
		t.Fatalf("topic filter: %s", r.raw)
	}
	if r := env.get(t, "/api/v1/cities/downers-grove/tours?mode=fly", ""); r.status != 422 {
		t.Fatalf("bad mode: %d", r.status)
	}
	r = env.get(t, "/api/v1/tours/downtown-memory-loop", "")
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.raw)
	}
	stops := r.body["stops"].([]any)
	if len(stops) != 4 {
		t.Fatalf("stops: %s", r.raw)
	}
	for _, s := range stops {
		if s.(map[string]any)["featured_moment"] == nil {
			t.Fatalf("every stop needs a featured moment: %s", r.raw)
		}
	}
	routes := r.body["routes"].(map[string]any)
	if routes["walk"].(map[string]any)["status"] != "pending" {
		t.Fatalf("routes before worker: %v", routes)
	}
	r = env.get(t, "/api/v1/tours/downtown-memory-loop/route?mode=walk", "")
	if r.status != 200 || r.body["status"] != "pending" || r.body["geometry"] != nil {
		t.Fatalf("pending route: %d %s", r.status, r.raw)
	}
	env.drain(t)
	r = env.get(t, "/api/v1/tours/downtown-memory-loop/route?mode=walk", "")
	if r.status != 200 || r.body["status"] != "ready" {
		t.Fatalf("ready route: %d %s", r.status, r.raw)
	}
	geom := r.body["geometry"].(map[string]any)
	if geom["type"] != "LineString" || len(geom["coordinates"].([]any)) != 4 || len(r.body["legs"].([]any)) != 3 {
		t.Fatalf("geometry: %s", r.raw)
	}
	if r := env.get(t, "/api/v1/tours/downtown-memory-loop/route?mode=hover", ""); r.status != 422 {
		t.Fatalf("bad mode: %d", r.status)
	}
	r = env.get(t, "/api/v1/tours/downtown-memory-loop", "")
	if r.body["routes"].(map[string]any)["bike"].(map[string]any)["estimated_minutes"] == nil {
		t.Fatalf("estimate after worker: %s", r.raw)
	}
}

func TestTourVisibility(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner, _ := env.store.UpsertAccount(ctx, models.AuthProviderDev, "o", "o@example.com", "", false)
	mk := func(vis models.TourVisibility, status models.EntityStatus) string {
		tour, err := env.store.CreateTour(ctx, repository_CreateTourInput(vis, status, owner.ID))
		if err != nil {
			t.Fatal(err)
		}
		return tour.Slug
	}
	unlisted := mk(models.TourVisibilityUnlisted, models.EntityStatusPublished)
	private := mk(models.TourVisibilityPrivate, models.EntityStatusPublished)
	draft := mk(models.TourVisibilityPublic, models.EntityStatusDraft)

	if r := env.get(t, "/api/v1/tours/"+unlisted, ""); r.status != 200 {
		t.Fatalf("unlisted by link: %d", r.status)
	}
	if r := env.get(t, "/api/v1/tours/"+private, ""); r.status != 404 {
		t.Fatalf("private anonymous: %d", r.status)
	}
	if r := env.get(t, "/api/v1/tours/"+draft, ""); r.status != 404 {
		t.Fatalf("draft anonymous: %d", r.status)
	}
	if r := env.get(t, "/api/v1/cities/downers-grove/tours", ""); len(items(t, r)) != 1 {
		t.Fatalf("public list must hide unlisted/private/draft: %s", r.raw)
	}
	curator := env.signIn(t, "c@example.com", models.AccountRoleCurator)
	if r := env.get(t, "/api/v1/tours/"+draft, curator); r.status != 200 {
		t.Fatalf("curator sees draft: %d", r.status)
	}
	if r := env.get(t, "/api/v1/tours/"+private, curator); r.status != 404 {
		t.Fatalf("curator does not see someone's private route: %d", r.status)
	}
}

func TestMediaFilesServed(t *testing.T) {
	env := newTestEnv(t)
	if err := env.blob.Put(context.Background(), "images/x.txt", "text/plain", stringsReader("hello"), 5); err != nil {
		t.Fatal(err)
	}
	r := env.get(t, "/media/files/images/x.txt", "")
	if r.status != http.StatusOK || string(r.raw) != "hello" {
		t.Fatalf("%d %q", r.status, r.raw)
	}
	if r := env.get(t, "/media/files/../go.mod", ""); r.status == 200 && len(r.raw) > 0 && string(r.raw[:6]) == "module" {
		t.Fatal("traversal served a repo file")
	}
	// Review fix: stored files are served on the admin origin; never let the browser sniff them.
	if r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing nosniff header: %v", r.header)
	}
}

// Review fix (cycle 3): shared route links are {base}/t/{slug}; the API answers with a landing
// page that hands off to the app's URL scheme instead of a 404.
func TestShareLinkLandingPage(t *testing.T) {
	env := newTestEnv(t)
	r := env.get(t, "/t/abc123XYZ0", "")
	if r.status != http.StatusOK || !strings.HasPrefix(r.header.Get("Content-Type"), "text/html") {
		t.Fatalf("%d %q", r.status, r.header.Get("Content-Type"))
	}
	if !strings.Contains(string(r.raw), `href="invisiblemuseum://tour/abc123XYZ0"`) ||
		!strings.Contains(string(r.raw), `content="0; url=invisiblemuseum://tour/abc123XYZ0"`) {
		t.Fatalf("landing page must link to the app scheme: %s", r.raw)
	}
	if r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	if r := env.get(t, "/t/%3Cscript%3E", ""); r.status != http.StatusNotFound {
		t.Fatalf("invalid slug should be 404, got %d %s", r.status, r.raw)
	}
}
