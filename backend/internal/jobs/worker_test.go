package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/sources"
	"invisible-museum/backend/internal/testdb"
)

type failingRouter struct{}

func (failingRouter) Route(context.Context, models.TravelMode, []routing.Point) (routing.Route, error) {
	return routing.Route{}, errors.New("no path could be found")
}

type fakeNarrator struct{ calls int }

func (f *fakeNarrator) Synthesize(_ context.Context, text string) (*narration.Audio, error) {
	f.calls++
	return &narration.Audio{Data: []byte("mp3:" + text[:8]), MimeType: "audio/mpeg"}, nil
}

func newWorker(t *testing.T, router routing.Router, narrator narration.Narrator) (*Worker, *repository.Store, context.Context) {
	t.Helper()
	store := repository.New(testdb.Open(t))
	fs, err := blob.NewFS(t.TempDir(), "http://media.test")
	if err != nil {
		t.Fatal(err)
	}
	w := New(store, router, narrator, fs, 1, quiet())
	return w, store, context.Background()
}

func drain(t *testing.T, w *Worker, ctx context.Context) int {
	t.Helper()
	n := 0
	for {
		processed, err := w.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !processed {
			return n
		}
		n++
	}
}

func TestComputeRouteJobsProduceReadyRoutes(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	if n := drain(t, w, ctx); n != 2 {
		t.Fatalf("expected 2 seed jobs processed, got %d", n)
	}
	tour, err := store.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	// The corrected Museum pin makes this development-polyline itinerary about
	// 921 m; the old 1.8 km floor depended on the misplaced seed coordinate.
	for _, mode := range models.AllTravelModes {
		rs := tour.Routes[mode]
		if rs.Status != models.RouteStatusReady || rs.EstimatedMinutes == nil || math.Abs(rs.DistanceM-920.6) > 1 {
			t.Fatalf("%s route: %+v", mode, rs)
		}
		full, err := store.GetTourRoute(ctx, testdb.SeedTourID, mode)
		if err != nil || len(full.Legs) != 3 || full.Geometry == nil || full.Geometry.Type != "LineString" || len(full.Geometry.Coordinates) != 4 {
			t.Fatalf("%s full route: %v %+v", mode, err, full)
		}
		if full.Legs[0].FromPosition != 1 || full.Legs[0].ToPosition != 2 || full.Provider != "straightline" {
			t.Fatalf("legs: %+v", full.Legs[0])
		}
	}
	if tour.Routes[models.TravelModeBike].DurationS >= tour.Routes[models.TravelModeWalk].DurationS {
		t.Fatal("bike should be faster than walk")
	}
	jobs, _ := store.ListJobs(ctx, models.JobStatusDone, 10, 0)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 done jobs, got %d", len(jobs))
	}
}

func TestRouteJobFailsAfterMaxAttempts(t *testing.T) {
	w, store, ctx := newWorker(t, failingRouter{}, narration.None{})
	w.backoff = func(int) time.Duration { return 0 } // make every retry claimable at once
	processed := drain(t, w, ctx)
	if processed != 6 {
		t.Fatalf("expected 2 jobs x 3 attempts = 6 runs, got %d", processed)
	}
	route, err := store.GetTourRoute(ctx, testdb.SeedTourID, models.TravelModeWalk)
	if err != nil {
		t.Fatal(err)
	}
	if route.Status != models.RouteStatusFailed || route.Error != "no path could be found" {
		t.Fatalf("route: %+v", route)
	}
	failed, _ := store.ListJobs(ctx, models.JobStatusFailed, 10, 0)
	if len(failed) != 2 || failed[0].Attempts != 3 || failed[0].LastError != "no path could be found" {
		t.Fatalf("failed jobs: %+v", failed)
	}
	blockers, _ := store.TourPublishBlockers(ctx, testdb.SeedTourID)
	if len(blockers) != 4 {
		t.Fatalf("tour must stay unpublishable: %v", blockers)
	}
}

func TestRouteJobRetriesWithBackoff(t *testing.T) {
	w, store, ctx := newWorker(t, failingRouter{}, narration.None{})
	if processed := drain(t, w, ctx); processed != 2 {
		t.Fatalf("first pass should run each job once, got %d", processed)
	}
	queued, _ := store.ListJobs(ctx, models.JobStatusQueued, 10, 0)
	if len(queued) != 2 {
		t.Fatalf("jobs should be requeued, got %d queued", len(queued))
	}
	if until := time.Until(queued[0].RunAfter); until < 25*time.Second || until > 35*time.Second {
		t.Fatalf("first retry should be ~30s out, got %v", until)
	}
	route, _ := store.GetTourRoute(ctx, testdb.SeedTourID, models.TravelModeWalk)
	if route.Status != models.RouteStatusPending {
		t.Fatalf("route should stay pending while retrying: %+v", route)
	}
}

func TestNarrationJobs(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	if err := store.Enqueue(ctx, models.JobKindGenerateNarration, models.GenerateNarrationPayload{MomentID: testdb.SeedMomentTalkiesID}); err != nil {
		t.Fatal(err)
	}
	if n := drain(t, w, ctx); n != 1 {
		t.Fatalf("expected 1 job, got %d", n)
	}
	m, _ := store.GetMoment(ctx, testdb.SeedMomentTalkiesID, true)
	if m.HasNarration || m.Narration != nil {
		t.Fatal("none narrator must not attach audio")
	}

	fake := &fakeNarrator{}
	w2 := New(store, routing.NewStraightLine(), fake, w.blob, 1, quiet())
	_ = store.Enqueue(ctx, models.JobKindGenerateNarration, models.GenerateNarrationPayload{MomentID: testdb.SeedMomentTalkiesID})
	drain(t, w2, ctx)
	m, _ = store.GetMoment(ctx, testdb.SeedMomentTalkiesID, true)
	if !m.HasNarration || m.Narration == nil || m.Narration.Kind != models.MediaKindAudio || m.Narration.License == "" || m.Narration.PublicURL == "" {
		t.Fatalf("narration asset: %+v", m.Narration)
	}
	if fake.calls != 1 {
		t.Fatalf("narrator calls %d", fake.calls)
	}

	_ = store.Enqueue(ctx, models.JobKindGenerateNarration, models.GenerateNarrationPayload{MomentID: "00000000-0000-4000-8000-00000000ffff"})
	w2.backoff = func(int) time.Duration { return 0 }
	drain(t, w2, ctx)
	failed, _ := store.ListJobs(ctx, models.JobStatusFailed, 10, 0)
	if len(failed) != 1 {
		t.Fatalf("unknown moment should fail the job, got %d failed", len(failed))
	}
}

func TestPlainTextStripsMarkdown(t *testing.T) {
	got := plainText("# Title\n\nSome **bold** and _italic_ text with a [link](http://x).\n\n- item")
	if got != "Title\n\nSome bold and italic text with a link.\n\nitem" {
		t.Fatalf("plainText = %q", got)
	}
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Finding 4: jobs left running by a crashed or restarted worker are requeued.
func TestStaleRunningJobsAreRequeued(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	pool := testdbPool(t, store)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status = 'running', started_at = now() - interval '1 hour', attempts = 1`); err != nil {
		t.Fatal(err)
	}
	if n := drain(t, w, ctx); n != 2 {
		t.Fatalf("stale running jobs should be reclaimed, processed %d", n)
	}
	done, _ := store.ListJobs(ctx, models.JobStatusDone, 10, 0)
	if len(done) != 2 {
		t.Fatalf("expected 2 done, got %d", len(done))
	}
	// A job that just started must not be stolen.
	_ = store.Enqueue(ctx, "noop", map[string]string{})
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status = 'running', started_at = now() WHERE kind = 'noop'`); err != nil {
		t.Fatal(err)
	}
	if n := drain(t, w, ctx); n != 0 {
		t.Fatalf("fresh running job must not be reclaimed, processed %d", n)
	}
}

// Finding 4: completion is recorded even when the request context is already cancelled.
func TestCompletionSurvivesCancelledContext(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	cctx, cancel := context.WithCancel(ctx)
	job, err := store.ClaimJob(ctx, time.Now())
	if err != nil || job == nil {
		t.Fatal(err)
	}
	cancel()
	_ = w.finish(cctx, job, "", errors.New("interrupted"))
	got, _ := store.GetJob(ctx, job.ID)
	if got.Status != models.JobStatusQueued {
		t.Fatalf("interrupted job should be requeued, got %+v", got)
	}
}

// Review fix: the import job's completion note must travel with its own job. A note parked
// on the worker could be picked up by whichever job finished next on another goroutine.
func TestImportNoteStaysWithItsJob(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	reg := sources.NewRegistry(sources.NewHTTPClient("ua"), nil)
	reg.Register(&fakeAdapter{finds: fakeFinds()}, sources.Info{SupportsNear: true})
	w.Sources = reg
	if err := store.Enqueue(ctx, models.JobKindImportSource, models.ImportSourcePayload{Source: "fake", CityID: testdb.SeedCityID}); err != nil {
		t.Fatal(err)
	}
	imp, err := store.ClaimJob(ctx, w.now())
	if err != nil || imp == nil {
		t.Fatalf("claim import: %v %v", imp, err)
	}
	note, err := w.dispatch(ctx, imp)
	if err != nil || !strings.Contains(note, "imported 3") {
		t.Fatalf("dispatch should return the note: %q %v", note, err)
	}
	// Another job completes on a sibling worker before the import job is finished.
	_ = store.Enqueue(ctx, models.JobKindComputeRoute, models.ComputeRoutePayload{TourID: testdb.SeedTourID, Mode: models.TravelModeWalk})
	if n := drain(t, w, ctx); n != 1 {
		t.Fatalf("processed %d", n)
	}
	done, _ := store.ListJobs(ctx, models.JobStatusDone, 10, 0)
	for _, j := range done {
		if j.Kind == models.JobKindComputeRoute && j.LastError != "" {
			t.Fatalf("route job carries the import note: %q", j.LastError)
		}
	}
	if err := w.finish(ctx, imp, note, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetJob(ctx, imp.ID)
	if got.Status != models.JobStatusDone || got.LastError != note {
		t.Fatalf("import job after finish: %+v", got)
	}
}

func TestNarrationLinkFailureLeavesNoAssetOrFile(t *testing.T) {
	store := repository.New(testdb.Open(t))
	dir := t.TempDir()
	fs, err := blob.NewFS(dir, "http://media.test")
	if err != nil {
		t.Fatal(err)
	}
	w := New(store, routing.NewStraightLine(), &fakeNarrator{}, fs, 1, quiet())
	ctx := context.Background()
	drain(t, w, ctx)
	pool := testdbPool(t, store)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION refuse_narration_link() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic link failure'; END $$;
		CREATE TRIGGER refuse_narration_link BEFORE UPDATE OF narration_media_id ON moments FOR EACH ROW EXECUTE FUNCTION refuse_narration_link()`); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM media_assets WHERE kind = 'audio'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(ctx, models.JobKindGenerateNarration, models.GenerateNarrationPayload{MomentID: testdb.SeedMomentTalkiesID}); err != nil {
		t.Fatal(err)
	}
	w.backoff = func(int) time.Duration { return 0 }
	drain(t, w, ctx)
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM media_assets WHERE kind = 'audio'`).Scan(&after); err != nil || after != before {
		t.Fatalf("failed links left %d audio assets behind (%v)", after-before, err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "narration", "*"))
	if len(files) != 0 {
		t.Fatalf("failed links left narration files: %v", files)
	}
}
