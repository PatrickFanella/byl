package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

const draftEvidenceTextCap = 12000

// draftCandidate appends a private proposal. It never writes curator prose,
// materializes entities or grants publication/media/narration authority.
func (w *Worker) draftCandidate(ctx context.Context, p models.DraftCandidatePayload) error {
	if w.Drafter == nil {
		return fmt.Errorf("%w: %v", ErrNoRetry, llm.ErrUnavailable)
	}
	if _, none := w.Drafter.(llm.None); none {
		return fmt.Errorf("%w: %v", ErrNoRetry, llm.ErrUnavailable)
	}
	cand, err := w.store.GetCandidate(ctx, p.CandidateID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%w: unknown candidate", ErrNoRetry)
		}
		return err
	}
	if cand.Kind != models.CandidateKindMoment || cand.Status != models.CandidateStatusPending {
		return fmt.Errorf("%w: only pending moment candidates can be drafted", ErrNoRetry)
	}
	if p.PayloadSHA256 == "" || p.PayloadSHA256 != repository.CandidatePayloadHash(cand.Payload) {
		return fmt.Errorf("%w: candidate changed; queue a fresh inspected draft", ErrNoRetry)
	}
	evidence, err := w.store.DraftEvidence(ctx, p.EvidenceIDs)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidReference) || errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%w: inspected evidence is required", ErrNoRetry)
		}
		return err
	}
	var payload map[string]any
	if json.Unmarshal(cand.Payload, &payload) != nil || payload == nil {
		return fmt.Errorf("%w: candidate payload is not an object", ErrNoRetry)
	}
	if history, exists := payload["generated_drafts"]; exists {
		items, ok := history.([]any)
		if !ok || len(items) >= 5 {
			return fmt.Errorf("%w: draft history is full or malformed", ErrNoRetry)
		}
	}
	str := func(k string) string { s, _ := payload[k].(string); return s }
	req := llm.DraftRequest{}
	for _, e := range evidence {
		r := e.Result
		source := llm.DraftEvidence{ID: e.ID, URL: r.FinalURL, Title: r.Title, Publisher: r.Publisher, Outcome: r.Outcome, ContentSHA256: r.ContentSHA256, RetrievedAt: r.RetrievedAt.UTC().Format(time.RFC3339Nano)}
		if source.URL == "" {
			source.URL = e.URL
		}
		remaining := draftEvidenceTextCap / len(evidence)
		for _, passage := range r.Passages {
			if remaining == 0 {
				break
			}
			text := []rune(passage.Text)
			if len(text) > remaining {
				text = text[:remaining]
			}
			source.Passages = append(source.Passages, models.EvidencePassage{Reference: passage.Reference, Text: string(text)})
			remaining -= len(text)
		}
		req.Evidence = append(req.Evidence, source)
	}
	placeID := str("place_id")
	if placeID == "" && cand.TargetPlaceID != nil {
		placeID = *cand.TargetPlaceID
	}
	cityID := ""
	if placeID != "" {
		place, err := w.store.GetPlace(ctx, placeID, "", false)
		if err != nil {
			return err
		}
		req.PlaceName, cityID = place.Name, place.CityID
	}
	topics := []models.Topic{}
	if cityID != "" {
		topics, err = w.store.ListTopics(ctx, cityID)
		if err != nil {
			return err
		}
		for _, t := range topics {
			req.Topics = append(req.Topics, llm.TopicOption{Slug: t.Slug, Name: t.Name, Description: t.Description})
		}
	}
	draft, err := w.Drafter.DraftMoment(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrInvalidDraft) {
			return fmt.Errorf("%w: provider returned an invalid evidence-linked draft", ErrNoRetry)
		}
		return errors.New("draft provider failed")
	}
	draft, err = llm.ValidateEvidence(draft, req)
	if err != nil {
		return fmt.Errorf("%w: provider returned an invalid evidence-linked draft", ErrNoRetry)
	}
	offered := []string{}
	for _, t := range topics {
		offered = append(offered, t.Slug)
	}
	draft, _ = llm.ValidateDraft(draft, offered)
	resolved, err := w.store.TopicsBySlugs(ctx, cityID, draft.TopicSlugs)
	if err != nil {
		return err
	}
	topicIDs := []string{}
	for _, t := range resolved {
		topicIDs = append(topicIDs, t.ID)
	}
	sources := []models.Source{}
	manifest := append([]llm.DraftEvidence{}, req.Evidence...)
	for i, e := range manifest {
		sources = append(sources, models.Source{Title: e.Title, URL: e.URL, Publisher: e.Publisher})
		manifest[i].Passages = nil
	}
	proposal := struct {
		llm.MomentDraft
		TopicIDs          []string            `json:"topic_ids"`
		Sources           []models.Source     `json:"sources"`
		Evidence          []llm.DraftEvidence `json:"evidence_manifest"`
		BasePayloadSHA256 string              `json:"base_payload_sha256"`
		GeneratedAt       time.Time           `json:"generated_at"`
		ReviewGaps        []string            `json:"review_gaps"`
	}{draft, topicIDs, sources, manifest, p.PayloadSHA256, time.Now().UTC(), models.HistoryReviewGaps(draft.Body, sources, nil)}
	raw, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	if err := w.store.SaveCandidateDraft(ctx, cand, raw); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return fmt.Errorf("%w: candidate changed or draft history is full; reload before retrying", ErrNoRetry)
		}
		return err
	}
	return nil
}
