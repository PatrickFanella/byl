package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
)

// CaptureItineraryMaterials resolves only identities linked to this brief. It
// snapshots existing sourced records or validated B5 proposals without approving
// candidates, editing curator text or materializing tours.
func (s *Store) CaptureItineraryMaterials(ctx context.Context, brief string) (models.ItineraryMaterials, error) {
	out := models.ItineraryMaterials{Version: 1, Items: []models.ItineraryMaterial{}, Gaps: []models.ItineraryGap{}}
	records, err := s.ListReconciliation(ctx, brief, 21, 0)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	seenGaps := map[string]bool{}
	for n, record := range records {
		gap := func(code, detail string) {
			key := record.ID + ":" + code
			if seenGaps[key] {
				return
			}
			seenGaps[key] = true
			out.Gaps = append(out.Gaps, models.ItineraryGap{Code: code, RecordID: record.ID, PlaceCandidateID: record.PlaceCandidateID, MomentCandidateID: record.MomentCandidateID, Detail: detail})
		}
		if n >= 20 {
			gap("material_limit", "Additional linked identity records remain available for curator review.")
			break
		}
		assessment := record.Assessment
		if record.Decision != nil {
			assessment.Outcome = record.Decision.Outcome
			assessment.PlaceID = record.Decision.PlaceID
			assessment.MomentID = record.Decision.MomentID
			assessment.Issues = nil
		}
		if assessment.Outcome == "rejected" {
			continue
		}
		if assessment.Outcome == "needs_review" || assessment.Outcome == "new_place" || assessment.PlaceID == "" || len(assessment.Issues) > 0 {
			gap("identity_unresolved", "Resolve this place/event identity before including it in a tour proposal.")
			continue
		}
		place, err := s.GetPlace(ctx, assessment.PlaceID, "", true)
		if errors.Is(err, ErrNotFound) {
			gap("place_unavailable", "The resolved place is unavailable.")
			continue
		}
		if err != nil {
			return out, err
		}
		if place.Status == models.EntityStatusArchived {
			gap("place_unavailable", "The resolved place is archived.")
			continue
		}
		add := func(m models.ItineraryMaterial) {
			key := m.PlaceID + ":" + m.MomentID + ":" + m.CandidateID
			if seen[key] {
				return
			}
			seen[key] = true
			m.RecordID = record.ID
			m.ReviewVersion = record.ReviewVersion
			m.PlaceID = place.ID
			m.PlaceName = place.Name
			m.CityID = place.CityID
			m.Latitude = place.Latitude
			m.Longitude = place.Longitude
			if strings.TrimSpace(m.MomentTitle) == "" || utf8.RuneCountInString(m.MomentTitle) > 300 || utf8.RuneCountInString(place.Name) > 300 || m.Narrative == "" || utf8.RuneCountInString(m.Narrative) > 2000 || len(m.SourceURLs) == 0 || len(out.Items) >= 20 {
				gap("historical_evidence_incomplete", "A bounded historical paragraph and original source links are required; no replacement prose was invented.")
				return
			}
			out.Items = append(out.Items, m)
			raw, _ := json.Marshal(out)
			if len(raw) > 200000 {
				out.Items = out.Items[:len(out.Items)-1]
				gap("material_limit", "The linked material exceeds this bounded proposal; review a smaller shortlist.")
			}
		}
		momentIDs := []string{}
		if assessment.MomentID != "" {
			momentIDs = append(momentIDs, assessment.MomentID)
		} else if assessment.Outcome == "existing_place" {
			for _, m := range place.Moments {
				momentIDs = append(momentIDs, m.ID)
			}
		}
		if len(momentIDs) > 20 {
			momentIDs = momentIDs[:20]
			gap("material_limit", "Additional historical records remain linked for curator review.")
		}
		for _, id := range momentIDs {
			m, err := s.GetMoment(ctx, id, true)
			if errors.Is(err, ErrNotFound) {
				gap("moment_unavailable", "The resolved historical account is unavailable.")
				continue
			}
			if err != nil {
				return out, err
			}
			if m.PlaceID != place.ID || m.Status == models.EntityStatusArchived {
				gap("moment_unavailable", "The selected event does not belong to this available place.")
				continue
			}
			urls := []string{}
			for _, source := range m.Sources {
				if sourceURL(source.URL) && !slices.Contains(urls, source.URL) && len(urls) < 5 {
					urls = append(urls, source.URL)
				}
			}
			add(models.ItineraryMaterial{PlaceID: place.ID, MomentID: m.ID, MomentTitle: m.Title, Narrative: firstParagraph(m.Body), EvidenceKind: "curated_record", ContentSHA256: textHash(m.Body), SourceURLs: urls, EvidenceIDs: []string{}})
		}
		if record.MomentCandidateID != nil {
			c, err := s.GetCandidate(ctx, *record.MomentCandidateID)
			if errors.Is(err, ErrNotFound) {
				gap("candidate_unavailable", "The linked historical candidate is unavailable.")
				continue
			}
			if err != nil {
				return out, err
			}
			if c.Status == models.CandidateStatusApproved && c.ApprovedEntityID != nil {
				m, err := s.GetMoment(ctx, *c.ApprovedEntityID, true)
				if err != nil {
					if errors.Is(err, ErrNotFound) {
						gap("moment_unavailable", "The approved candidate's historical record is unavailable.")
						continue
					}
					return out, err
				}
				if m.PlaceID != place.ID || m.Status == models.EntityStatusArchived {
					gap("identity_unresolved", "The approved event needs identity review.")
					continue
				}
				urls := []string{}
				for _, source := range m.Sources {
					if sourceURL(source.URL) && !slices.Contains(urls, source.URL) && len(urls) < 5 {
						urls = append(urls, source.URL)
					}
				}
				add(models.ItineraryMaterial{PlaceID: place.ID, MomentID: m.ID, CandidateID: c.ID, MomentTitle: m.Title, Narrative: firstParagraph(m.Body), EvidenceKind: "curated_record", ContentSHA256: textHash(m.Body), SourceURLs: urls, EvidenceIDs: []string{}})
			} else if c.Status == models.CandidateStatusPending && c.Kind == models.CandidateKindMoment {
				m, err := s.itineraryCandidateMaterial(ctx, c, place.ID)
				if errors.Is(err, ErrInvalidReference) {
					gap("candidate_needs_evidence", "The pending event needs a current validated evidence-linked historical draft.")
					continue
				}
				if err != nil {
					return out, err
				}
				add(m)
			} else {
				gap("candidate_unavailable", "The historical candidate is not available for assembly.")
			}
		}
	}
	return out, nil
}

func firstParagraph(body string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(body), "\n\n")
	return strings.TrimSpace(first)
}
func textHash(text string) string { h := sha256.Sum256([]byte(text)); return hex.EncodeToString(h[:]) }
func sourceURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && u.User == nil && (u.Scheme == "https" || u.Scheme == "http") && len(raw) <= 2000
}

func (s *Store) itineraryCandidateMaterial(ctx context.Context, c *models.Candidate, place string) (models.ItineraryMaterial, error) {
	bad := func() (models.ItineraryMaterial, error) { return models.ItineraryMaterial{}, ErrInvalidReference }
	var payload map[string]json.RawMessage
	if json.Unmarshal(c.Payload, &payload) != nil {
		return bad()
	}
	var history []json.RawMessage
	if json.Unmarshal(payload["generated_drafts"], &history) != nil || len(history) == 0 || len(history) > 5 {
		return bad()
	}
	raw := history[len(history)-1]
	var draft struct {
		llm.MomentDraft
		BasePayloadSHA256 string              `json:"base_payload_sha256"`
		Evidence          []llm.DraftEvidence `json:"evidence_manifest"`
	}
	if json.Unmarshal(raw, &draft) != nil || len(draft.Evidence) < 1 || len(draft.Evidence) > 5 {
		return bad()
	}
	// Reconstruct the editor input immediately before the last proposal was added.
	// A subsequent curator edit makes that generated proposal stale for assembly.
	if len(history) > 1 {
		payload["generated_drafts"], _ = json.Marshal(history[:len(history)-1])
	} else {
		delete(payload, "generated_drafts")
	}
	before, _ := json.Marshal(payload)
	matches := CandidatePayloadHash(before) == draft.BasePayloadSHA256
	if !matches && len(history) == 1 {
		payload["generated_drafts"] = json.RawMessage(`[]`)
		before, _ = json.Marshal(payload)
		matches = CandidatePayloadHash(before) == draft.BasePayloadSHA256
	}
	if !matches {
		return bad()
	}
	ids := []string{}
	for _, e := range draft.Evidence {
		ids = append(ids, e.ID)
	}
	evidence, err := s.DraftEvidence(ctx, ids)
	if errors.Is(err, ErrNotFound) {
		return bad()
	}
	if err != nil {
		return models.ItineraryMaterial{}, err
	}
	req := llm.DraftRequest{}
	urls := []string{}
	for i, e := range evidence {
		manifest := draft.Evidence[i]
		source := e.Result.FinalURL
		if source == "" {
			source = e.URL
		}
		if manifest.URL != source || e.Result.ContentSHA256 != manifest.ContentSHA256 || e.Result.Outcome != manifest.Outcome || !sourceURL(manifest.URL) {
			return bad()
		}
		manifest.Passages = e.Result.Passages
		req.Evidence = append(req.Evidence, manifest)
		if !slices.Contains(urls, manifest.URL) {
			urls = append(urls, manifest.URL)
		}
	}
	checked, err := llm.ValidateEvidence(draft.MomentDraft, req)
	if err != nil {
		return bad()
	}
	var target string
	_ = json.Unmarshal(payload["place_id"], &target)
	if target == "" && c.TargetPlaceID != nil {
		target = *c.TargetPlaceID
	}
	if target != place {
		return bad()
	}
	return models.ItineraryMaterial{PlaceID: place, CandidateID: c.ID, MomentTitle: checked.Title, Narrative: checked.Claims[0].Text, EvidenceKind: "evidence_linked_draft", ContentSHA256: textHash(string(raw)), SourceURLs: urls, EvidenceIDs: ids}, nil
}

// GetBriefItinerary exposes a retained private snapshot; transport requires a
// curator. SourceVersion identifies the frozen submitted brief, not a ready tour.
func (s *Store) GetBriefItinerary(ctx context.Context, brief, id string) (*models.BriefItinerary, error) {
	if _, err := s.GetRouteBrief(ctx, brief, ""); err != nil {
		return nil, err
	}
	var run *models.ResearchRun
	var err error
	if id == "" {
		run, err = s.LatestBriefResearch(ctx, brief, "")
	} else {
		run, err = s.GetResearchRun(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	if run.BriefID != brief {
		return nil, ErrNotFound
	}
	out := &models.BriefItinerary{RunID: run.ID, SourceVersion: run.SourceVersion, BriefVersion: run.BriefVersion}
	for _, stage := range run.Stages {
		if stage.Stage == "assembly" && len(stage.Output) > 0 && string(stage.Output) != "{}" {
			var p models.ItineraryProposal
			if json.Unmarshal(stage.Output, &p) != nil || p.Version != 1 {
				return nil, ErrInvalidReference
			}
			out.Proposal = &p
		}
	}
	return out, nil
}
