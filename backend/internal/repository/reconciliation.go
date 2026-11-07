package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/reconcile"
	"invisible-museum/backend/internal/sources"
)

// PrepareReconciliation reads a bounded identity catalogue, including unpublished
// records, so an earlier accepted draft is reused rather than rediscovered.
func (s *Store) PrepareReconciliation(ctx context.Context, in models.RouteBriefInput, item models.DiscoveryItem) (models.ReconciliationProposal, error) {
	if len((models.ReconciliationRequest{Version: 1, Item: item}).Validate()) > 0 {
		return models.ReconciliationProposal{}, ErrInvalidReference
	}
	if item.URL != "" {
		if _, _, _, err := sources.NormalizeEvidenceURL(item.URL); err != nil {
			return models.ReconciliationProposal{}, ErrInvalidReference
		}
	}
	cities, err := s.ListCities(ctx)
	if err != nil {
		return models.ReconciliationProposal{}, err
	}
	cityID := ""
	area := item.Area
	if area == "" {
		area = in.Area
	}
	for _, c := range cities {
		if item.Identity != nil && item.Identity.CityID != "" {
			if item.Identity.CityID == c.ID {
				cityID = c.ID
			}
		} else if sources.DiscoveryTerms(area) == sources.DiscoveryTerms(c.Name) || sources.DiscoveryTerms(area) == sources.DiscoveryTerms(c.Name+" "+c.Region) {
			if cityID != "" {
				cityID = ""
				break
			}
			cityID = c.ID
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id,p.city_id,p.name,p.address,p.latitude,p.longitude,p.status,
	 COALESCE((SELECT jsonb_agg(jsonb_build_object('id',m.id,'title',m.title,'date_start',COALESCE(m.date_start::text,''),'date_precision',m.date_precision,'source_urls',COALESCE((SELECT jsonb_agg(ms.url) FROM (SELECT url FROM moment_sources WHERE moment_id=m.id ORDER BY position LIMIT 21) ms),'[]'::jsonb)) ORDER BY m.id) FROM (SELECT id,title,date_start,date_precision FROM moments WHERE place_id=p.id ORDER BY id LIMIT 101) m),'[]'::jsonb)
	 FROM places p WHERE p.city_id::text=$1 OR p.id::text=$2 OR EXISTS(SELECT 1 FROM moments m WHERE m.place_id=p.id AND m.id::text=$2) ORDER BY p.id LIMIT 201`, cityID, item.EntityID)
	if err != nil {
		return models.ReconciliationProposal{}, err
	}
	defer rows.Close()
	places := []models.IdentityPlace{}
	for rows.Next() {
		var p models.IdentityPlace
		var raw []byte
		if err := rows.Scan(&p.ID, &p.CityID, &p.Name, &p.Address, &p.Lat, &p.Lng, &p.Status, &raw); err != nil {
			return models.ReconciliationProposal{}, err
		}
		if err := json.Unmarshal(raw, &p.Moments); err != nil {
			return models.ReconciliationProposal{}, err
		}
		if cityID == "" && item.EntityID != "" {
			cityID = p.CityID
		}
		places = append(places, p)
	}
	if err := rows.Err(); err != nil {
		return models.ReconciliationProposal{}, err
	}
	rows.Close()
	complete := len(places) <= 200
	for _, p := range places {
		if len(p.Moments) > 100 {
			complete = false
		}
		for _, m := range p.Moments {
			if len(m.SourceURLs) > 20 {
				complete = false
			}
		}
	}
	claims := []models.IdentityClaim{}
	claimRows, err := s.pool.Query(ctx, `SELECT d.decision FROM reconciliation_records r JOIN reconciliation_decisions d ON d.record_id=r.id AND d.version=r.review_version WHERE d.decision->>'place_id' IN (SELECT id::text FROM places WHERE city_id::text=$1) ORDER BY r.id LIMIT 1001`, cityID)
	if err != nil {
		return models.ReconciliationProposal{}, err
	}
	defer claimRows.Close()
	for claimRows.Next() {
		var raw []byte
		var d models.IdentityDecision
		if err := claimRows.Scan(&raw); err != nil {
			return models.ReconciliationProposal{}, err
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return models.ReconciliationProposal{}, err
		}
		claims = append(claims, models.IdentityClaim{PlaceID: d.PlaceID, Aliases: d.Aliases, HistoricalAddresses: d.HistoricalAddresses})
	}
	if err := claimRows.Err(); err != nil {
		return models.ReconciliationProposal{}, err
	}
	complete = complete && len(claims) <= 1000
	p := reconcile.Propose(item, cityID, places, claims, complete)
	if item.Identity != nil && item.Identity.CityID != "" && item.Identity.CityID != cityID {
		p.Outcome = "needs_review"
		p.Issues = append(p.Issues, "unknown_city")
	}
	for _, c := range cities {
		if c.ID == cityID && item.Lat != nil && item.Lng != nil && !c.BBox.Contains(*item.Lat, *item.Lng) {
			p.Outcome = "needs_review"
			p.Issues = append(p.Issues, "outside_city_boundary")
		}
	}
	// Approval binds the original canonical candidate to its curated entity even
	// if the curator corrected its name or address. Conflicting old leads need
	// review and cannot create a replacement entity.
	if p.Outcome == "new_place" {
		var approved string
		err := s.pool.QueryRow(ctx, `SELECT approved_entity_id::text FROM candidates WHERE source_name='brief_reconciliation' AND external_id=$1 AND status='approved' AND approved_entity_id IS NOT NULL`, p.PlaceKey).Scan(&approved)
		if err == nil {
			p.PlaceID = approved
			p.Outcome = "needs_review"
			p.Issues = append(p.Issues, "approved_identity_changed")
		} else if !errors.Is(mapErr(err), ErrNotFound) {
			return models.ReconciliationProposal{}, err
		}
	}
	return p, nil
}

const reconciliationCols = `r.id,r.identity_key,r.proposal,r.place_candidate_id,r.moment_candidate_id,r.review_version,r.created_at,COALESCE(d.decision,'null'::jsonb),COALESCE((SELECT x.source FROM reconciliation_sources x WHERE x.record_id=r.id ORDER BY x.id DESC LIMIT 1),r.proposal)`

func scanReconciliation(row interface{ Scan(...any) error }) (*models.ReconciliationRecord, error) {
	r := &models.ReconciliationRecord{}
	var proposal, decision, assessment []byte
	if err := row.Scan(&r.ID, &r.Key, &proposal, &r.PlaceCandidateID, &r.MomentCandidateID, &r.ReviewVersion, &r.CreatedAt, &decision, &assessment); err != nil {
		return nil, mapErr(err)
	}
	if err := json.Unmarshal(proposal, &r.Proposal); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(decision, &r.Decision); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(assessment, &r.Assessment); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) reconciliationRecord(ctx context.Context, id string) (*models.ReconciliationRecord, error) {
	return scanReconciliation(s.pool.QueryRow(ctx, `SELECT `+reconciliationCols+` FROM reconciliation_records r LEFT JOIN reconciliation_decisions d ON d.record_id=r.id AND d.version=r.review_version WHERE r.id=$1`, id))
}

// ListReconciliation keeps source payloads behind the curator boundary. A brief
// lists only identities linked to it, even when another brief shares a record.
func (s *Store) ListReconciliation(ctx context.Context, briefID string, limit, offset int) ([]models.ReconciliationRecord, error) {
	if _, err := s.GetRouteBrief(ctx, briefID, ""); err != nil {
		return nil, err
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `SELECT `+reconciliationCols+` FROM reconciliation_records r LEFT JOIN reconciliation_decisions d ON d.record_id=r.id AND d.version=r.review_version WHERE EXISTS(SELECT 1 FROM reconciliation_sources x WHERE x.record_id=r.id AND x.brief_id=$1) ORDER BY r.created_at,r.id LIMIT $2 OFFSET $3`, briefID, clamp(limit), offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ReconciliationRecord{}
	for rows.Next() {
		r, err := scanReconciliation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	return out, nil
}

// ListReconciliationSources pages retained provenance without multiplying source
// payloads into every record in the review queue.
func (s *Store) ListReconciliationSources(ctx context.Context, briefID, id string, limit, offset int) ([]models.ReconciliationSource, error) {
	if !isUUID(briefID) || !isUUID(id) {
		return nil, ErrNotFound
	}
	var linked bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reconciliation_sources WHERE record_id=$1 AND brief_id=$2)`, id, briefID).Scan(&linked); err != nil {
		return nil, err
	}
	if !linked {
		return nil, ErrNotFound
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `SELECT brief_id,run_id,source FROM reconciliation_sources WHERE record_id=$1 AND brief_id=$2 ORDER BY id DESC LIMIT $3 OFFSET $4`, id, briefID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ReconciliationSource{}
	for rows.Next() {
		var x models.ReconciliationSource
		var raw []byte
		if err := rows.Scan(&x.BriefID, &x.RunID, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &x.Proposal); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ReconcileBrief lets a curator supply sourced structured facts at an expected
// brief version. It never updates an established place or moment.
func (s *Store) ReconcileBrief(ctx context.Context, briefID string, in models.ReconciliationRequest) (*models.ReconciliationRecord, error) {
	if len(in.Validate()) > 0 {
		return nil, ErrInvalidReference
	}
	var recordID string
	err := s.Transaction(ctx, func(tx *Store) error {
		if !isUUID(briefID) {
			return ErrNotFound
		}
		b, err := scanBrief(tx.pool.QueryRow(ctx, `SELECT `+briefCols+` FROM route_briefs WHERE id=$1 FOR UPDATE`, briefID))
		if err != nil {
			return err
		}
		if b.Version != in.Version {
			return fmt.Errorf("%w: brief changed; reload before reconciling", ErrConflict)
		}
		p, err := tx.PrepareReconciliation(ctx, b.Submitted, in.Item)
		if err != nil {
			return err
		}
		records, err := tx.commitReconciliation(ctx, b, "", []models.ReconciliationProposal{p})
		if err != nil {
			return err
		}
		recordID = records[0].ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.reconciliationRecord(ctx, recordID)
}

// commitReconciliation runs after the caller's brief/run/job fences. A single
// transaction advisory lock serializes identity proposals across overlapping briefs.
// This bounded private stage does not lock or rewrite published collection rows.
func (s *Store) commitReconciliation(ctx context.Context, b *models.RouteBrief, runID string, proposals []models.ReconciliationProposal) ([]models.ReconciliationRecord, error) {
	if len(proposals) > 20 {
		return nil, ErrInvalidReference
	}
	if _, err := s.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(743892,28)`); err != nil {
		return nil, err
	}
	sort.SliceStable(proposals, func(i, j int) bool { return proposals[i].Key < proposals[j].Key })
	out := []models.ReconciliationRecord{}
	for _, input := range proposals {
		// Recompute against current curator claims and collection metadata inside
		// the serialization boundary; external processors cannot inject target IDs.
		p, err := s.PrepareReconciliation(ctx, b.Submitted, input.Item)
		if err != nil {
			return nil, err
		}
		if len((models.ReconciliationRequest{Version: b.Version, Item: p.Item}).Validate()) > 0 {
			return nil, ErrInvalidReference
		}
		if p.Outcome == "new_place" && p.Item.Identity != nil {
			name := p.Item.Identity.PlaceName
			if name == "" {
				name = p.Item.Title
			}
			var conflict bool
			if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reconciliation_records WHERE place_candidate_id IS NOT NULL AND proposal->>'place_key'<>$1 AND proposal->>'city_id'=$2 AND (trim(regexp_replace(lower(proposal->'item'->'identity'->>'address'),'[^[:alnum:]]+',' ','g'))=$3 OR trim(regexp_replace(lower(COALESCE(NULLIF(proposal->'item'->'identity'->>'place_name',''),proposal->'item'->>'title')),'[^[:alnum:]]+',' ','g'))=$4))`, p.PlaceKey, p.CityID, sources.DiscoveryTerms(p.Item.Identity.Address), sources.DiscoveryTerms(name)).Scan(&conflict); err != nil {
				return nil, err
			}
			if conflict {
				p.Outcome = "needs_review"
				p.Issues = append(p.Issues, "pending_identity_conflict")
			}
		}
		raw, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO reconciliation_records(identity_key,proposal) VALUES($1,$2) ON CONFLICT(identity_key) DO NOTHING`, p.Key, raw); err != nil {
			return nil, err
		}
		var id string
		if err := s.pool.QueryRow(ctx, `SELECT id FROM reconciliation_records WHERE identity_key=$1 FOR UPDATE`, p.Key).Scan(&id); err != nil {
			return nil, err
		}
		r, err := s.reconciliationRecord(ctx, id)
		if err != nil {
			return nil, err
		}
		if r.Decision != nil {
			p.Outcome = r.Decision.Outcome
			p.PlaceID = r.Decision.PlaceID
			p.MomentID = r.Decision.MomentID
		} else if r.Proposal.Item.Identity != nil && p.Item.Identity != nil {
			old := *r.Proposal.Item.Identity
			now := p.Item.Identity
			if old.DateStart == "" {
				var facts []byte
				err := s.pool.QueryRow(ctx, `SELECT source->'item'->'identity' FROM reconciliation_sources WHERE record_id=$1 AND COALESCE(source->'item'->'identity'->>'date_start','')<>'' ORDER BY id LIMIT 1`, r.ID).Scan(&facts)
				if err == nil {
					if err := json.Unmarshal(facts, &old); err != nil {
						return nil, err
					}
				} else if !errors.Is(mapErr(err), ErrNotFound) {
					return nil, err
				}
			}
			if reconcile.ConflictingDates(old, *now) || slices.Contains(r.Assessment.Issues, "conflicting_discoveries") {
				p.Outcome = "needs_review"
				p.Issues = append(p.Issues, "conflicting_discoveries")
			}
		}
		raw, err = json.Marshal(p)
		if err != nil {
			return nil, err
		}
		// Every distinct source, query and inferred fact is retained. A replay of
		// identical bytes is idempotent; nothing overwrites earlier provenance.
		h := sha256.Sum256(append(append([]byte{}, raw...), []byte("\x00"+runID)...))
		sourceKey := hex.EncodeToString(h[:])
		if _, err := s.pool.Exec(ctx, `INSERT INTO reconciliation_sources(record_id,brief_id,run_id,source_key,source) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5) ON CONFLICT DO NOTHING`, id, b.ID, runID, sourceKey, raw); err != nil {
			return nil, err
		}
		if err := s.reconciliationCandidates(ctx, b, r, p); err != nil {
			return nil, err
		}
		r, err = s.reconciliationRecord(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, nil
}

func (s *Store) identityCandidate(ctx context.Context, b *models.RouteBrief, key string, kind models.CandidateKind, placeID string, payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var id string
	err = s.pool.QueryRow(ctx, `INSERT INTO candidates(kind,origin,source_name,external_id,submitted_by_account_id,target_place_id,payload) VALUES($1,'draft','brief_reconciliation',$2,$3,NULLIF($4,'')::uuid,$5) ON CONFLICT(source_name,external_id) WHERE source_name IS NOT NULL AND external_id IS NOT NULL DO NOTHING RETURNING id`, kind, key, b.OwnerAccountID, placeID, raw).Scan(&id)
	if errors.Is(mapErr(err), ErrNotFound) {
		err = s.pool.QueryRow(ctx, `SELECT id FROM candidates WHERE source_name='brief_reconciliation' AND external_id=$1`, key).Scan(&id)
	}
	return id, mapErr(err)
}

func (s *Store) reconciliationCandidates(ctx context.Context, b *models.RouteBrief, r *models.ReconciliationRecord, p models.ReconciliationProposal) error {
	if p.Outcome == "needs_review" || p.Outcome == "rejected" || p.Outcome == "existing_place_existing_moment" {
		return nil
	}
	f := models.IdentityFacts{}
	if p.Item.Identity != nil {
		f = *p.Item.Identity
	}
	placeID := p.PlaceID
	if p.Outcome == "new_place" {
		name := f.PlaceName
		if name == "" {
			name = p.Item.Title
		}
		if p.Item.Lat == nil || p.Item.Lng == nil || p.CityID == "" || f.Address == "" {
			return nil
		}
		id, err := s.identityCandidate(ctx, b, p.PlaceKey, models.CandidateKindPlace, "", models.CreatePlaceRequest{CityID: p.CityID, Name: name, Address: f.Address, Latitude: *p.Item.Lat, Longitude: *p.Item.Lng})
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, `UPDATE reconciliation_records SET place_candidate_id=COALESCE(place_candidate_id,$2) WHERE id=$1`, r.ID, id); err != nil {
			return err
		}
		c, err := s.GetCandidate(ctx, id)
		if err != nil {
			return err
		}
		if c.ApprovedEntityID == nil {
			return nil
		}
		placeID = *c.ApprovedEntityID
	}
	title := f.MomentTitle
	if title == "" && p.Item.Kind == models.CandidateKindMoment {
		title = p.Item.Title
	}
	if placeID != "" && title != "" && p.Outcome != "existing_place" {
		id, err := s.identityCandidate(ctx, b, reconcile.Key("moment", placeID, title), models.CandidateKindMoment, placeID, models.CreateMomentRequest{PlaceID: placeID, Title: title, DateStart: f.DateStart, DatePrecision: f.DatePrecision})
		if err != nil {
			return err
		}
		_, err = s.pool.Exec(ctx, `UPDATE reconciliation_records SET moment_candidate_id=COALESCE(moment_candidate_id,$2) WHERE id=$1`, r.ID, id)
		return err
	}
	return nil
}

// DecideReconciliation appends an optimistic curator decision. It only links
// existing entities and aliases; it cannot merge, publish or replace their content.
func (s *Store) DecideReconciliation(ctx context.Context, briefID, id, reviewer string, in models.IdentityDecisionRequest) (*models.ReconciliationRecord, error) {
	if !isUUID(briefID) || !isUUID(id) {
		return nil, ErrNotFound
	}
	if len(in.Validate()) > 0 {
		return nil, ErrInvalidReference
	}
	err := s.Transaction(ctx, func(tx *Store) error {
		var brief string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM route_briefs WHERE id=$1 FOR UPDATE`, briefID).Scan(&brief); err != nil {
			return mapErr(err)
		}
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(743892,28)`); err != nil {
			return err
		}
		var version int
		if err := tx.pool.QueryRow(ctx, `SELECT review_version FROM reconciliation_records WHERE id=$1 AND EXISTS(SELECT 1 FROM reconciliation_sources WHERE record_id=$1 AND brief_id=$2) FOR UPDATE`, id, briefID).Scan(&version); err != nil {
			return mapErr(err)
		}
		if version != in.Version {
			return fmt.Errorf("%w: identity decision changed; reload before review", ErrConflict)
		}
		if in.PlaceID != "" {
			var exists bool
			if err := tx.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM places p WHERE p.id::text=$1 AND ($2='' OR EXISTS(SELECT 1 FROM moments m WHERE m.id::text=$2 AND m.place_id=p.id)))`, in.PlaceID, in.MomentID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return ErrInvalidReference
			}
		}
		d := models.IdentityDecision{Version: version + 1, Outcome: in.Outcome, PlaceID: in.PlaceID, MomentID: in.MomentID, Aliases: in.Aliases, HistoricalAddresses: in.HistoricalAddresses, Note: in.Note, ReviewerID: reviewer}
		raw, err := json.Marshal(d)
		if err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `INSERT INTO reconciliation_decisions(record_id,version,decision,reviewer_id) VALUES($1,$2,$3,$4)`, id, d.Version, raw, reviewer); err != nil {
			return mapErr(err)
		}
		_, err = tx.pool.Exec(ctx, `UPDATE reconciliation_records SET review_version=$2 WHERE id=$1`, id, d.Version)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.reconciliationRecord(ctx, id)
}
