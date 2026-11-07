package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/sources"
)

type evidenceFunc func(context.Context, string, string) (models.EvidenceResult, error)

func (f evidenceFunc) RetrieveEvidence(ctx context.Context, u, k string) (models.EvidenceResult, error) {
	return f(ctx, u, k)
}

func evidenceSetup(t *testing.T) (*Worker, *repository.Store, context.Context, *models.SourceEvidence) {
	t.Helper()
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "evidence", "evidence@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Rail workers", Area: "Downers Grove"}, "")
	if err != nil {
		t.Fatal(err)
	}
	u, reference, key, err := sources.NormalizeEvidenceURL("https://archive.example/history?access=private-token#page=3")
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.QueueSourceEvidence(ctx, b.ID, models.EvidenceRequest{Version: 1, URL: u, SourceKind: "page"}, reference, key, "one-request")
	if err != nil {
		t.Fatal(err)
	}
	return w, s, ctx, e
}

func evidenceResult() models.EvidenceResult {
	return models.EvidenceResult{Outcome: "inspected_page", FinalURL: "https://archive.example/history", Publisher: "archive.example", Title: "Depot record", HTTPStatus: 200, ContentType: "text/html", ContentSHA256: strings.Repeat("a", 64), Passages: []models.EvidencePassage{{Reference: "paragraph:1", Text: "Private inspected source passage."}}}
}

func TestEvidenceSurvivesWorkerRestartAndKeepsItsSnapshot(t *testing.T) {
	w, s, ctx, e := evidenceSetup(t)
	job, err := s.GetJob(ctx, e.JobID)
	if err != nil || strings.Contains(string(job.Payload), "private-token") || strings.Contains(string(job.Payload), "archive.example") {
		t.Fatalf("source URL leaked to job: %v", err)
	}
	// A new store/worker instance reads the persisted request, not in-memory state.
	w2 := New(repository.New(s.Pool()), w.router, w.narrator, w.blob, 1, quiet())
	calls := 0
	w2.Evidence = evidenceFunc(func(_ context.Context, url, kind string) (models.EvidenceResult, error) {
		calls++
		if url != e.URL || kind != "page" {
			t.Fatal("request lost provenance")
		}
		return evidenceResult(), nil
	})
	drain(t, w2, ctx)
	saved, err := s.GetSourceEvidence(ctx, e.ID)
	if err != nil || saved.Result == nil || saved.Reference != "page=3" || saved.SourceKey != e.SourceKey || saved.Result.MediaReuse != "unresolved" || !saved.Result.Untrusted {
		t.Fatalf("checkpoint: %+v %v", saved, err)
	}
	before, err := json.Marshal(saved.Result)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET status='queued',run_after=now() WHERE id=$1`, e.JobID); err != nil {
		t.Fatal(err)
	}
	w3 := New(repository.New(s.Pool()), w.router, w.narrator, w.blob, 1, quiet())
	w3.Evidence = evidenceFunc(func(context.Context, string, string) (models.EvidenceResult, error) {
		t.Fatal("completed evidence fetched again")
		return models.EvidenceResult{}, nil
	})
	drain(t, w3, ctx)
	got, err := s.GetSourceEvidence(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(got.Result)
	if err != nil || string(before) != string(after) || calls != 1 {
		t.Fatal("snapshot changed after restart")
	}
	if got.BriefVersion != 1 {
		t.Fatal("source lost original brief revision")
	}
}

func TestEvidenceInterruptedJobResumesWithoutStrandingRequest(t *testing.T) {
	w, s, ctx, e := evidenceSetup(t)
	stopping, cancel := context.WithCancel(ctx)
	defer cancel()
	w.Evidence = evidenceFunc(func(context.Context, string, string) (models.EvidenceResult, error) {
		cancel()
		return models.EvidenceResult{}, context.Canceled
	})
	if _, err := w.RunOnce(stopping); err != nil {
		t.Fatal(err)
	}
	job, err := s.GetJob(ctx, e.JobID)
	if err != nil || job.Status != models.JobStatusQueued {
		t.Fatalf("interrupted request stranded: %+v %v", job, err)
	}
	w.Evidence = evidenceFunc(func(context.Context, string, string) (models.EvidenceResult, error) { return evidenceResult(), nil })
	drain(t, w, ctx)
	saved, err := s.GetSourceEvidence(ctx, e.ID)
	if err != nil || saved.Result == nil || saved.Result.Outcome != "inspected_page" {
		t.Fatalf("resume: %+v %v", saved, err)
	}
}

func TestEvidenceStaleClaimCannotReplaceOrCompleteNewerWork(t *testing.T) {
	w, s, ctx, e := evidenceSetup(t)
	old, err := s.ClaimJob(ctx, time.Now())
	if err != nil || old == nil {
		t.Fatal(err)
	}
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET started_at=now()-interval '1 hour' WHERE id=$1`, old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReclaimStaleJobs(ctx, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	current, err := s.ClaimJob(ctx, time.Now())
	if err != nil || current == nil {
		t.Fatal(err)
	}
	if err := s.CompleteSourceEvidence(ctx, e.ID, old, evidenceResult()); !errors.Is(err, repository.ErrResearchInactive) {
		t.Fatalf("stale evidence committed: %v", err)
	}
	if err := w.finish(ctx, old, "old completion", nil); err != nil {
		t.Fatal(err)
	}
	job, err := s.GetJob(ctx, current.ID)
	if err != nil || job.Status != models.JobStatusRunning {
		t.Fatalf("stale finish stole current claim: %+v %v", job, err)
	}
	if err := s.CompleteSourceEvidence(ctx, e.ID, current, evidenceResult()); err != nil {
		t.Fatal(err)
	}
	if err := w.finish(ctx, current, "evidence_checkpointed", nil); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceFailuresRemainExplicitGapsAndRespectAttemptLimit(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		attempts      int
	}{
		{"access gap", "inaccessible", 0}, {"budget exhausted", "worker_failed", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, s, ctx, e := evidenceSetup(t)
			if tc.attempts > 0 {
				if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET attempts=$2 WHERE id=$1`, e.JobID, tc.attempts); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			w.Evidence = evidenceFunc(func(context.Context, string, string) (models.EvidenceResult, error) {
				calls++
				return models.EvidenceResult{Outcome: "inaccessible", HTTPStatus: 403}, nil
			})
			drain(t, w, ctx)
			saved, err := s.GetSourceEvidence(ctx, e.ID)
			if err != nil || saved.Result == nil || saved.Result.Outcome != tc.outcome || len(saved.Result.Passages) != 0 {
				t.Fatalf("failure became source evidence: %+v %v", saved, err)
			}
			if tc.attempts > 0 && calls != 0 {
				t.Fatal("exhausted job made another external request")
			}
		})
	}
}
