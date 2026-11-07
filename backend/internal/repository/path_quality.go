package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"invisible-museum/backend/internal/models"
)

// captureCorridorInspections resolves actual retrieval snapshots before storing
// immutable proposal input. Public intent never carries these private excerpts.
func (s *Store) captureCorridorInspections(ctx context.Context, in models.CorridorProposalRequest) ([]models.PathInspection, error) {
	inspections := []models.PathInspection{}
	if len(in.QualityEvidence) > 192 || in.Remove && len(in.QualityEvidence) > 0 {
		return nil, ErrInvalidReference
	}
	seen := map[string]bool{}
	for _, ref := range in.QualityEvidence {
		key := ref.SegmentID + ":" + ref.Purpose + ":" + string(ref.Mode)
		if seen[key] || in.Intent == nil || !isUUID(ref.EvidenceID) || !slices.ContainsFunc(in.Intent.Segments, func(s models.CorridorSegment) bool { return s.ID == ref.SegmentID }) {
			return nil, ErrInvalidReference
		}
		seen[key] = true
		e, err := s.GetSourceEvidence(ctx, ref.EvidenceID)
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidReference
		}
		if err != nil {
			return nil, err
		}
		inspection, err := models.InspectPathEvidence(ref, e)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid retained path inspection", ErrInvalidReference)
		}
		inspections = append(inspections, inspection)
	}
	return inspections, nil
}

func (s *Store) saveCorridorInspections(ctx context.Context, proposal string, refs []models.CorridorEvidenceRef, inspections []models.PathInspection) error {
	if refs == nil {
		refs = []models.CorridorEvidenceRef{}
	}
	a, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	b, err := json.Marshal(inspections)
	if err != nil {
		return err
	}
	if len(a) > 262144 || len(b) > 524288 {
		return ErrInvalidReference
	}
	h := sha256.Sum256(append(append([]byte(nil), a...), b...))
	_, err = s.pool.Exec(ctx, `INSERT INTO corridor_quality_evidence(proposal_id,refs,inspections,content_sha256) VALUES($1,$2,$3,$4)`, proposal, a, b, hex.EncodeToString(h[:]))
	return mapErr(err)
}

func (s *Store) corridorInspectionRefs(ctx context.Context, proposal string) ([]models.CorridorEvidenceRef, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT refs FROM corridor_quality_evidence WHERE proposal_id=$1`, proposal).Scan(&raw)
	if errors.Is(mapErr(err), ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var refs []models.CorridorEvidenceRef
	err = json.Unmarshal(raw, &refs)
	return refs, err
}

// ApprovedPathInspections reads completed private proof for exactly the captured
// approved proposal. The immutable proposal version is already in the route hash.
func (s *Store) ApprovedPathInspections(ctx context.Context, tour *models.TourDetail) ([]models.PathInspection, error) {
	result := []models.PathInspection{}
	if tour.Corridor == nil || tour.Corridor.Intent == nil {
		return result, nil
	}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT e.inspections FROM corridor_quality_evidence e JOIN tour_corridor_proposals p ON p.id=e.proposal_id WHERE p.tour_id=$1 AND p.id=$2 AND p.status='approved'`, tour.ID, tour.Corridor.ID).Scan(&raw)
	if errors.Is(mapErr(err), ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &result)
	return result, err
}

// GetPathQualityReport returns private evidence only; callers enforce curator
// authorization. An optional retained ID allows review of a frozen checkpoint.
func (s *Store) GetPathQualityReport(ctx context.Context, tour string, mode models.TravelMode, id string) (*models.PathQualityReport, error) {
	if !isUUID(tour) || !mode.Valid() || id != "" && !isUUID(id) {
		return nil, ErrInvalidReference
	}
	var raw []byte
	var reportID string
	var current bool
	err := s.pool.QueryRow(ctx, `SELECT q.id,q.report,COALESCE(st.deleted_at IS NULL AND q.id=(SELECT report_id FROM tour_path_quality_current WHERE tour_id=$1 AND mode=$2) AND q.route_version=st.route_version AND q.input_hash=byl_route_input_hash($1,$2),false) FROM tour_path_quality_reports q JOIN tour_revision_state st ON st.tour_id=q.tour_id WHERE q.tour_id=$1 AND q.mode=$2 AND q.id=COALESCE(NULLIF($3,'')::uuid,(SELECT report_id FROM tour_path_quality_current WHERE tour_id=$1 AND mode=$2))`, tour, mode, id).Scan(&reportID, &raw, &current)
	if err != nil {
		return nil, mapErr(err)
	}
	var report models.PathQualityReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, err
	}
	report.ReportID = reportID
	report.InputCurrent = current
	return &report, nil
}

func (s *Store) savePathQuality(ctx context.Context, tour string, version int64, report models.PathQualityReport) error {
	report.ReportID = ""
	if !report.Valid() {
		return ErrInvalidReference
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO tour_path_quality_reports(tour_id,mode,route_version,input_hash,report,content_sha256) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, tour, report.Mode, version, report.InputHash, raw, hex.EncodeToString(digest[:])).Scan(&id); err != nil {
		return mapErr(err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO tour_path_quality_current(tour_id,mode,report_id) VALUES($1,$2,$3) ON CONFLICT(tour_id,mode) DO UPDATE SET report_id=excluded.report_id`, tour, report.Mode, id)
	return mapErr(err)
}
