package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
	"strings"
	"testing"
)

type fakeDrafter struct {
	last   llm.DraftRequest
	calls  int
	change func(*llm.MomentDraft)
	during func()
}

func (f *fakeDrafter) DraftMoment(_ context.Context, req llm.DraftRequest) (llm.MomentDraft, error) {
	f.calls++
	f.last = req
	if f.during != nil {
		f.during()
	}
	cite := llm.DraftCitation{EvidenceID: req.Evidence[0].ID, Passage: req.Evidence[0].Passages[0].Reference, Quote: "A train derailed in 1940."}
	d := llm.MomentDraft{Title: "A documented derailment", Body: "The archive records a derailment in 1940.", DatePrecision: "circa", TopicSlugs: []string{"rail-and-the-commuter-town", "ghost-stories"}, Notes: "Find further records.", TitleCitations: []llm.DraftCitation{cite}, Claims: []llm.DraftClaim{{Text: "The archive records a derailment in 1940.", Kind: "fact", Citations: []llm.DraftCitation{cite}}}}
	if f.change != nil {
		f.change(&d)
	}
	return d, nil
}
func (f *fakeDrafter) RankMoments(context.Context, llm.RankRequest) (llm.Ranking, error) {
	return llm.Ranking{}, nil
}

type draftEvidenceFetcher struct{}

func (draftEvidenceFetcher) RetrieveEvidence(context.Context, string, string) (models.EvidenceResult, error) {
	text := strings.Repeat("A train derailed in 1940. ", 700)
	h := sha256.Sum256([]byte(text))
	return models.EvidenceResult{Outcome: "inspected_page", FinalURL: "https://archive.example/history", Title: "Depot record", Publisher: "Local archive", ContentSHA256: hex.EncodeToString(h[:]), Passages: []models.EvidencePassage{{Reference: "paragraph:1", Text: text}}}, nil
}
func inspectedDraftEvidence(t *testing.T, w *Worker, ctx context.Context) string {
	t.Helper()
	a, err := w.store.UpsertAccount(ctx, models.AuthProviderDev, "draft-evidence", "archive@example.com", "", true)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := w.store.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Rail", Area: "Downers Grove"}, "")
	if err != nil {
		t.Fatal(err)
	}
	e, err := w.store.QueueSourceEvidence(ctx, b.ID, models.EvidenceRequest{Version: 1, URL: "https://archive.example/history", SourceKind: "page"}, "", strings.Repeat("a", 64), "")
	if err != nil {
		t.Fatal(err)
	}
	w.Evidence = draftEvidenceFetcher{}
	drain(t, w, ctx)
	return e.ID
}
func momentCandidate(t *testing.T, w *Worker, place *string) models.Candidate {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"title": "Curator title", "body": "Curator text", "raw_text": "UNSUPPORTED_USER_ANECDOTE", "sources": []map[string]any{{"title": "Curator archive", "url": "https://curator.example/record"}}})
	c, err := w.store.CreateCandidate(context.Background(), models.Candidate{Kind: models.CandidateKindMoment, Origin: models.CandidateOriginImport, TargetPlaceID: place, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return *c
}
func inspectedDraftPayload(t *testing.T, s *repository.Store, ctx context.Context, id, evidence string) models.DraftCandidatePayload {
	t.Helper()
	c, err := s.GetCandidate(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return models.DraftCandidatePayload{CandidateID: id, EvidenceIDs: []string{evidence}, PayloadSHA256: repository.CandidatePayloadHash(c.Payload)}
}
func TestDraftJobKeepsCuratorFieldsAndAppendsInspectedProposal(t *testing.T) {
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	eid := inspectedDraftEvidence(t, w, ctx)
	f := &fakeDrafter{}
	w.Drafter = f
	place := testdb.SeedPlaceStationID
	c := momentCandidate(t, w, &place)
	p := inspectedDraftPayload(t, s, ctx, c.ID, eid)
	if err := w.draftCandidate(ctx, p); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || f.last.PlaceName != "Main Street Station" || len([]rune(f.last.Evidence[0].Passages[0].Text)) > 12000 || len(f.last.Topics) != 4 {
		t.Fatalf("request %+v", f.last)
	}
	requestJSON, _ := json.Marshal(f.last)
	if strings.Contains(string(requestJSON), "UNSUPPORTED_USER_ANECDOTE") || strings.Contains(string(requestJSON), "Curator text") {
		t.Fatal("uninspected candidate text reached provider")
	}
	got, _ := s.GetCandidate(ctx, c.ID)
	var payload map[string]any
	_ = json.Unmarshal(got.Payload, &payload)
	if got.Status != models.CandidateStatusPending || payload["body"] != "Curator text" || payload["title"] != "Curator title" || payload["sources"].([]any)[0].(map[string]any)["url"] != "https://curator.example/record" {
		t.Fatal("curator fields changed")
	}
	proposal := payload["generated_drafts"].([]any)[0].(map[string]any)
	if proposal["title"] != "A documented derailment" || len(proposal["topic_ids"].([]any)) != 1 || len(proposal["review_gaps"].([]any)) < 2 || !strings.Contains(proposal["notes"].(string), "ghost-stories") {
		t.Fatalf("proposal %+v", proposal)
	}
	if proposal["evidence_manifest"].([]any)[0].(map[string]any)["id"] != eid || len(proposal["claims"].([]any)) != 1 {
		t.Fatal("lost evidence trail")
	}
	if err := w.draftCandidate(ctx, p); !errors.Is(err, ErrNoRetry) || f.calls != 1 {
		t.Fatal("a stale queued draft spent another provider call")
	}
	for range 4 {
		if err := w.draftCandidate(ctx, inspectedDraftPayload(t, s, ctx, c.ID, eid)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.draftCandidate(ctx, inspectedDraftPayload(t, s, ctx, c.ID, eid)); !errors.Is(err, ErrNoRetry) {
		t.Fatal("unbounded proposal history")
	}
}
func TestDraftJobRejectsUnsupportedCitationAndNoEvidence(t *testing.T) {
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	eid := inspectedDraftEvidence(t, w, ctx)
	f := &fakeDrafter{change: func(d *llm.MomentDraft) { d.Claims[0].Citations[0].Quote = "UNSUPPORTED_USER_ANECDOTE" }}
	w.Drafter = f
	c := momentCandidate(t, w, nil)
	p := inspectedDraftPayload(t, s, ctx, c.ID, eid)
	if err := w.draftCandidate(ctx, p); !errors.Is(err, ErrNoRetry) {
		t.Fatal("unsupported citation accepted")
	}
	got, _ := s.GetCandidate(ctx, c.ID)
	if repository.CandidatePayloadHash(got.Payload) != p.PayloadSHA256 {
		t.Fatal("invalid result changed candidate")
	}
	p.EvidenceIDs = nil
	if err := w.draftCandidate(ctx, p); !errors.Is(err, ErrNoRetry) || f.calls != 1 {
		t.Fatal("uninspected text reached provider")
	}
	w.Drafter = llm.None{}
	if err := w.draftCandidate(ctx, p); !errors.Is(err, ErrNoRetry) {
		t.Fatal("missing provider retried")
	}
}
func TestDraftCompletionFencesConcurrentCuratorEditAndReview(t *testing.T) {
	for _, action := range []string{"edit", "review"} {
		t.Run(action, func(t *testing.T) {
			w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
			drain(t, w, ctx)
			eid := inspectedDraftEvidence(t, w, ctx)
			c := momentCandidate(t, w, nil)
			p := inspectedDraftPayload(t, s, ctx, c.ID, eid)
			f := &fakeDrafter{during: func() {
				if action == "edit" {
					if err := s.UpdateCandidatePayload(ctx, c.ID, json.RawMessage(`{"body":"New curator edit"}`), nil); err != nil {
						t.Fatal(err)
					}
				} else {
					a, _ := s.UpsertAccount(ctx, models.AuthProviderDev, "reviewer", "review@example.com", "", true)
					if _, err := s.ReviewCandidate(ctx, c.ID, a.ID, models.CandidateStatusRejected, "Curator decision", nil); err != nil {
						t.Fatal(err)
					}
				}
			}}
			w.Drafter = f
			if err := w.draftCandidate(ctx, p); !errors.Is(err, ErrNoRetry) {
				t.Fatalf("stale completion %v", err)
			}
			got, _ := s.GetCandidate(ctx, c.ID)
			if strings.Contains(string(got.Payload), "generated_drafts") {
				t.Fatal("draft overwrote newer work")
			}
		})
	}
}
