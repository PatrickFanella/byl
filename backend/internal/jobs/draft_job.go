package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

// draftRawTextCap bounds what is sent to the model.
const draftRawTextCap = 12000

// draftCandidate asks the LLM to turn a pending moment candidate's source text into a draft
// and merges the result into the candidate payload. The candidate stays pending for review.
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
	if cand.Kind != models.CandidateKindMoment {
		return fmt.Errorf("%w: only moment candidates can be drafted", ErrNoRetry)
	}
	if cand.Status != models.CandidateStatusPending {
		return fmt.Errorf("%w: candidate is %s, not pending", ErrNoRetry, cand.Status)
	}
	var payload map[string]any
	if err := json.Unmarshal(cand.Payload, &payload); err != nil {
		return fmt.Errorf("%w: candidate payload is not an object", ErrNoRetry)
	}
	str := func(k string) string { s, _ := payload[k].(string); return s }
	rawText := str("raw_text")
	if rawText == "" {
		rawText = str("body")
	}
	if rawText == "" {
		return fmt.Errorf("%w: candidate has no raw_text to draft from", ErrNoRetry)
	}
	if len(rawText) > draftRawTextCap {
		rawText = rawText[:draftRawTextCap]
	}

	req := llm.DraftRequest{
		SourceTitle: str("source_title"), SourceURL: str("source_url"), SourcePublisher: str("source_publisher"),
		SourceLicense: str("source_license"), RawText: rawText,
	}
	if req.SourceTitle == "" {
		req.SourceTitle = str("title")
	}
	placeID := str("place_id")
	if placeID == "" && cand.TargetPlaceID != nil {
		placeID = *cand.TargetPlaceID
	}
	var cityID string
	if placeID != "" {
		place, err := w.store.GetPlace(ctx, placeID, "", true)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		if err == nil {
			req.PlaceName, req.PlaceSummary, cityID = place.Name, place.Summary, place.CityID
		}
	}
	var topics []models.Topic
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
		return fmt.Errorf("draft: %w", err)
	}
	offered := make([]string, 0, len(topics))
	for _, t := range topics {
		offered = append(offered, t.Slug)
	}
	draft, _ = llm.ValidateDraft(draft, offered)
	resolved, err := w.store.TopicsBySlugs(ctx, cityID, draft.TopicSlugs)
	if err != nil && cityID != "" {
		return err
	}
	topicIDs := make([]string, 0, len(resolved))
	for _, t := range resolved {
		topicIDs = append(topicIDs, t.ID)
	}

	payload["title"] = draft.Title
	payload["body"] = draft.Body
	payload["date_start"] = draft.DateStart
	payload["date_end"] = draft.DateEnd
	payload["date_precision"] = draft.DatePrecision
	payload["date_display"] = draft.DateDisplay
	payload["topic_ids"] = topicIDs
	payload["sources"] = []map[string]any{{
		"title": req.SourceTitle, "url": req.SourceURL, "publisher": req.SourcePublisher, "license": req.SourceLicense,
	}}
	payload["draft_notes"] = draft.Notes
	if placeID != "" {
		payload["place_id"] = placeID
	}
	merged, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	origin := models.CandidateOriginDraft
	return w.store.UpdateCandidatePayload(ctx, cand.ID, merged, &origin)
}
