package repository

import (
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
)

func TestClaimJobHandsOutEachJobOnce(t *testing.T) {
	s, ctx := newStore(t)
	now := time.Now()
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		j, err := s.ClaimJob(ctx, now)
		if err != nil || j == nil {
			t.Fatalf("claim %d: %v %v", i, err, j)
		}
		if j.Status != models.JobStatusRunning || j.Attempts != 1 || seen[j.ID] {
			t.Fatalf("claimed job: %+v", j)
		}
		seen[j.ID] = true
	}
	if j, err := s.ClaimJob(ctx, now); err != nil || j != nil {
		t.Fatalf("third claim should find nothing: %v %v", err, j)
	}
}

func TestFailJobRetriesThenFails(t *testing.T) {
	s, ctx := newStore(t)
	if err := s.Enqueue(ctx, "noop", map[string]string{"x": "y"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	var last *models.Job
	for attempt := 1; attempt <= 3; attempt++ {
		var j *models.Job
		for {
			var err error
			j, err = s.ClaimJob(ctx, now.Add(time.Hour*24))
			if err != nil {
				t.Fatal(err)
			}
			if j == nil {
				t.Fatalf("attempt %d: nothing to claim", attempt)
			}
			if j.Kind == "noop" {
				break
			}
			_ = s.CompleteJob(ctx, j.ID)
		}
		if j.Attempts != attempt {
			t.Fatalf("attempts = %d want %d", j.Attempts, attempt)
		}
		var retry *time.Time
		if j.Attempts < j.MaxAttempts {
			at := now.Add(time.Duration(attempt) * time.Minute)
			retry = &at
		}
		if err := s.FailJob(ctx, j.ID, "boom", retry); err != nil {
			t.Fatal(err)
		}
		last, _ = s.GetJob(ctx, j.ID)
		if attempt < 3 && (last.Status != models.JobStatusQueued || last.RunAfter.Before(now.Add(time.Duration(attempt)*time.Minute-time.Second))) {
			t.Fatalf("attempt %d: expected requeued later, got %+v", attempt, last)
		}
	}
	if last.Status != models.JobStatusFailed || last.LastError != "boom" || last.FinishedAt == nil {
		t.Fatalf("final: %+v", last)
	}
	failed, err := s.ListJobs(ctx, models.JobStatusFailed, 10, 0)
	if err != nil || len(failed) != 1 {
		t.Fatalf("failed list: %v %d", err, len(failed))
	}
	if err := s.RetryJob(ctx, last.ID); err != nil {
		t.Fatal(err)
	}
	re, _ := s.GetJob(ctx, last.ID)
	if re.Status != models.JobStatusQueued || re.Attempts != 0 {
		t.Fatalf("retry should reset: %+v", re)
	}
	if err := s.CompleteJob(ctx, last.ID); err != nil {
		t.Fatal(err)
	}
	done, _ := s.GetJob(ctx, last.ID)
	if done.Status != models.JobStatusDone || done.FinishedAt == nil {
		t.Fatalf("complete: %+v", done)
	}
}

func TestReclaimStaleJobsFailsJobsThatUsedEveryAttempt(t *testing.T) {
	s, ctx := newStore(t)
	if _, err := s.pool.Exec(ctx, `DELETE FROM jobs`); err != nil {
		t.Fatal(err)
	}
	stale := func(kind string, attempts int) string {
		t.Helper()
		j, err := s.EnqueueReturning(ctx, kind, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `UPDATE jobs SET status='running', attempts=$2, started_at=now()-interval '1 hour' WHERE id=$1`, j.ID, attempts); err != nil {
			t.Fatal(err)
		}
		return j.ID
	}
	exhausted, retrying, fenced := stale("noop", 3), stale("noop", 1), stale(models.JobKindResearchBrief, 3)
	if n, err := s.ReclaimStaleJobs(ctx, 10*time.Minute); err != nil || n != 3 {
		t.Fatalf("reclaim: %d %v", n, err)
	}
	for id, want := range map[string]models.JobStatus{exhausted: models.JobStatusFailed, retrying: models.JobStatusQueued, fenced: models.JobStatusQueued} {
		j, err := s.GetJob(ctx, id)
		if err != nil || j.Status != want {
			t.Fatalf("job %s: got %+v %v, want %s", id, j, err, want)
		}
	}
}

func TestFinishClaimedJobIgnoresAReclaimedClaim(t *testing.T) {
	s, ctx := newStore(t)
	if _, err := s.pool.Exec(ctx, `DELETE FROM jobs`); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue(ctx, "noop", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	first, err := s.ClaimJob(ctx, time.Now())
	if err != nil || first == nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE jobs SET started_at=now()-interval '1 hour' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReclaimStaleJobs(ctx, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	second, err := s.ClaimJob(ctx, time.Now())
	if err != nil || second == nil || second.ID != first.ID {
		t.Fatalf("reclaimed job was not claimed again: %+v %v", second, err)
	}
	if err := s.FinishClaimedJob(ctx, first, "stale run", nil, true); err != nil {
		t.Fatal(err)
	}
	if j, _ := s.GetJob(ctx, first.ID); j.Status != models.JobStatusRunning {
		t.Fatalf("the stale run overwrote the newer claim: %+v", j)
	}
}
