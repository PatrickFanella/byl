package jobs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
)

type fakeDrafter struct {
	last  llm.DraftRequest
	draft llm.MomentDraft
	calls int
}

func (f *fakeDrafter) DraftMoment(_ context.Context, req llm.DraftRequest) (llm.MomentDraft, error) {
	f.calls++
	f.last = req
	return f.draft, nil
}

func (f *fakeDrafter) RankMoments(context.Context, llm.RankRequest) (llm.Ranking, error) {
	return llm.Ranking{}, nil
}

func momentCandidate(t *testing.T, w *Worker, place *string) models.Candidate {
	t.Helper()
	src, ext := "wikipedia", "wikipedia:1:moment"
	payload, _ := json.Marshal(map[string]any{
		"title": "Downers Grove train wreck", "raw_text": strings.Repeat("On 1940-01-01 a train derailed near the station. ", 400),
		"source_url": "https://en.wikipedia.org/wiki/Downers_Grove_train_wreck", "source_title": "Downers Grove train wreck",
		"source_publisher": "Wikipedia contributors", "source_license": "CC-BY-SA-4.0",
	})
	c, err := w.store.CreateCandidate(context.Background(), models.Candidate{Kind: models.CandidateKindMoment, Origin: models.CandidateOriginImport, SourceName: &src, ExternalID: &ext, TargetPlaceID: place, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return *c
}

func TestDraftJobMergesDraftIntoCandidate(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	fd := &fakeDrafter{draft: llm.MomentDraft{Title: "The 1940 Wreck", Body: "A freight train left the rails beside the depot.", DateStart: "1940-01-01", DatePrecision: "day", DateDisplay: "January 1, 1940", TopicSlugs: []string{"rail-and-the-commuter-town", "ghost-stories"}, Notes: "Casualty count not in source."}}
	w.Drafter = fd
	place := testdb.SeedPlaceStationID
	cand := momentCandidate(t, w, &place)
	if err := store.Enqueue(ctx, models.JobKindDraftCandidate, models.DraftCandidatePayload{CandidateID: cand.ID}); err != nil {
		t.Fatal(err)
	}
	if n := drain(t, w, ctx); n != 1 {
		t.Fatalf("processed %d", n)
	}
	if fd.calls != 1 || fd.last.PlaceName != "Main Street Station" || len(fd.last.RawText) > 12000 || len(fd.last.Topics) != 4 || fd.last.SourceURL == "" {
		t.Fatalf("draft request: calls=%d place=%q raw=%d topics=%d url=%q", fd.calls, fd.last.PlaceName, len(fd.last.RawText), len(fd.last.Topics), fd.last.SourceURL)
	}
	got, _ := store.GetCandidate(ctx, cand.ID)
	if got.Origin != models.CandidateOriginDraft || got.Status != models.CandidateStatusPending {
		t.Fatalf("candidate after draft: %+v", got)
	}
	var payload map[string]any
	_ = json.Unmarshal(got.Payload, &payload)
	if payload["title"] != "The 1940 Wreck" || payload["body"] == nil || payload["date_start"] != "1940-01-01" || payload["date_precision"] != "day" || payload["place_id"] != testdb.SeedPlaceStationID {
		t.Fatalf("payload %v", payload)
	}
	topicIDs, _ := payload["topic_ids"].([]any)
	if len(topicIDs) != 1 || topicIDs[0] != testdb.SeedTopicRailID {
		t.Fatalf("topic_ids should resolve the known slug only: %v", payload["topic_ids"])
	}
	sourcesList, _ := payload["sources"].([]any)
	if len(sourcesList) != 1 || sourcesList[0].(map[string]any)["url"] != "https://en.wikipedia.org/wiki/Downers_Grove_train_wreck" || sourcesList[0].(map[string]any)["license"] != "CC-BY-SA-4.0" {
		t.Fatalf("sources %v", payload["sources"])
	}
	notes, _ := payload["draft_notes"].(string)
	if !strings.Contains(notes, "Casualty count") || !strings.Contains(notes, "ghost-stories") {
		t.Fatalf("draft_notes should keep the model note and name the dropped slug: %q", notes)
	}
	if payload["raw_text"] == nil || payload["source_url"] == nil {
		t.Fatal("original payload keys must survive the merge")
	}
}

func TestDraftJobFailsWithoutRetryWhenNotDraftable(t *testing.T) {
	w, store, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	w.Drafter = &fakeDrafter{draft: llm.MomentDraft{Title: "x", Body: "y", DatePrecision: "circa"}}
	reviewer, _ := store.UpsertAccount(ctx, models.AuthProviderDev, "r", "r@example.com", "", true)
	cand := momentCandidate(t, w, nil)
	if _, err := store.ReviewCandidate(ctx, cand.ID, reviewer.ID, models.CandidateStatusRejected, "no", nil); err != nil {
		t.Fatal(err)
	}
	_ = store.Enqueue(ctx, models.JobKindDraftCandidate, models.DraftCandidatePayload{CandidateID: cand.ID})
	drain(t, w, ctx)
	failed, _ := store.ListJobs(ctx, models.JobStatusFailed, 10, 0)
	if len(failed) != 1 || failed[0].Attempts != 1 || !strings.Contains(failed[0].LastError, "pending") {
		t.Fatalf("reviewed candidate must fail once: %+v", failed)
	}

	w2, store2, ctx2 := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w2, ctx2)
	w2.Drafter = llm.None{}
	c2 := momentCandidate(t, w2, nil)
	_ = store2.Enqueue(ctx2, models.JobKindDraftCandidate, models.DraftCandidatePayload{CandidateID: c2.ID})
	drain(t, w2, ctx2)
	failed2, _ := store2.ListJobs(ctx2, models.JobStatusFailed, 10, 0)
	if len(failed2) != 1 || failed2[0].Attempts != 1 || !strings.Contains(failed2[0].LastError, "no provider") {
		t.Fatalf("no provider must fail once without retry: %+v", failed2)
	}
	c2after, _ := store2.GetCandidate(ctx2, c2.ID)
	if c2after.Origin != models.CandidateOriginImport {
		t.Fatal("a failed draft must leave the candidate untouched")
	}
}
