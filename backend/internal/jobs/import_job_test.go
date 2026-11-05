package jobs

import (
	"context"
	"encoding/json"
	"invisible-museum/backend/internal/blob"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/sources"
	"invisible-museum/backend/internal/testdb"
)

type fakeAdapter struct {
	finds []sources.Found
	calls int
	last  sources.Request
}

func (f *fakeAdapter) Name() string { return "fake" }
func (f *fakeAdapter) Discover(_ context.Context, req sources.Request) ([]sources.Found, error) {
	f.calls++
	f.last = req
	return f.finds, nil
}

func fl(v float64) *float64 { return &v }

func fakeFinds() []sources.Found {
	return []sources.Found{
		{Kind: models.CandidateKindPlace, ExternalID: "fake:p1", Title: "Lyman Woods", Text: "A preserve.", URL: "https://x/p1", Lat: fl(41.82), Lng: fl(-87.99), License: "CC-BY-SA-4.0", Credit: "Wikipedia contributors"},
		{Kind: models.CandidateKindMoment, ExternalID: "fake:m1", Title: "Station Fire", Text: "A fire damaged the depot in 1910.", URL: "https://x/m1", Lat: fl(41.7960), Lng: fl(-88.0093), License: "CC-BY-SA-4.0", Credit: "Wikipedia contributors"},
		{Kind: models.CandidateKindMoment, ExternalID: "fake:m1", Title: "Station Fire (dup)", Text: "dup", URL: "https://x/m1"},
		{Kind: models.CandidateKindMedia, ExternalID: "fake:i1", Title: "Depot.jpg", Text: "The depot", URL: "https://x/i1", MediaURL: "https://x/files/depot.jpg", MimeType: "image/jpeg", Width: 100, Height: 80, License: "CC-BY-2.0", Credit: "Someone"},
	}
}

func candidatesBySource(t *testing.T, store *repository.Store, source string) []models.Candidate {
	t.Helper()
	cands, _, err := store.ListCandidates(context.Background(), repository.CandidateFilter{Source: source, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return cands
}

func TestImportJobCreatesCandidatesAndSkipsDuplicates(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx) // clear seed route jobs
	fake := &fakeAdapter{finds: fakeFinds()}
	reg := sources.NewRegistry(sources.NewHTTPClient("ua"), nil)
	reg.Register(fake, sources.Info{SupportsNear: true})
	w.Sources = reg

	if err := store.Enqueue(ctx, models.JobKindImportSource, models.ImportSourcePayload{Source: "fake", CityID: testdb.SeedCityID, RadiusM: 1500}); err != nil {
		t.Fatal(err)
	}
	if n := drain(t, w, ctx); n != 1 {
		t.Fatalf("processed %d", n)
	}
	if fake.last.Center.Lat == 0 || fake.last.CityID != testdb.SeedCityID {
		t.Fatalf("adapter request should carry the city centre and id: %+v", fake.last)
	}
	cands := candidatesBySource(t, store, "fake")
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates (duplicate external id collapsed), got %d", len(cands))
	}
	var moment models.Candidate
	for _, c := range cands {
		if c.Origin != models.CandidateOriginImport || c.Status != models.CandidateStatusPending || c.SourceName == nil || *c.SourceName != "fake" {
			t.Fatalf("candidate %+v", c)
		}
		if c.Kind == models.CandidateKindMoment {
			moment = c
		}
	}
	if moment.TargetPlaceID == nil || *moment.TargetPlaceID != testdb.SeedPlaceStationID {
		t.Fatalf("moment within 150 m of the station should target it: %+v", moment)
	}
	var payload map[string]any
	_ = json.Unmarshal(moment.Payload, &payload)
	if payload["raw_text"] != "A fire damaged the depot in 1910." || payload["source_url"] != "https://x/m1" || payload["source_license"] != "CC-BY-SA-4.0" || payload["place_id"] != testdb.SeedPlaceStationID {
		t.Fatalf("moment payload %v", payload)
	}

	jobs, _ := store.ListJobs(ctx, models.JobStatusDone, 10, 0)
	var note string
	for _, j := range jobs {
		if j.Kind == models.JobKindImportSource {
			note = j.LastError
		}
	}
	if !strings.Contains(note, "imported 3") || !strings.Contains(note, "skipped 1") {
		t.Fatalf("job note %q", note)
	}

	// Second run: everything already exists.
	_ = store.Enqueue(ctx, models.JobKindImportSource, models.ImportSourcePayload{Source: "fake", CityID: testdb.SeedCityID})
	drain(t, w, ctx)
	if len(candidatesBySource(t, store, "fake")) != 3 {
		t.Fatal("second import must not duplicate candidates")
	}
}

func TestImportJobUsesPlaceAsCentreAndTarget(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	fake := &fakeAdapter{finds: []sources.Found{{Kind: models.CandidateKindMoment, ExternalID: "fake:far", Title: "Far away", Text: "text", URL: "https://x/far", Lat: fl(41.9), Lng: fl(-87.6)}}}
	reg := sources.NewRegistry(sources.NewHTTPClient("ua"), nil)
	reg.Register(fake, sources.Info{SupportsNear: true})
	w.Sources = reg
	place := testdb.SeedPlaceTivoliID
	_ = store.Enqueue(ctx, models.JobKindImportSource, models.ImportSourcePayload{Source: "fake", CityID: testdb.SeedCityID, PlaceID: &place})
	drain(t, w, ctx)
	if fake.last.Center.Lat < 41.79 || fake.last.Center.Lat > 41.80 {
		t.Fatalf("centre should be the place: %+v", fake.last.Center)
	}
	cands := candidatesBySource(t, store, "fake")
	if len(cands) != 1 || cands[0].TargetPlaceID == nil || *cands[0].TargetPlaceID != place {
		t.Fatalf("explicit place wins over distance: %+v", cands)
	}
	_ = store.Enqueue(ctx, models.JobKindImportSource, models.ImportSourcePayload{Source: "nope", CityID: testdb.SeedCityID})
	drain(t, w, ctx)
	failed, _ := store.ListJobs(ctx, models.JobStatusFailed, 10, 0)
	if len(failed) != 1 || !strings.Contains(failed[0].LastError, "unknown source") {
		t.Fatalf("unknown source must fail without retry: %+v", failed)
	}
}

func TestFetchMediaJobDownloadsIntoBlob(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nfake"))
	}))
	defer srv.Close()
	w.Sources = sources.NewRegistry(sources.NewHTTPClient("ua"), nil)
	w.Sources.Client().AllowPrivateHosts = true // the test server is on loopback
	payload, _ := json.Marshal(map[string]any{"kind": "image", "media_url": srv.URL + "/depot.png", "caption": "The depot", "license": "CC-BY-2.0"})
	cand, err := store.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindMedia, Origin: models.CandidateOriginImport, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := store.CreateMedia(ctx, models.MediaAsset{Kind: models.MediaKindImage, Caption: "The depot", License: "CC-BY-2.0"})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Enqueue(ctx, models.JobKindFetchMedia, models.FetchMediaPayload{CandidateID: cand.ID, AssetID: asset.ID})
	if n := drain(t, w, ctx); n != 1 {
		t.Fatalf("processed %d", n)
	}
	got, _ := store.GetMedia(ctx, asset.ID)
	if got.StorageKey == "" || !strings.HasPrefix(got.PublicURL, "http://media.test/") || got.MimeType != "image/png" {
		t.Fatalf("asset after fetch: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(w.blob.(*blob.FS).Dir(), filepath.FromSlash(got.StorageKey))); err != nil {
		t.Fatalf("file not stored: %v", err)
	}
	_ = geo.Point{}
}

// mediaFetchFixture enqueues a fetch_media job for an image asset whose media_url is served by handler.
func mediaFetchFixture(t *testing.T, w *Worker, store *repository.Store, ctx context.Context, path string, handler http.HandlerFunc, allowPrivate bool) (*httptest.Server, string, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	w.Sources = sources.NewRegistry(sources.NewHTTPClient("ua"), nil)
	w.Sources.Client().AllowPrivateHosts = allowPrivate
	payload, _ := json.Marshal(map[string]any{"kind": "image", "media_url": srv.URL + path, "caption": "x", "license": "CC0-1.0"})
	cand, err := store.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindMedia, Origin: models.CandidateOriginImport, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := store.CreateMedia(ctx, models.MediaAsset{Kind: models.MediaKindImage, Caption: "x", License: "CC0-1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(ctx, models.JobKindFetchMedia, models.FetchMediaPayload{CandidateID: cand.ID, AssetID: asset.ID}); err != nil {
		t.Fatal(err)
	}
	return srv, cand.ID, asset.ID
}

func onlyJob(t *testing.T, store *repository.Store, ctx context.Context, status models.JobStatus) models.Job {
	t.Helper()
	jobs, err := store.ListJobs(ctx, status, 10, 0)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("expected one %s job, got %d (%v)", status, len(jobs), err)
	}
	return jobs[0]
}

// Review fix: media_url comes from the candidate payload, which any curator controls; the
// download must not reach private networks.
func TestFetchMediaJobRefusesPrivateHosts(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	hits := 0
	_, _, assetID := mediaFetchFixture(t, w, store, ctx, "/latest/meta-data", func(rw http.ResponseWriter, _ *http.Request) {
		hits++
		rw.Header().Set("Content-Type", "image/png")
		_, _ = rw.Write([]byte("secret"))
	}, false)
	drain(t, w, ctx)
	if hits != 0 {
		t.Fatal("the private host was contacted")
	}
	job := onlyJob(t, store, ctx, models.JobStatusFailed)
	if job.Attempts != 1 || !strings.Contains(job.LastError, "private") {
		t.Fatalf("should fail once without retry: %+v", job)
	}
	if got, _ := store.GetMedia(ctx, assetID); got.StorageKey != "" {
		t.Fatalf("asset must stay unfetched: %+v", got)
	}
}

// Review fix: a response that is not an image/video/audio matching the asset kind must be
// refused; an HTML body stored under /media/files would run on the admin origin.
func TestFetchMediaJobRefusesNonMediaContent(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	_, _, assetID := mediaFetchFixture(t, w, store, ctx, "/x-image.jpg", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = rw.Write([]byte("<script>alert(1)</script>"))
	}, true)
	drain(t, w, ctx)
	job := onlyJob(t, store, ctx, models.JobStatusFailed)
	if job.Attempts != 1 || !strings.Contains(job.LastError, "text/html") {
		t.Fatalf("should fail once naming the content type: %+v", job)
	}
	if got, _ := store.GetMedia(ctx, assetID); got.StorageKey != "" {
		t.Fatalf("asset must stay unfetched: %+v", got)
	}
	entries, _ := os.ReadDir(w.blob.(*blob.FS).Dir())
	if len(entries) != 0 {
		t.Fatalf("nothing should be stored, found %v", entries)
	}
}

func TestFetchMediaJobRefusesKindMismatch(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	mediaFetchFixture(t, w, store, ctx, "/clip.mp4", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "video/mp4")
		_, _ = rw.Write([]byte("mp4"))
	}, true)
	drain(t, w, ctx)
	job := onlyJob(t, store, ctx, models.JobStatusFailed)
	if job.Attempts != 1 || !strings.Contains(job.LastError, "video/mp4") {
		t.Fatalf("an image asset must not accept video: %+v", job)
	}
}

// Review fix: the stored key carries an extension from the media type so the file server
// never has to sniff.
func TestFetchMediaJobForcesExtensionFromContentType(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	_, _, assetID := mediaFetchFixture(t, w, store, ctx, "/download/item", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "image/jpeg")
		_, _ = rw.Write([]byte("ÿØÿ"))
	}, true)
	drain(t, w, ctx)
	got, _ := store.GetMedia(ctx, assetID)
	if !strings.HasSuffix(got.StorageKey, ".jpg") || got.MimeType != "image/jpeg" {
		t.Fatalf("asset after fetch: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(w.blob.(*blob.FS).Dir(), filepath.FromSlash(got.StorageKey))); err != nil {
		t.Fatalf("file not stored: %v", err)
	}
}

// Review fix (cycle 3): SVG is image/* but can carry script; stored on the admin origin it
// would run with the curator's token, so it is refused.
func TestFetchMediaJobRefusesSVG(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	_, _, assetID := mediaFetchFixture(t, w, store, ctx, "/logo.svg", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "image/svg+xml")
		_, _ = rw.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	}, true)
	drain(t, w, ctx)
	job := onlyJob(t, store, ctx, models.JobStatusFailed)
	if job.Attempts != 1 || !strings.Contains(job.LastError, "svg") {
		t.Fatalf("svg must be refused without retry: %+v", job)
	}
	if got, _ := store.GetMedia(ctx, assetID); got.StorageKey != "" {
		t.Fatalf("asset must stay unfetched: %+v", got)
	}
}

func TestFetchMediaJobRefusesOversizedResponse(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	_, _, assetID := mediaFetchFixture(t, w, store, ctx, "/oversized.png", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "image/png")
		rw.Header().Set("Content-Length", "67108865")
		rw.WriteHeader(http.StatusOK)
	}, true)
	drain(t, w, ctx)
	job := onlyJob(t, store, ctx, models.JobStatusFailed)
	if job.Attempts != 1 || !strings.Contains(job.LastError, "exceeds") {
		t.Fatalf("oversized media must fail without retry: %+v", job)
	}
	if got, _ := store.GetMedia(ctx, assetID); got.StorageKey != "" {
		t.Fatal("oversized media must not be attached")
	}
}
