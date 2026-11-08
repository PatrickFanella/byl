package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"invisible-museum/backend/internal/models"
)

// GetCuratorReviewPackage takes one database snapshot, without locks on writers,
// external calls or mutations. A later approval must recheck this complete read set.
func (s *Store) GetCuratorReviewPackage(ctx context.Context, brief, run, tour, expected string) (*models.CuratorReviewPackage, error) {
	tx, err := s.rawPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scoped := &Store{pool: transactionDB{tx}, rawPool: s.rawPool}
	out, err := scoped.curatorReviewPackage(ctx, brief, run, tour)
	if err != nil {
		return nil, err
	}
	if expected != "" && out.Version != expected {
		return nil, fmt.Errorf("%w: review package changed; reload before reviewing", ErrConflict)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

type packageReader struct {
	store    *Store
	out      *models.CuratorReviewPackage
	entities map[string]int
	taskKeys map[string]bool
	evidence map[string]bool
}

func (p *packageReader) task(code, key, detail string) {
	k := code + ":" + key + ":" + detail
	if p.taskKeys[k] {
		return
	}
	p.taskKeys[k] = true
	p.out.Tasks = append(p.out.Tasks, models.ReviewPackageTask{Code: code, EntityKey: key, Detail: detail})
}
func packageJSON(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }

func (s *Store) curatorReviewPackage(ctx context.Context, brief, runID, tourID string) (*models.CuratorReviewPackage, error) {
	b, err := s.GetRouteBrief(ctx, brief, "")
	if err != nil {
		return nil, err
	}
	out := &models.CuratorReviewPackage{SchemaVersion: 1, VisitorStories: []models.ReviewVisitorStory{}, State: "needs_research", Brief: b, Routes: map[models.TravelMode]*models.TourRoute{}, PathReports: map[models.TravelMode]*models.PathQualityReport{}, Reconciliation: []models.ReconciliationRecord{}, Entities: []models.ReviewPackageEntity{}, Evidence: []models.SourceEvidence{}, Tasks: []models.ReviewPackageTask{}}
	p := &packageReader{store: s, out: out, entities: map[string]int{}, taskKeys: map[string]bool{}, evidence: map[string]bool{}}
	var run *models.ResearchRun
	if runID == "" {
		run, err = s.LatestBriefResearch(ctx, brief, "")
	} else {
		run, err = s.GetResearchRun(ctx, runID)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if runID != "" && errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if run != nil {
		if run.BriefID != brief {
			return nil, ErrNotFound
		}
		out.Research = run
		out.Submitted = &run.Submitted
		if !bytes.Equal(packageJSON(b.Submitted), packageJSON(run.Submitted)) {
			p.task("brief_changed", brief, "The retained run describes an earlier brief; start or select current research.")
		}
		for _, stage := range run.Stages {
			if stage.Status != "completed" {
				p.task("stage_incomplete", stage.Stage, "This research stage has not completed: "+stage.Status+" "+stage.Reason)
			}
			if stage.Stage == "assembly" && len(stage.Output) > 0 && string(stage.Output) != "{}" {
				var proposal models.ItineraryProposal
				if json.Unmarshal(stage.Output, &proposal) != nil || proposal.Version != 1 {
					return nil, ErrInvalidReference
				}
				out.Proposal = &proposal
			}
		}
	} else {
		p.task("research_not_started", brief, "No retained research run is available.")
	}
	records, err := s.ListReconciliation(ctx, brief, 21, 0)
	if err != nil {
		return nil, err
	}
	if len(records) > 20 {
		records = records[:20]
		p.task("package_limit", brief, "Inspect a smaller shortlist; more than 20 linked identities remain.")
	}
	out.Reconciliation = records
	recordVersions := map[string]models.ReconciliationRecord{}
	selectedCandidates := map[string]bool{}
	if out.Proposal != nil {
		for _, stop := range out.Proposal.Stops {
			if stop.Material.CandidateID != "" {
				selectedCandidates[stop.Material.CandidateID] = true
			}
		}
		for _, gap := range out.Proposal.ResearchGaps {
			p.task(gap.Code, gap.RecordID, gap.Detail)
		}
		if out.Proposal.Status != "draft" {
			p.task("proposal_needs_research", brief, "The retained itinerary is a partial proposal.")
		}
	} else {
		p.task("itinerary_missing", brief, "No assembled visitor draft is available.")
	}
	if run != nil {
		candidates := map[string]bool{}
		for _, stage := range run.Stages {
			for _, id := range stage.CandidateIDs {
				candidates[id] = true
			}
		}
		ids := make([]string, 0, len(candidates))
		for id := range candidates {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		if len(ids) > 64 {
			ids = ids[:64]
			p.task("package_limit", brief, "More than 64 research candidates remain; inspect a smaller shortlist.")
		}
		for _, id := range ids {
			if err := p.candidate(ctx, id, selectedCandidates[id]); err != nil {
				return nil, err
			}
		}
	}
	for _, record := range records {
		recordVersions[record.ID] = record
		for _, id := range []*string{record.PlaceCandidateID, record.MomentCandidateID} {
			if id != nil {
				if err := p.candidate(ctx, *id, selectedCandidates[*id]); err != nil {
					return nil, err
				}
			}
		}
	}
	if out.Proposal != nil {
		if len(out.Proposal.Stops) < 2 || len(out.Proposal.Stops) > 8 {
			p.task("itinerary_size", brief, "A complete bounded itinerary needs 2–8 supported stops.")
		}
		for i, stop := range out.Proposal.Stops {
			if err := p.material(ctx, i+1, stop.Material, recordVersions); err != nil {
				return nil, err
			}
		}
	}
	// Include the brief's retained passages, then resolve selected cross-brief proof.
	evidence, err := s.ListSourceEvidence(ctx, brief, 50, 0)
	if err != nil {
		return nil, err
	}
	if len(evidence) == 50 {
		p.task("package_limit", brief, "The brief has reached the 50-source read bound; inspect a smaller evidence set.")
	}
	found := map[string]bool{}
	for _, e := range evidence {
		out.Evidence = append(out.Evidence, e)
		found[e.ID] = true
	}
	ids := make([]string, 0, len(p.evidence))
	for id := range p.evidence {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		var e *models.SourceEvidence
		if found[id] {
			for i := range out.Evidence {
				if out.Evidence[i].ID == id {
					e = &out.Evidence[i]
					break
				}
			}
		} else {
			if len(out.Evidence) >= 64 {
				p.task("package_limit", id, "The selected sources exceed the 64-source package bound.")
				continue
			}
			e, err = s.GetSourceEvidence(ctx, id)
			if errors.Is(err, ErrNotFound) {
				p.task("evidence_missing", id, "Selected proof is unavailable.")
				continue
			}
			if err != nil {
				return nil, err
			}
			out.Evidence = append(out.Evidence, *e)
		}
		if p.evidence[id] && !packageInspectedEvidence(e) {
			p.task("evidence_not_inspected", id, "Selected historical claims need retained inspected passages.")
		}
	}
	slices.SortFunc(out.Evidence, func(a, b models.SourceEvidence) int { return strings.Compare(a.ID, b.ID) })
	if tourID != "" {
		if !isUUID(tourID) {
			return nil, ErrNotFound
		}
		tour, err := s.GetTour(ctx, tourID, false)
		if err != nil {
			return nil, err
		}
		out.Tour = tour
		if tour.HeroMedia != nil {
			if err := p.entity(ctx, "media", tour.HeroMedia.ID, true); err != nil {
				return nil, err
			}
		}
		if tour.OwnerAccountID != nil {
			p.task("personal_tour", tour.ID, "Select a curated working tour; personal routes have a separate owner workflow.")
		}
		if out.Proposal == nil || len(tour.Stops) != len(out.Proposal.Stops) {
			p.task("tour_itinerary_mismatch", tour.ID, "The prepared tour does not match the retained story itinerary.")
		} else {
			for i, stop := range tour.Stops {
				m := out.Proposal.Stops[i].Material
				if stop.Place.ID != m.PlaceID || m.MomentID == "" || stop.FeaturedMoment == nil || stop.FeaturedMoment.ID != m.MomentID {
					p.task("tour_itinerary_mismatch", stop.ID, "Stop order, place and selected event must match the reviewed proposal.")
				}
			}
		}
		requiredModes := b.Submitted.Modes
		if out.Proposal != nil {
			requiredModes = out.Proposal.Modes
		}
		if len(requiredModes) < 1 || len(requiredModes) > 2 {
			p.task("tour_modes_missing", tour.ID, "Select walking, cycling or both for the reviewed itinerary.")
		}
		actualModes, wantedModes := slices.Clone(tour.Modes), slices.Clone(requiredModes)
		slices.Sort(actualModes)
		slices.Sort(wantedModes)
		if !slices.Equal(actualModes, wantedModes) {
			p.task("tour_modes_mismatch", tour.ID, "The prepared tour modes differ from the retained proposal.")
		}
		for _, mode := range requiredModes {
			route, err := s.GetTourRoute(ctx, tour.ID, mode)
			if errors.Is(err, ErrNotFound) {
				out.Routes[mode] = nil
				p.task("route_missing", string(mode), "This mode has no computed path.")
				continue
			}
			if err != nil {
				return nil, err
			}
			route.Journey = models.DescribeJourney(tour, route)
			if out.Proposal != nil {
				shape := out.Proposal.TripShape
				included := route.Journey.Return.Status == "included"
				if ((shape == "loop" || shape == "out_and_back") && !included) || (shape == "point_to_point" && included) {
					p.task("tour_shape_mismatch", string(mode), "The selected geometry's explicit return does not match the proposed trip shape.")
				}
				if shape == "out_and_back" && !outAndBackReference(tour, mode) {
					p.task("return_reference_unresolved", string(mode), "Out-and-back travel needs a reviewed reference with explicitly reversed return traversals.")
				}
				if out.Proposal.TargetDurationMinutes != nil && route.Journey.TravelDurationS != nil && *route.Journey.TravelDurationS+60*(out.Proposal.PlannedDwellMinutes+out.Proposal.PlannedNarrationMinutes) > 60*(*out.Proposal.TargetDurationMinutes) {
					p.task("duration_exceeds_target", string(mode), "Computed travel plus the proposal's stop assumptions exceeds the requested duration.")
				}
			}
			out.Routes[mode] = route
			if !route.Journey.Usable {
				p.task("route_not_usable", string(mode), "Route readiness is "+route.Journey.State+"; inspect the mode's retained quality report.")
			}
			report, err := s.GetPathQualityReport(ctx, tour.ID, mode, "")
			if errors.Is(err, ErrNotFound) {
				out.PathReports[mode] = nil
			} else if err != nil {
				return nil, err
			} else {
				out.PathReports[mode] = report
			}
		}
	} else {
		p.task("tour_not_prepared", brief, "Select a prepared working tour to inspect computed geometry and path quality.")
	}
	if b.Status != models.BriefReview {
		p.task("brief_not_in_review", brief, "The brief must reach curator review before publication is eligible.")
	}
	if run == nil {
		out.State = "not_started"
	} else if run.Status == "active" {
		out.State = "processing"
	} else if run.Status == "completed" && b.Status == models.BriefReview && len(out.Tasks) == 0 {
		out.State = "ready_for_review"
		out.ReadyToPublish = true
	}
	for _, task := range out.Tasks {
		switch task.Code {
		case "brief_changed", "story_changed", "place_changed", "identity_changed", "source_changed", "candidate_changed":
			out.State = "stale"
			out.ReadyToPublish = false
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(raw) > 8<<20 {
		return nil, fmt.Errorf("%w: review package exceeds 8 MiB; narrow linked material", ErrInvalidReference)
	}
	digest := sha256.Sum256(raw)
	out.Version = hex.EncodeToString(digest[:])
	return out, nil
}

func (p *packageReader) material(ctx context.Context, position int, m models.ItineraryMaterial, versions map[string]models.ReconciliationRecord) error {
	story := models.ReviewVisitorStory{Position: position, PlaceID: m.PlaceID, MomentID: m.MomentID, CandidateID: m.CandidateID, Title: m.MomentTitle, Body: m.Narrative, TextSource: "captured_partial", SourceURLs: m.SourceURLs, EvidenceIDs: m.EvidenceIDs}
	defer func() { p.out.VisitorStories = append(p.out.VisitorStories, story) }()
	record, linked := versions[m.RecordID]
	assessment := record.Assessment
	if record.Decision != nil {
		assessment.PlaceID = record.Decision.PlaceID
		assessment.MomentID = record.Decision.MomentID
		assessment.Outcome = record.Decision.Outcome
	}
	if !linked || record.ReviewVersion != m.ReviewVersion || assessment.PlaceID != m.PlaceID || (assessment.MomentID != "" && assessment.MomentID != m.MomentID) || assessment.Outcome == "rejected" || assessment.Outcome == "needs_review" || assessment.Outcome == "new_place" {
		p.task("identity_changed", m.RecordID, "The selected identity decision changed or is no longer linked.")
	}
	if err := p.entity(ctx, "place", m.PlaceID, true); err != nil {
		return err
	}
	placeEntry := p.out.Entities[p.entities["place:"+m.PlaceID]]
	if len(placeEntry.Current) > 0 {
		var place models.PlaceSummary
		_ = json.Unmarshal(placeEntry.Current, &place)
		if place.Name != m.PlaceName || place.CityID != m.CityID || place.Latitude != m.Latitude || place.Longitude != m.Longitude {
			p.task("place_changed", place.ID, "Location or identity differs from the captured itinerary material.")
		}
	}
	if m.MomentID != "" {
		if err := p.entity(ctx, "moment", m.MomentID, true); err != nil {
			return err
		}
		entry := p.out.Entities[p.entities["moment:"+m.MomentID]]
		if len(entry.Current) > 0 {
			var moment models.MomentDetail
			_ = json.Unmarshal(entry.Current, &moment)
			if moment.PlaceID == m.PlaceID && textHash(moment.Body) == m.ContentSHA256 && moment.Title == m.MomentTitle {
				story.Body = moment.Body
				story.TextSource = "curated_record"
			}
			if moment.PlaceID != m.PlaceID || textHash(moment.Body) != m.ContentSHA256 || moment.Title != m.MomentTitle {
				p.task("story_changed", m.MomentID, "Selected history differs from the captured narrative; regenerate the proposal.")
			}
			for _, url := range m.SourceURLs {
				if !slices.ContainsFunc(moment.Sources, func(s models.Source) bool { return s.URL == url }) {
					p.task("source_changed", m.MomentID, "A captured historical source is no longer attached.")
				}
			}
		}
	} else {
		p.task("moment_not_prepared", m.CandidateID, "The selected historical candidate needs an approved draft entity before matching prepared route stops.")
	}
	if m.CandidateID != "" {
		if err := p.candidate(ctx, m.CandidateID, true); err != nil {
			return err
		}
		entry := p.out.Entities[p.entities["candidate:"+m.CandidateID]]
		if m.EvidenceKind == "evidence_linked_draft" && entry.Candidate != nil {
			current, err := p.store.itineraryCandidateMaterial(ctx, entry.Candidate, m.PlaceID)
			if errors.Is(err, ErrInvalidReference) || (err == nil && current.ContentSHA256 != m.ContentSHA256) {
				p.task("candidate_changed", m.CandidateID, "The captured generated draft or its inspected proof is stale.")
			} else if err != nil {
				return err
			} else {
				var draft struct {
					Title string `json:"title"`
					Body  string `json:"body"`
				}
				if json.Unmarshal(entry.Proposed, &draft) == nil {
					story.Title = draft.Title
					story.Body = draft.Body
					story.TextSource = "evidence_linked_draft"
				}
			}
		}
	}
	if len(m.SourceURLs) == 0 {
		p.task("historical_sources_missing", m.MomentID, "Selected narrative has no source links.")
	}
	for _, id := range m.EvidenceIDs {
		p.evidence[id] = true
	}
	return nil
}

func (p *packageReader) entity(ctx context.Context, kind, id string, required bool) error {
	key := kind + ":" + id
	n, exists := p.entities[key]
	if exists {
		if !required || p.out.Entities[n].Required {
			return nil
		}
		p.out.Entities[n].Required = true
	} else {
		n = len(p.out.Entities)
		p.entities[key] = n
		p.out.Entities = append(p.out.Entities, models.ReviewPackageEntity{Key: key, Kind: kind, ID: id, Required: required, Disposition: "unavailable", Changes: []models.ReviewPackageChange{}})
	}
	var value any
	var status models.EntityStatus
	var err error
	switch kind {
	case "place":
		var place *models.PlaceDetail
		place, err = p.store.GetPlace(ctx, id, "", true)
		if err == nil {
			value = place.PlaceSummary
			status = place.Status
			if required {
				if place.PrimaryMomentID == nil {
					p.task("primary_history_missing", key, "The place needs a selected primary history.")
				} else if err := p.entity(ctx, "moment", *place.PrimaryMomentID, true); err != nil {
					return err
				}
				for _, asset := range place.Media {
					if err := p.entity(ctx, "media", asset.ID, true); err != nil {
						return err
					}
				}
			}
		}
	case "moment":
		var moment *models.MomentDetail
		moment, err = p.store.GetMoment(ctx, id, true)
		if err == nil {
			value = moment
			status = moment.Status
			if required {
				media := slices.Clone(moment.Media)
				if moment.HeroMedia != nil {
					media = append(media, *moment.HeroMedia)
				}
				for _, gap := range models.HistoryReviewGaps(moment.Body, moment.Sources, media) {
					p.task("history_incomplete", key, gap)
				}
				// Optional generated audio follows the existing narration contract;
				// historical/image publication checks do not require a reusable audio license.
				if moment.Narration != nil {
					if err := p.entity(ctx, "media", moment.Narration.ID, false); err != nil {
						return err
					}
				}
				for _, asset := range media {
					if err := p.entity(ctx, "media", asset.ID, true); err != nil {
						return err
					}
				}
			}
		}
	case "media":
		var asset *models.MediaAsset
		asset, err = p.store.GetMedia(ctx, id)
		if err == nil {
			value = asset
			status = asset.Status
			if required {
				if issue := asset.PublicationIssue(); issue != "" {
					p.task("media_rights_incomplete", key, issue)
				}
			}
		}
	default:
		return ErrInvalidReference
	}
	if errors.Is(err, ErrNotFound) {
		if required {
			p.task("dependency_missing", key, "This required entity is unavailable.")
		}
		return nil
	}
	if err != nil {
		return err
	}
	entry := &p.out.Entities[n]
	entry.Current = packageJSON(value)
	entry.Disposition = "reused_draft"
	if status == models.EntityStatusPublished {
		entry.Disposition = "reused_published"
	}
	if status == models.EntityStatusArchived && required {
		p.task("dependency_archived", key, "This required entity is archived.")
	}
	return nil
}

func (p *packageReader) candidate(ctx context.Context, id string, required bool) error {
	key := "candidate:" + id
	if n, ok := p.entities[key]; ok {
		entry := &p.out.Entities[n]
		if required && !entry.Required && (entry.Candidate == nil || entry.Candidate.Status != models.CandidateStatusApproved) {
			p.task("candidate_requires_review", key, "This selected candidate has not been approved as a draft entity.")
		}
		entry.Required = entry.Required || required
		return nil
	}
	c, err := p.store.GetCandidate(ctx, id)
	if errors.Is(err, ErrNotFound) {
		p.entities[key] = len(p.out.Entities)
		p.out.Entities = append(p.out.Entities, models.ReviewPackageEntity{Key: key, Kind: "candidate", ID: id, Required: required, Disposition: "unavailable", Changes: []models.ReviewPackageChange{}})
		p.task("candidate_missing", key, "A linked candidate is unavailable.")
		return nil
	}
	if err != nil {
		return err
	}
	entry := models.ReviewPackageEntity{Key: key, Kind: string(c.Kind), ID: id, Required: required, Disposition: "new_entity", Candidate: c, Proposed: c.Payload, ProposalSource: "candidate_editor", Changes: []models.ReviewPackageChange{}}
	target := ""
	if c.Kind == models.CandidateKindPlace && c.TargetPlaceID != nil {
		target = *c.TargetPlaceID
	}
	if c.Kind == models.CandidateKindMoment && c.TargetMomentID != nil {
		target = *c.TargetMomentID
	}
	if c.Status == models.CandidateStatusApproved && c.ApprovedEntityID != nil {
		target = *c.ApprovedEntityID
	}
	if target != "" {
		if err := p.entity(ctx, string(c.Kind), target, false); err != nil {
			return err
		}
		targetEntry := p.out.Entities[p.entities[string(c.Kind)+":"+target]]
		entry.Current = targetEntry.Current
		entry.Disposition = "proposed_draft_edit"
		if targetEntry.Disposition == "reused_published" {
			entry.Disposition = "proposed_shared_edit"
		}
		if c.Status == models.CandidateStatusApproved {
			entry.Disposition = targetEntry.Disposition
		}
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(c.Payload, &payload) == nil {
		var drafts []json.RawMessage
		if json.Unmarshal(payload["generated_drafts"], &drafts) == nil && len(drafts) > 0 {
			entry.Proposed = drafts[len(drafts)-1]
			entry.ProposalSource = "generated_draft_unapplied"
			var draft struct {
				Evidence []struct {
					ID string `json:"id"`
				} `json:"evidence_manifest"`
			}
			_ = json.Unmarshal(entry.Proposed, &draft)
			for _, ref := range draft.Evidence {
				p.evidence[ref.ID] = p.evidence[ref.ID] || required
			}
		}
	}
	entry.Changes = packageChanges(entry.Current, entry.Proposed)
	p.entities[key] = len(p.out.Entities)
	p.out.Entities = append(p.out.Entities, entry)
	if required && c.Status != models.CandidateStatusApproved {
		p.task("candidate_requires_review", key, "This selected candidate has not been approved as a draft entity.")
	}
	return nil
}

func packageChanges(current, proposed json.RawMessage) []models.ReviewPackageChange {
	out := []models.ReviewPackageChange{}
	var before, after map[string]json.RawMessage
	if json.Unmarshal(current, &before) != nil || json.Unmarshal(proposed, &after) != nil {
		return out
	}
	if _, requested := after["hero_media_id"]; requested && before["hero_media_id"] == nil {
		var hero struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(before["hero_media"], &hero) == nil && hero.ID != "" {
			before["hero_media_id"] = packageJSON(hero.ID)
		}
	}
	fields := []string{"address", "body", "city_id", "date_display", "date_end", "date_precision", "date_start", "hero_media_id", "kind", "latitude", "longitude", "name", "place_id", "sources", "summary", "title", "topic_ids"}
	for _, field := range fields {
		v, ok := after[field]
		if !ok {
			continue
		}
		a := before[field]
		if a == nil {
			a = json.RawMessage("null")
		}
		var av, bv any
		_ = json.Unmarshal(a, &av)
		_ = json.Unmarshal(v, &bv)
		if !reflect.DeepEqual(av, bv) {
			out = append(out, models.ReviewPackageChange{Field: field, Before: a, After: v})
		}
	}
	return out
}

func outAndBackReference(tour *models.TourDetail, mode models.TravelMode) bool {
	if tour.Corridor == nil || tour.Corridor.Intent == nil {
		return false
	}
	for _, selection := range tour.Corridor.Intent.Routes {
		if selection.Mode == mode {
			path := selection.Path
			if len(path) == 0 || len(path)%2 != 0 {
				return false
			}
			for i := 0; i < len(path)/2; i++ {
				a, b := path[i], path[len(path)-i-1]
				if a.SegmentID != b.SegmentID || a.Reverse == b.Reverse {
					return false
				}
			}
			return true
		}
	}
	return false
}

func packageInspectedEvidence(e *models.SourceEvidence) bool {
	if e == nil || e.Result == nil {
		return false
	}
	r := e.Result
	_, err := hex.DecodeString(r.ContentSHA256)
	return r.Outcome == "inspected_page" && r.HTTPStatus >= 200 && r.HTTPStatus < 300 && !r.RetrievedAt.IsZero() && len(r.ContentSHA256) == 64 && err == nil && len(r.Passages) > 0
}
