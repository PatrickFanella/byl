package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
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
	for _, mode := range models.AllTravelModes {
		rs := tour.Routes[mode]
		if rs.Status != models.RouteStatusReady || rs.EstimatedMinutes == nil || rs.DistanceM < 1800 {
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
	if len(blockers) != 2 {
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
	w.finish(cctx, job, errors.New("interrupted"))
	got, _ := store.GetJob(ctx, job.ID)
	if got.Status != models.JobStatusQueued {
		t.Fatalf("interrupted job should be requeued, got %+v", got)
	}
}
