package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/sources"
)

type briefDiscoveryAdapter struct {
	calls *int
	wait  bool
}

func (a briefDiscoveryAdapter) Name() string {
	if a.wait {
		return "loc"
	}
	return "wikipedia_search"
}
func (a briefDiscoveryAdapter) Discover(ctx context.Context, r sources.Request) ([]sources.Found, error) {
	*a.calls++
	if a.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return []sources.Found{{Kind: models.CandidateKindMoment, Title: "Downers Grove workers", Text: "The workers organized at this depot in Downers Grove.", URL: "https://history.example/workers"}}, nil
}

func TestDiscoveryTimeoutRetainsPartialShortlist(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	claim, err := s.ClaimJob(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginResearchStage(ctx, run.ID, "research", claim); err != nil {
		t.Fatal(err)
	}
	calls := 0
	reg := sources.NewRegistry(sources.NewHTTPClient("fixture"), nil)
	reg.Register(briefDiscoveryAdapter{calls: &calls}, sources.Info{SupportsQuery: true})
	reg.Register(briefDiscoveryAdapter{calls: &calls, wait: true}, sources.Info{SupportsQuery: true})
	w.Discovery = sources.NewDiscovery(reg, sources.DiscoveryPolicy{Providers: []string{"wikipedia_search", "loc"}})
	limited, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	out, err := w.discoverBrief(limited, run, claim)
	if err != nil || out.NeedsResearch != "provider_unavailable" {
		t.Fatalf("partial timeout: %+v %v", out, err)
	}
	var report models.DiscoveryReport
	if err := json.Unmarshal(out.Data, &report); err != nil || len(report.Items) != 1 {
		t.Fatalf("successful provider lost: %+v %v", report, err)
	}
	if err := s.CommitResearchStage(ctx, run.ID, "research", claim, out); err != nil {
		t.Fatal(err)
	}
	saved, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || len(saved.Stages[0].Output) == 0 || saved.Reason != "provider_unavailable" {
		t.Fatalf("partial checkpoint lost: %+v %v", saved, err)
	}
}

func TestDiscoveryCheckpointSurvivesRestartAndDoesNotPublish(t *testing.T) {
	w, s, ctx, run := researchSetup(t)
	calls := 0
	reg := sources.NewRegistry(sources.NewHTTPClient("fixture"), nil)
	reg.Register(briefDiscoveryAdapter{calls: &calls}, sources.Info{SupportsQuery: true})
	w.Discovery = sources.NewDiscovery(reg, sources.DiscoveryPolicy{Providers: []string{"wikipedia_search"}})
	drain(t, w, ctx)
	saved, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || saved.Status != "needs_research" || saved.Reason != "processor_unavailable" || saved.Stages[0].Status != "completed" || calls != 1 {
		t.Fatalf("discovery checkpoint: %+v calls=%d err=%v", saved, calls, err)
	}
	var report models.DiscoveryReport
	if err := json.Unmarshal(saved.Stages[0].Output, &report); err != nil || len(report.Items) != 1 || report.Items[0].Evidence != "search_snippet" {
		t.Fatalf("saved shortlist: %+v %v", report, err)
	}
	if len(saved.Stages[0].CandidateIDs) != 0 {
		t.Fatal("discovery bypassed reconciliation")
	}
	encoded, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var progress map[string]any
	if err := json.Unmarshal(encoded, &progress); err != nil {
		t.Fatal(err)
	}
	stages := progress["stages"].([]any)
	if _, leaked := stages[0].(map[string]any)["output"]; leaked {
		t.Fatal("source text leaked to owner progress")
	}
	b, err := s.GetRouteBrief(ctx, run.BriefID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeBriefResearch(ctx, b.ID, models.ResearchActionRequest{Version: b.Version, RunID: run.ID}, true); err != nil {
		t.Fatal(err)
	}
	// A fresh worker/store resumes completed discovery without repeating providers.
	w.store = repository.New(s.Pool())
	w.Discovery = nil
	w.Research = researchFunc(func(_ context.Context, task ResearchTask) (models.ResearchOutput, error) {
		if task.Stage == "research" {
			t.Fatal("completed discovery replayed")
		}
		return models.ResearchOutput{Data: json.RawMessage(`{}`), NeedsResearch: "insufficient_evidence"}, nil
	})
	drain(t, w, ctx)
	if calls != 1 {
		t.Fatal("provider replayed after restart")
	}
}

func TestDiscoveryConcurrentRequestBudgetAndClaimFence(t *testing.T) {
	_, s, ctx, run := researchSetup(t)
	claim, err := s.ClaimJob(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if claim.ID != run.JobID {
		t.Fatalf("unexpected job: %+v", claim)
	}
	if started, err := s.BeginResearchStage(ctx, run.ID, "research", claim); err != nil || !started {
		t.Fatalf("start: %v %v", started, err)
	}
	results := make(chan bool, 20)
	failures := make(chan error, 20)
	for range 20 {
		go func() {
			ok, err := s.ReserveDiscoveryRequest(ctx, run.ID, claim, false)
			results <- ok
			failures <- err
		}()
	}
	allowed := 0
	for range 20 {
		if <-results {
			allowed++
		}
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
	}
	if allowed != 11 {
		t.Fatalf("budget escaped: %d additional requests", allowed)
	}
	saved, err := s.GetResearchRun(ctx, run.ID)
	if err != nil || saved.RequestsUsed != 12 || saved.TokensReserved != 2000 {
		t.Fatalf("durable budget: %+v %v", saved, err)
	}
	// Budget exhaustion must still permit a partial successful checkpoint.
	if err := s.CommitResearchStage(ctx, run.ID, "research", claim, models.ResearchOutput{Data: json.RawMessage(`{"items":[{"title":"Partial successful result"}]}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveDiscoveryRequest(ctx, run.ID, claim, true); !errors.Is(err, repository.ErrResearchInactive) {
		t.Fatalf("completed stage permitted request: %v", err)
	}
}

func TestDiscoveryRejectsCancelledClaimBeforeFirstHTTP(t *testing.T) {
	_, s, ctx, run := researchSetup(t)
	claim, err := s.ClaimJob(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginResearchStage(ctx, run.ID, "research", claim); err != nil {
		t.Fatal(err)
	}
	b, err := s.GetRouteBrief(ctx, run.BriefID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeBriefResearch(ctx, b.ID, models.ResearchActionRequest{Version: b.Version, RunID: run.ID}, false); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ReserveDiscoveryRequest(ctx, run.ID, claim, true); ok || !errors.Is(err, repository.ErrResearchInactive) {
		t.Fatalf("cancelled request: %v %v", ok, err)
	}
}
