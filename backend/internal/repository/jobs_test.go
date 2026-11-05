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
