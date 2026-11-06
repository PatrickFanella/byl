package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
)

type researchFunc func(context.Context, ResearchTask) (models.ResearchOutput, error)

func (f researchFunc) Process(ctx context.Context, task ResearchTask) (models.ResearchOutput, error) {
	return f(ctx, task)
}

func researchSetup(t *testing.T) (*Worker, *repository.Store, context.Context, *models.ResearchRun) {
	t.Helper()
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "research", "research@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Workers", Area: "Downers Grove", Notes: "Private submitted notes"}, "")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.StartBriefResearch(ctx, b.ID, b.Version)
	if err != nil {
		t.Fatal(err)
	}
	w.backoff = func(int) time.Duration { return 0 }
	return w, s, ctx, r
}

func stageResult(task ResearchTask) models.ResearchOutput {
	out := models.ResearchOutput{Data: json.RawMessage(`{"private_source_excerpt":"not for the progress API"}`)}
	if task.Stage == "research" {
		out.Candidates = []models.Candidate{{Kind: models.CandidateKindPlace, Payload: json.RawMessage(`{"name":"Unverified depot"}`)}}
	}
	return out
}

func TestResearchRetriesKeepCheckpointsAndCuratorEdits(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	calls := map[string]int{}
	var logs bytes.Buffer
	w.log = slog.New(slog.NewTextHandler(&logs, nil))
	w.Research = researchFunc(func(callCtx context.Context, task ResearchTask) (models.ResearchOutput, error) {
		calls[task.Stage]++
		if deadline, ok := callCtx.Deadline(); !ok || time.Until(deadline) > 30*time.Second {
			t.Fatal("missing bounded stage deadline")
		}
		if task.MaxTokens != repository.ResearchTokenReservation {
			t.Fatal("missing model ceiling")
		}
		if task.Stage == "reconciliation" && calls[task.Stage] == 1 {
			return models.ResearchOutput{}, errors.New("credential=never-log-this raw source excerpt")
		}
		if task.Stage != "research" && len(task.Previous["research"]) == 0 {
			t.Fatal("missing saved stage input")
		}
		return stageResult(task), nil
	})
	if processed, err := w.RunOnce(ctx); !processed || err != nil {
		t.Fatalf("first run: %v %v", processed, err)
	}
	checkpoint, err := s.GetResearchRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Stages[0].Status != "completed" || len(checkpoint.Stages[0].CandidateIDs) != 1 {
		t.Fatalf("checkpoint: %+v", checkpoint)
	}
	id := checkpoint.Stages[0].CandidateIDs[0]
	if err := s.UpdateCandidatePayload(ctx, id, json.RawMessage(`{"name":"Curator corrected depot"}`), nil); err != nil {
		t.Fatal(err)
	}
	drain(t, w, ctx)
	final, err := s.GetResearchRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != "completed" || calls["research"] != 1 || calls["reconciliation"] != 2 || calls["drafting"] != 1 || calls["assembly"] != 1 {
		t.Fatalf("resume: %+v calls=%v", final, calls)
	}
	if len(final.Stages[1].Outcomes) != 2 || final.Stages[1].Outcomes[0].Reason != "provider_failure" || final.Stages[1].Outcomes[1].Status != "completed" {
		t.Fatalf("lost failure history: %+v", final.Stages[1])
	}
	c, err := s.GetCandidate(ctx, id)
	if err != nil || !strings.Contains(string(c.Payload), "Curator corrected") {
		t.Fatalf("curator edits lost: %+v %v", c, err)
	}
	var count int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM candidates WHERE source_name='brief_research'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate candidates: %d", count)
	}
	b, err := s.GetRouteBrief(ctx, run.BriefID, "")
	if err != nil || b.Status != models.BriefReview || b.Submitted.Notes != "Private submitted notes" {
		t.Fatalf("brief disposition: %+v %v", b, err)
	}
	raw, err := json.Marshal(final)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private_source_excerpt", "Private submitted notes", "never-log-this", "raw source excerpt"} {
		if strings.Contains(string(raw), private) || strings.Contains(logs.String(), private) {
			t.Fatalf("private data leaked: %s", private)
		}
	}
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM tours WHERE status='published'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("research published content: %d %v", count, err)
	}
}

func TestResearchCancellationFencesOldOutputAndResumesWithBudgets(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	oldJob, err := s.ClaimJob(ctx, time.Now())
	if err != nil || oldJob == nil {
		t.Fatalf("claim: %v", err)
	}
	if started, err := s.BeginResearchStage(ctx, run.ID, "research", oldJob); err != nil || !started {
		t.Fatalf("begin: %v %v", started, err)
	}
	if _, err := s.ChangeBriefResearch(ctx, run.BriefID, models.ResearchActionRequest{Version: run.BriefVersion, RunID: run.ID}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitResearchStage(ctx, run.ID, "research", oldJob, stageResult(ResearchTask{Stage: "research"})); !errors.Is(err, repository.ErrResearchInactive) {
		t.Fatalf("cancelled write: %v", err)
	}
	b, err := s.GetRouteBrief(ctx, run.BriefID, "")
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := s.ChangeBriefResearch(ctx, run.BriefID, models.ResearchActionRequest{Version: b.Version, RunID: run.ID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.JobID == oldJob.ID || resumed.RequestsUsed != 1 || resumed.TokensReserved != 2000 || !resumed.DeadlineAt.Equal(run.DeadlineAt) {
		t.Fatalf("resume reset safety bounds: %+v", resumed)
	}
	if err := s.CommitResearchStage(ctx, run.ID, "research", oldJob, stageResult(ResearchTask{Stage: "research"})); !errors.Is(err, repository.ErrResearchInactive) {
		t.Fatalf("old job after resume: %v", err)
	}
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		return stageResult(task), nil
	})
	drain(t, w, ctx)
	final, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || final.Status != "completed" || final.RequestsUsed != 5 || len(final.Stages[0].Outcomes) != 2 || final.Stages[0].Outcomes[0].Status != "cancelled" {
		t.Fatalf("resumed run: %+v %v", final, err)
	}
}

func TestResearchGapResumePreservesPartialCandidateEdits(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		out := stageResult(task)
		out.NeedsResearch = "insufficient_evidence"
		return out, nil
	})
	drain(t, w, ctx)
	gap, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || gap.Stages[0].Status != "needs_research" || len(gap.Stages[0].CandidateIDs) != 1 {
		t.Fatalf("gap checkpoint: %+v %v", gap, err)
	}
	id := gap.Stages[0].CandidateIDs[0]
	if err := s.UpdateCandidatePayload(ctx, id, json.RawMessage(`{"name":"Curator enriched candidate"}`), nil); err != nil {
		t.Fatal(err)
	}
	b, err := s.GetRouteBrief(ctx, run.BriefID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeBriefResearch(ctx, b.ID, models.ResearchActionRequest{Version: b.Version, RunID: run.ID}, true); err != nil {
		t.Fatal(err)
	}
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		return stageResult(task), nil
	})
	drain(t, w, ctx)
	final, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || final.Status != "completed" || len(final.Stages[0].CandidateIDs) != 1 || final.Stages[0].CandidateIDs[0] != id {
		t.Fatalf("gap replay duplicated candidate: %+v %v", final, err)
	}
	candidate, err := s.GetCandidate(ctx, id)
	if err != nil || !strings.Contains(string(candidate.Payload), "Curator enriched") {
		t.Fatalf("gap replay discarded curator edit: %+v %v", candidate, err)
	}
}

func TestResearchWorkerInterruptionResumesCompletedStages(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		if task.Stage == "reconciliation" {
			cancel()
			return models.ResearchOutput{}, context.Canceled
		}
		return stageResult(task), nil
	})
	if _, err := w.RunOnce(cancelled); err != nil {
		t.Fatal(err)
	}
	job, err := s.GetJob(ctx, run.JobID)
	if err != nil || job.Status != models.JobStatusQueued {
		t.Fatalf("interrupted job stranded: %+v %v", job, err)
	}
	calls := map[string]int{}
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		calls[task.Stage]++
		return stageResult(task), nil
	})
	drain(t, w, ctx)
	final, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || final.Status != "completed" || calls["research"] != 0 || calls["reconciliation"] != 1 {
		t.Fatalf("restart reran completed work: %+v %v %v", final, calls, err)
	}
	if final.Stages[1].Outcomes[0].Status != "interrupted" {
		t.Fatalf("interruption lost: %+v", final.Stages[1])
	}
}

func TestResearchStaleClaimCannotCommitOrFinishNewClaim(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	old, err := s.ClaimJob(ctx, time.Now())
	if err != nil || old == nil {
		t.Fatal(err)
	}
	if _, err := s.BeginResearchStage(ctx, run.ID, "research", old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET started_at=now()-interval '1 hour' WHERE id=$1`, old.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ReclaimStaleJobs(ctx, 10*time.Minute); err != nil || n != 1 {
		t.Fatalf("reclaim: %d %v", n, err)
	}
	current, err := s.ClaimJob(ctx, time.Now())
	if err != nil || current == nil {
		t.Fatal(err)
	}
	if err := s.CommitResearchStage(ctx, run.ID, "research", old, stageResult(ResearchTask{Stage: "research"})); !errors.Is(err, repository.ErrResearchInactive) {
		t.Fatalf("stale output: %v", err)
	}
	if err := w.finish(ctx, old, "stale success", nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetJob(ctx, current.ID)
	if err != nil || got.Status != models.JobStatusRunning || got.Attempts != current.Attempts {
		t.Fatalf("stale completion stole claim: %+v %v", got, err)
	}
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		return stageResult(task), nil
	})
	note, err := w.dispatch(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.finish(ctx, current, note, nil); err != nil {
		t.Fatal(err)
	}
	final, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || final.Status != "completed" || final.Stages[0].Attempts != 2 {
		t.Fatalf("stale recovery: %+v %v", final, err)
	}
}

func TestResearchBudgetExhaustion(t *testing.T) {
	for _, tc := range []struct {
		name, sql, reason string
		fail              bool
	}{
		{"requests", `UPDATE brief_research_runs SET request_limit=1 WHERE id=$1`, "request_budget", false},
		{"model", `UPDATE brief_research_runs SET token_limit=1999 WHERE id=$1`, "model_budget", false},
		{"elapsed", `UPDATE brief_research_runs SET deadline_at=now()-interval '1 second' WHERE id=$1`, "elapsed_budget", false},
		{"retries", `UPDATE brief_research_runs SET request_limit=12 WHERE id=$1`, "retry_budget", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, s, ctx, run := researchSetup(t)
			if _, err := s.Pool().Exec(ctx, tc.sql, run.ID); err != nil {
				t.Fatal(err)
			}
			calls := 0
			w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
				calls++
				if tc.fail {
					return models.ResearchOutput{}, ResearchFailure{Code: "source_unavailable"}
				}
				return stageResult(task), nil
			})
			drain(t, w, ctx)
			final, err := s.GetResearchRun(ctx, run.ID)
			if err != nil || final.Status != "needs_research" || final.Reason != tc.reason {
				t.Fatalf("budget: %+v %v", final, err)
			}
			b, err := s.GetRouteBrief(ctx, run.BriefID, "")
			if err != nil || b.Status != models.BriefNeedsResearch || !strings.HasPrefix(b.Reason, tc.reason+":") {
				t.Fatalf("stranded brief: %+v %v", b, err)
			}
			if tc.fail && calls != 3 {
				t.Fatalf("unbounded retries: %d", calls)
			}
			if _, err := s.ChangeBriefResearch(ctx, run.BriefID, models.ResearchActionRequest{Version: b.Version, RunID: run.ID}, true); !errors.Is(err, repository.ErrConflict) {
				t.Fatalf("resume reset exhausted budget: %v", err)
			}
		})
	}
}

func TestResearchDoesNotOverwriteChangedBrief(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		b, err := s.SetRouteBriefState(ctx, run.BriefID, models.BriefStateRequest{Version: run.BriefVersion, Status: models.BriefNeedsResearch, Reason: "Curator paused research"})
		if err != nil {
			t.Fatal(err)
		}
		b.Submitted.Notes = "Curator revised scope"
		if _, err := s.UpdateRouteBrief(ctx, b.ID, "", models.UpdateRouteBriefRequest{Version: b.Version, Submitted: b.Submitted}); err != nil {
			t.Fatal(err)
		}
		return stageResult(task), nil
	})
	drain(t, w, ctx)
	b, err := s.GetRouteBrief(ctx, run.BriefID, "")
	if err != nil || b.Status != models.BriefDraft || b.Submitted.Notes != "Curator revised scope" {
		t.Fatalf("overwrote curator: %+v %v", b, err)
	}
	final, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || final.Reason != "brief_changed" {
		t.Fatalf("outcome: %+v %v", final, err)
	}
}

func TestResearchFinalFailureRemainsRecoverableUntilDispositionPersists(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET max_attempts=1 WHERE id=$1`, run.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool().Exec(ctx, `CREATE FUNCTION reject_research_disposition() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'simulated checkpoint write failure'; END $$; CREATE TRIGGER research_disposition BEFORE UPDATE OF status ON brief_research_runs FOR EACH ROW EXECUTE FUNCTION reject_research_disposition()`); err != nil {
		t.Fatal(err)
	}
	w.Research = researchFunc(func(context.Context, ResearchTask) (models.ResearchOutput, error) {
		return models.ResearchOutput{}, ResearchFailure{Code: "source_unavailable"}
	})
	if _, err := w.RunOnce(ctx); err == nil {
		t.Fatal("expected checkpoint failure")
	}
	job, err := s.GetJob(ctx, run.JobID)
	if err != nil || job.Status != models.JobStatusRunning {
		t.Fatalf("failed job stranded its active brief: %+v %v", job, err)
	}
	if _, err := s.Pool().Exec(ctx, `DROP TRIGGER research_disposition ON brief_research_runs`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET started_at=now()-interval '1 hour' WHERE id=$1`, run.JobID); err != nil {
		t.Fatal(err)
	}
	drain(t, w, ctx)
	final, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || final.Status != "needs_research" || final.Reason != "worker_retry_exhausted" {
		t.Fatalf("final disposition: %+v %v", final, err)
	}
	job, err = s.GetJob(ctx, run.JobID)
	if err != nil || job.Status != models.JobStatusFailed {
		t.Fatalf("recovered job: %+v %v", job, err)
	}
}

func TestResearchUnconfiguredAndInvalidResultsStayPrivate(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "unconfigured", true: "invalid output"}[invalid], func(t *testing.T) {
			w, s, ctx, run := researchSetup(t)
			reason := "processor_unavailable"
			if invalid {
				reason = "invalid_output"
				w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
					out := stageResult(task)
					out.Candidates = append(out.Candidates, models.Candidate{Kind: "invalid", Payload: json.RawMessage(`{}`)})
					return out, nil
				})
			}
			drain(t, w, ctx)
			final, err := s.GetResearchRun(ctx, run.ID)
			if err != nil || final.Status != "needs_research" || final.Reason != reason || final.Stages[0].Reason != reason {
				t.Fatalf("outcome: %+v %v", final, err)
			}
			var count int
			if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM candidates WHERE source_name='brief_research'`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("partial candidates committed: %d %v", count, err)
			}
		})
	}
}
