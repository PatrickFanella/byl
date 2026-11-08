package repository

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"invisible-museum/backend/internal/models"
)

// Read one stable snapshot. Approval independently rechecks this complete set,
// including newer retrieval attempts and the current place/publication state.
func (s *Store) GetHappeningReviewPackage(ctx context.Context, id string, now time.Time) (*models.HappeningReviewPackage, error) {
	tx, err := s.rawPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	p, err := (&Store{pool: transactionDB{tx}, rawPool: s.rawPool}).happeningReviewPackage(ctx, id, now)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) latestHappeningEvidence(ctx context.Context, key string) (*models.SourceEvidence, error) {
	return scanEvidence(s.pool.QueryRow(ctx, `SELECT `+evidenceCols+` FROM source_evidence WHERE source_key=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, key))
}

// Snippets, catalogue-only hints, direct user wording and failed retrievals
// cannot support a current announcement's public claims.
func inspectedHappeningEvidence(e models.SourceEvidence) bool {
	r := e.Result
	if r == nil || !r.Valid() || r.Outcome != "inspected_page" || len(r.Passages) == 0 || r.RetrievedAt.IsZero() {
		return false
	}
	digest, err := hex.DecodeString(r.ContentSHA256)
	return err == nil && len(digest) == 32
}

func (s *Store) happeningReviewPackage(ctx context.Context, id string, now time.Time) (*models.HappeningReviewPackage, error) {
	d, err := s.GetHappeningDraft(ctx, id)
	if err != nil {
		return nil, err
	}
	place, err := scanPlace(s.pool.QueryRow(ctx, `SELECT `+placeCols+` FROM places p WHERE p.id=$1`, d.Input.PlaceID), false)
	if err != nil {
		return nil, err
	}
	city, err := s.GetCity(ctx, place.CityID)
	if err != nil {
		return nil, err
	}
	zone, err := time.LoadLocation(city.Timezone)
	if err != nil {
		return nil, ErrInvalidReference
	}
	p := &models.HappeningReviewPackage{Draft: *d, Place: &place, Evidence: []models.SourceEvidence{}, LatestChecks: []models.SourceEvidence{}, Tasks: []models.ReviewPackageTask{}}
	task := func(code, detail string) {
		p.Tasks = append(p.Tasks, models.ReviewPackageTask{Code: code, EntityKey: "happening:" + id, Detail: detail})
	}
	if place.Status != models.EntityStatusPublished {
		task("place_unpublished", "Publish the independently reviewed existing place before announcing a current occurrence.")
	}
	invalid := []string{}
	for field := range d.Input.Validate(now.In(zone)) {
		invalid = append(invalid, field)
	}
	slices.Sort(invalid)
	for _, field := range invalid {
		task("invalid_"+field, "Refresh the bounded occurrence, source and expiry decision before review.")
	}
	err = s.pool.QueryRow(ctx, `SELECT action,note,reviewed_by,reviewed_at FROM happening_reviews WHERE draft_id=$1`, d.DraftID).Scan(&p.ReviewedAction, &p.ReviewNote, &p.ReviewedBy, &p.ReviewedAt)
	if err == nil {
		task("already_reviewed", "A completed decision is immutable; make a new draft for another review.")
	} else if !errors.Is(mapErr(err), ErrNotFound) {
		return nil, err
	}
	byID := map[string]models.SourceEvidence{}
	for _, evidenceID := range d.Input.EvidenceIDs {
		e, err := s.GetSourceEvidence(ctx, evidenceID)
		if errors.Is(err, ErrNotFound) {
			task("evidence_missing", "A selected retained source is unavailable.")
			continue
		}
		if err != nil {
			return nil, err
		}
		p.Evidence = append(p.Evidence, *e)
		byID[e.ID] = *e
		if !inspectedHappeningEvidence(*e) {
			task("announcement_not_inspected", "Select a retained inspected announcement with its content hash and passages.")
		}
	}
	covered := map[string]bool{}
	for _, c := range d.Input.Claims {
		e, ok := byID[c.EvidenceID]
		valid := false
		if ok && inspectedHappeningEvidence(e) {
			for _, passage := range e.Result.Passages {
				if passage.Reference == c.Passage && c.Quote != "" && strings.Contains(passage.Text, c.Quote) {
					valid = true
					break
				}
			}
		}
		if valid {
			covered[c.Field] = true
		} else {
			task("uncaptured_claim", "A public claim must cite an exact passage from selected inspected evidence.")
		}
	}
	for _, field := range []string{"title", "summary", "organizer", "occurrence", "place"} {
		if !covered[field] {
			task("missing_"+field+"_citation", "Review captured support for each public wording, organizer, venue and occurrence field.")
		}
	}
	if d.Input.State != "scheduled" && !covered["status"] {
		task("missing_status_citation", "Cancellation, conflict or source-unavailability wording needs retained source support.")
	}
	if d.Input.StartsOn != nil && d.Input.EndsOn != nil && d.Input.ReviewOn != nil && d.Input.ExpiresOn != nil {
		h := &models.Happening{ID: id, PlaceID: place.ID, Title: d.Input.Title, Summary: d.Input.Summary, Organizer: d.Input.Organizer, StartsOn: *d.Input.StartsOn, EndsOn: *d.Input.EndsOn, Timezone: city.Timezone, DeclaredState: d.Input.State, ReviewOn: *d.Input.ReviewOn, ExpiresOn: *d.Input.ExpiresOn, Sources: []models.JourneySource{}, Issues: []string{}}
		for _, e := range p.Evidence {
			if inspectedHappeningEvidence(e) {
				r := e.Result
				h.Sources = append(h.Sources, models.JourneySource{URL: e.URL, Title: r.Title, Publisher: r.Publisher, Version: r.ContentSHA256, RetrievedOn: r.RetrievedAt.UTC().Format("2006-01-02"), License: "unresolved"})
			}
		}
		checks, health, last, err := s.happeningSourceHealth(ctx, p.Evidence)
		if err != nil {
			return nil, err
		}
		p.LatestChecks = checks
		h.SourceHealth = health
		h.LastCheckedAt = last
		if health != "unchanged" {
			h.Issues = append(h.Issues, "source_"+health)
			task("source_"+health, "Inspect the latest announcement attempt and create a newly cited draft before publishing.")
		}
		if last.IsZero() || now.Before(last) || now.Sub(last) >= 7*24*time.Hour {
			task("stale_source", "Current notices require source checks within the last seven days.")
		}
		h.RefreshStatus(now, health == "unavailable")
		p.Preview = h
	}
	if d.PublishedRevisionID != nil {
		var raw []byte
		if err := s.pool.QueryRow(ctx, `SELECT public_snapshot FROM happening_revisions WHERE id=$1 AND happening_id=$2`, *d.PublishedRevisionID, id).Scan(&raw); err != nil {
			return nil, mapErr(err)
		}
		if err := json.Unmarshal(raw, &p.Current); err != nil {
			return nil, err
		}
	}
	p.ReadyToPublish = len(p.Tasks) == 0 && p.Preview != nil
	// Fence calendar boundaries without changing a version every second while
	// the curator reads. All other inputs in this material are retained rows.
	p.ReviewDay = now.In(zone).Format("2006-01-02")
	material, _ := json.Marshal(p)
	p.Version = CandidatePayloadHash(material)
	return p, nil
}

// A changed source hash cannot silently refresh an old occurrence date. Pending
// and failing attempts remain visible even when earlier captured text exists.
func (s *Store) happeningSourceHealth(ctx context.Context, proof []models.SourceEvidence) ([]models.SourceEvidence, string, time.Time, error) {
	checks := []models.SourceEvidence{}
	health := "unchanged"
	last := time.Time{}
	latest := map[string]*models.SourceEvidence{}
	for _, e := range proof {
		current := latest[e.SourceKey]
		if current == nil {
			var err error
			current, err = s.latestHappeningEvidence(ctx, e.SourceKey)
			if err != nil {
				return nil, "", last, err
			}
			latest[e.SourceKey] = current
			checks = append(checks, *current)
		}
		if current.Result == nil {
			// A pending request has not checked the source yet. Preserve the last
			// captured check time while explicitly flagging that pending attempt.
			completed, err := scanEvidence(s.pool.QueryRow(ctx, `SELECT `+evidenceCols+` FROM source_evidence WHERE source_key=$1 AND result IS NOT NULL ORDER BY created_at DESC,id DESC LIMIT 1`, e.SourceKey))
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, "", last, err
			}
			if completed != nil && completed.Result != nil {
				if last.IsZero() || completed.Result.RetrievedAt.Before(last) {
					last = completed.Result.RetrievedAt
				}
				checks = append(checks, *completed)
			}
			if health == "unchanged" {
				health = "pending"
			}
			continue
		}
		checked := current.Result.RetrievedAt
		if last.IsZero() || checked.Before(last) {
			last = checked
		}
		if !inspectedHappeningEvidence(*current) {
			health = "unavailable"
		} else if e.Result == nil || current.Result.ContentSHA256 != e.Result.ContentSHA256 {
			if health != "unavailable" {
				health = "changed"
			}
		}
	}
	if len(proof) == 0 {
		health = "unavailable"
	}
	return checks, health, last, nil
}
