package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/sources"
)

const investigationCols = `id,report_id,job_id,created_by,input,result,created_at`

func scanInvestigation(row interface{ Scan(...any) error }) (*models.RouteInvestigation, error) {
	i := &models.RouteInvestigation{}
	var input, result []byte
	if err := row.Scan(&i.ID, &i.ReportID, &i.JobID, &i.CreatedBy, &input, &result, &i.CreatedAt); err != nil {
		return nil, mapErr(err)
	}
	if err := json.Unmarshal(input, &i.Input); err != nil {
		return nil, err
	}
	if len(result) > 0 {
		if err := json.Unmarshal(result, &i.Result); err != nil {
			return nil, err
		}
	}
	return i, nil
}

// RelatedRouteReports groups only exact frozen references. Nearby points and
// similar prose cannot establish that two reports describe the same crossing.
func (s *Store) RelatedRouteReports(ctx context.Context, id string, limit, offset int) ([]models.RouteReport, error) {
	r, err := s.GetRouteReport(ctx, id, "")
	if err != nil {
		return nil, err
	}
	affected, err := json.Marshal(r.Submitted.Affected)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+routeReportCols+` FROM route_reports WHERE revision_id=$1 AND submitted->>'mode'=$2 AND submitted->>'direction'=$3 AND submitted->'affected'=$4::jsonb AND id<>$5 ORDER BY created_at,id LIMIT $6 OFFSET $7`, r.Submitted.RevisionID, r.Submitted.Mode, r.Submitted.Direction, affected, id, clamp(limit), max(0, offset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RouteReport{}
	for rows.Next() {
		r, err := scanRouteReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func sameReportScope(a, b models.RouteReportInput) bool {
	return a.TourID == b.TourID && a.RevisionID == b.RevisionID && a.Mode == b.Mode && a.Direction == b.Direction && a.Affected == b.Affected
}

func observationGroups(reports []models.RouteReport) []models.InvestigationObservation {
	out := make([]models.InvestigationObservation, len(reports))
	parent, keys, hashes := make([]int, len(reports)), make([]string, len(reports)), make([]string, len(reports))
	for i, r := range reports {
		out[i].Report, parent[i] = r, i
		_, _, keys[i], _ = sources.NormalizeEvidenceURL(r.Submitted.SourceURL)
		in := r.Submitted
		in.SourceURL = ""
		in.Details = strings.Join(strings.Fields(strings.ToLower(in.Details)), " ")
		in.Better = strings.Join(strings.Fields(strings.ToLower(in.Better)), " ")
		raw, _ := json.Marshal(in)
		hashes[i] = string(raw)
	}
	root := func(i int) int {
		for parent[i] != i {
			i = parent[i]
		}
		return i
	}
	for i := range reports {
		for j := 0; j < i; j++ {
			if reports[i].OwnerAccountID == reports[j].OwnerAccountID || hashes[i] == hashes[j] || keys[i] != "" && keys[i] == keys[j] {
				a, b := root(i), root(j)
				parent[max(a, b)] = min(a, b)
			}
		}
	}
	for i := range out {
		if r := root(i); r != i {
			out[i].DuplicateOf = reports[r].ID
		}
	}
	return out
}

// QueueRouteInvestigation captures reports, a sourcing brief and source jobs in
// one transaction. It changes neither report dispositions nor tour working data.
func (s *Store) QueueRouteInvestigation(ctx context.Context, id, curator string, in models.RouteInvestigationRequest, key string) (*models.RouteInvestigation, error) {
	id = strings.ToLower(id)
	in.RelatedReportIDs = slices.Clone(in.RelatedReportIDs)
	if in.RelatedReportIDs == nil {
		in.RelatedReportIDs = []string{}
	}
	for j := range in.RelatedReportIDs {
		in.RelatedReportIDs[j] = strings.ToLower(in.RelatedReportIDs[j])
	}
	if in.Research != nil {
		research := *in.Research
		research.Normalize()
		in.Research = &research
	}
	if len(in.Validate()) > 0 || len(key) > 128 {
		return nil, ErrInvalidReference
	}
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	seen := map[string]bool{id: true}
	for _, related := range in.RelatedReportIDs {
		if !isUUID(related) || seen[related] {
			return nil, ErrInvalidReference
		}
		seen[related] = true
	}
	slices.Sort(in.RelatedReportIDs)
	raw, err := json.Marshal(struct {
		ID      string
		Request models.RouteInvestigationRequest
	}{id, in})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	var out *models.RouteInvestigation
	err = s.Transaction(ctx, func(tx *Store) error {
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,2233))`, curator); err != nil {
			return err
		}
		if key != "" {
			var prior, previousHash string
			err := tx.pool.QueryRow(ctx, `SELECT id,request_hash FROM route_investigations WHERE created_by=$1 AND request_key=$2`, curator, key).Scan(&prior, &previousHash)
			if err == nil {
				if previousHash != hash {
					return fmt.Errorf("%w: idempotency key was used for another investigation", ErrConflict)
				}
				out, err = tx.GetRouteInvestigation(ctx, id, prior)
				return err
			}
			if !errors.Is(mapErr(err), ErrNotFound) {
				return err
			}
		}
		seed, err := scanRouteReport(tx.pool.QueryRow(ctx, `SELECT `+routeReportCols+` FROM route_reports WHERE id=$1 FOR SHARE`, id))
		if err != nil {
			return err
		}
		if seed.Version != in.Version {
			return fmt.Errorf("%w: report changed; reload before investigating", ErrConflict)
		}
		reports := []models.RouteReport{*seed}
		for _, related := range in.RelatedReportIDs {
			r, err := scanRouteReport(tx.pool.QueryRow(ctx, `SELECT `+routeReportCols+` FROM route_reports WHERE id=$1 FOR SHARE`, related))
			if err != nil {
				return err
			}
			if !sameReportScope(seed.Submitted, r.Submitted) {
				return fmt.Errorf("%w: reports must match revision, mode, direction and affected reference", ErrInvalidReference)
			}
			reports = append(reports, *r)
		}
		revision, err := tx.GetTourRevision(ctx, seed.Submitted.TourID, seed.Submitted.RevisionID)
		if err != nil {
			return err
		}
		briefInput := models.RouteBriefInput{Topic: "Investigate route report", Area: revision.Snapshot.Tour.Title, Notes: "Private unverified route observations; inspect sources before drafting claims.", Modes: []models.TravelMode{seed.Submitted.Mode}}
		if in.Research != nil {
			briefInput = *in.Research
		}
		urls, unique := []string{}, map[string]bool{}
		for _, r := range reports {
			if r.Submitted.SourceURL != "" {
				urls = append(urls, r.Submitted.SourceURL)
			}
		}
		urls = append(urls, briefInput.SourceLinks...)
		briefInput.SourceLinks = []string{}
		for _, rawURL := range urls {
			url, _, sourceKey, err := sources.NormalizeEvidenceURL(rawURL)
			if err != nil {
				// Retain the original lead on the report; no unsafe request is queued.
				continue
			}
			if !unique[sourceKey] {
				unique[sourceKey] = true
				briefInput.SourceLinks = append(briefInput.SourceLinks, url)
			}
		}
		briefInput.Normalize()
		if len(briefInput.Validate()) > 0 {
			return ErrInvalidReference
		}
		brief, _, err := tx.CreateRouteBrief(ctx, curator, briefInput, "")
		if err != nil {
			return err
		}
		input := models.RouteInvestigationInput{Observations: observationGroups(reports), BriefID: brief.ID, EvidenceIDs: []string{}, Research: in.Research}
		for _, url := range briefInput.SourceLinks {
			_, reference, sourceKey, _ := sources.NormalizeEvidenceURL(url)
			e, err := tx.QueueSourceEvidence(ctx, brief.ID, models.EvidenceRequest{Version: brief.Version, URL: url, SourceKind: "page"}, reference, sourceKey, "")
			if err != nil {
				return err
			}
			input.EvidenceIDs = append(input.EvidenceIDs, e.ID)
		}
		var investigationID string
		if err := tx.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&investigationID); err != nil {
			return err
		}
		job, err := tx.EnqueueReturning(ctx, models.JobKindInvestigateRouteReport, models.InvestigateRouteReportPayload{InvestigationID: investigationID})
		if err != nil {
			return err
		}
		inputRaw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		var requestKey any
		if key != "" {
			requestKey = key
		}
		out, err = scanInvestigation(tx.pool.QueryRow(ctx, `INSERT INTO route_investigations(id,report_id,job_id,created_by,request_key,request_hash,input) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+investigationCols, investigationID, id, job.ID, curator, requestKey, hash, inputRaw))
		return err
	})
	return out, err
}

// GetRouteInvestigation and its list transport are curator-only. The report
// predicate prevents confusing checkpoints from two investigations' URLs.
func (s *Store) GetRouteInvestigation(ctx context.Context, report, id string) (*models.RouteInvestigation, error) {
	if !isUUID(id) || report != "" && !isUUID(report) {
		return nil, ErrNotFound
	}
	return scanInvestigation(s.pool.QueryRow(ctx, `SELECT `+investigationCols+` FROM route_investigations WHERE id=$1 AND ($2='' OR report_id::text=$2)`, id, report))
}

func (s *Store) ListRouteInvestigations(ctx context.Context, report string, limit, offset int) ([]models.RouteInvestigation, error) {
	if _, err := s.GetRouteReport(ctx, report, ""); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+investigationCols+` FROM route_investigations WHERE report_id=$1 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, report, clamp(limit), max(0, offset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RouteInvestigation{}
	for rows.Next() {
		i, err := scanInvestigation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func (s *Store) CompleteRouteInvestigation(ctx context.Context, id string, claim *models.Job, result models.RouteInvestigationResult) error {
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > 4<<20 || result.CheckedAt.IsZero() || result.IndependentCorroboration || !result.StoriesPreserved || (result.Status != "review" && result.Status != "needs_research") {
		return ErrInvalidReference
	}
	return s.Transaction(ctx, func(tx *Store) error {
		i, err := scanInvestigation(tx.pool.QueryRow(ctx, `SELECT `+investigationCols+` FROM route_investigations WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if i.Result != nil {
			return nil
		}
		var attempts int
		var status models.JobStatus
		if err := tx.pool.QueryRow(ctx, `SELECT attempts,status FROM jobs WHERE id=$1 FOR UPDATE`, i.JobID).Scan(&attempts, &status); err != nil {
			return mapErr(err)
		}
		if claim.ID != i.JobID || claim.Attempts != attempts || status != models.JobStatusRunning {
			return ErrResearchInactive
		}
		_, err = tx.pool.Exec(ctx, `UPDATE route_investigations SET result=$2 WHERE id=$1 AND result IS NULL`, id, raw)
		return err
	})
}
