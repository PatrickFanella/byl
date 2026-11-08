package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"invisible-museum/backend/internal/models"
)

const correctionCols = `id,report_id,investigation_id,tour_id,previous_revision_id,revision_id,comparison_version,corridor_id,reviewed_by,note,temporary,review_on,COALESCE(review_on<=CURRENT_DATE,false),potential_tours,created_at`

func scanCorrection(row interface{ Scan(...any) error }) (*models.RouteCorrection, error) {
	c := &models.RouteCorrection{}
	var date *time.Time
	var raw []byte
	if err := row.Scan(&c.ID, &c.ReportID, &c.InvestigationID, &c.TourID, &c.PreviousRevisionID, &c.RevisionID, &c.ComparisonVersion, &c.CorridorID, &c.ReviewedBy, &c.Note, &c.Temporary, &date, &c.ReviewDue, &raw, &c.CreatedAt); err != nil {
		return nil, mapErr(err)
	}
	c.ReviewOn = models.DateFromTime(date)
	if err := json.Unmarshal(raw, &c.PotentiallyAffectedTours); err != nil {
		return nil, err
	}
	return c, nil
}

// RouteCorrectionComparison returns one consistent private view. Ordinary draft
// preparation remains explicit; neither observations nor investigation routes are applied.
func (s *Store) RouteCorrectionComparison(ctx context.Context, report, investigation string) (*models.RouteCorrectionComparison, error) {
	tx, err := s.rawPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scoped := &Store{pool: transactionDB{tx}, rawPool: s.rawPool}
	p, err := scoped.routeCorrectionComparison(ctx, report, investigation)
	if err != nil {
		return nil, err
	}
	return p, tx.Commit(ctx)
}

func (s *Store) routeCorrectionComparison(ctx context.Context, report, investigation string) (*models.RouteCorrectionComparison, error) {
	p := &models.RouteCorrectionComparison{PathReports: map[models.TravelMode]*models.PathQualityReport{}, Blockers: []string{}}
	var err error
	p.Report, err = s.GetRouteReport(ctx, report, "")
	if err != nil {
		return nil, err
	}
	p.Investigation, err = s.GetRouteInvestigation(ctx, report, investigation)
	if err != nil {
		return nil, err
	}
	p.Before, err = s.GetTourRevision(ctx, p.Report.Submitted.TourID, p.Report.Submitted.RevisionID)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err = s.pool.QueryRow(ctx, `SELECT byl_tour_snapshot(t.id) FROM tours t JOIN tour_revision_state st ON st.tour_id=t.id WHERE t.id=$1 AND st.deleted_at IS NULL`, p.Before.TourID).Scan(&raw); err != nil {
		return nil, mapErr(err)
	}
	if err = json.Unmarshal(raw, &p.Prepared); err != nil {
		return nil, err
	}
	working, err := s.GetTour(ctx, p.Before.TourID, false)
	if err != nil {
		return nil, err
	}
	// The generated snapshot does not include the current released pointer/version.
	p.Prepared.Tour.DraftVersion = working.DraftVersion
	p.Prepared.Tour.RouteVersion = working.RouteVersion
	p.Prepared.Tour.RevisionID = working.RevisionID
	p.Prepared.Tour.RevisionNumber = working.RevisionNumber
	if working.RevisionID == nil || *working.RevisionID != p.Before.ID {
		p.Blockers = append(p.Blockers, "published_revision_changed: investigate a report on the current release")
	}
	if !p.Report.Status.CanTransitionTo(models.BriefAccepted) {
		p.Blockers = append(p.Blockers, "report_not_reviewable")
	}
	if p.Investigation.Result == nil {
		p.Blockers = append(p.Blockers, "investigation_incomplete")
	}
	for _, observation := range p.Investigation.Input.Observations {
		current, err := s.GetRouteReport(ctx, observation.Report.ID, "")
		if err != nil {
			return nil, err
		}
		if current.Version != observation.Report.Version {
			p.Blockers = append(p.Blockers, "investigated_report_changed: request a fresh investigation")
		}
	}
	corridor := working.Corridor
	mode := p.Report.Submitted.Mode
	if corridor == nil || corridor.Intent == nil || !slices.ContainsFunc(corridor.Intent.Routes, func(r models.ModeCorridor) bool { return r.Mode == mode }) {
		p.Blockers = append(p.Blockers, "durable_routing_intent_required: approve a mode-specific corridor before release")
	}
	blockers, err := s.TourPublishBlockers(ctx, working.ID)
	if err != nil {
		return nil, err
	}
	p.Blockers = append(p.Blockers, blockers...)
	for _, mode := range working.Modes {
		quality, err := s.GetPathQualityReport(ctx, working.ID, mode, "")
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		p.PathReports[mode] = quality
	}
	p.PotentiallyAffectedTours, err = s.correctionImpacts(ctx, p.Before, working, p.Report.Submitted)
	if err != nil {
		return nil, err
	}
	if len(p.PotentiallyAffectedTours) > 200 {
		p.Blockers = append(p.Blockers, "impact_limit: narrow the shared-reference review before publication")
	}
	p.ReadyToPublish = len(p.Blockers) == 0
	raw, err = json.Marshal(p)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	p.Version = hex.EncodeToString(digest[:])
	return p, nil
}

// correctionImpacts flags exact retained source identities. Nearby geometry is
// not connectivity. Flags are private review leads and never edit another tour.
func (s *Store) correctionImpacts(ctx context.Context, before *models.TourRevision, working *models.TourDetail, in models.RouteReportInput) ([]string, error) {
	ways := []int64{}
	for _, tour := range []*models.TourDetail{&before.Snapshot.Tour, working} {
		if tour.Corridor == nil || tour.Corridor.Intent == nil {
			continue
		}
		selected := map[string]bool{}
		for _, r := range tour.Corridor.Intent.Routes {
			if r.Mode == in.Mode {
				for _, p := range r.Path {
					selected[p.SegmentID] = true
				}
			}
		}
		for _, segment := range tour.Corridor.Intent.Segments {
			if selected[segment.ID] && segment.Network != nil {
				for _, way := range segment.Network.Ways {
					ways = append(ways, way.ID)
				}
			}
		}
	}
	places := []string{}
	if in.Affected.Kind != "segment" {
		for _, stop := range before.Snapshot.Tour.Stops {
			if stop.ID == in.Affected.StopID {
				places = append(places, stop.Place.ID)
			}
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT st.tour_id FROM tour_revision_state st JOIN tour_revisions r ON r.id=st.current_revision_id WHERE st.tour_id<>$1 AND st.deleted_at IS NULL AND (
 EXISTS(SELECT 1 FROM jsonb_array_elements(COALESCE(r.snapshot#>'{tour,stops}','[]'::jsonb)) stop WHERE stop#>>'{place,id}'=ANY($3::text[]))
 OR EXISTS(SELECT 1 FROM jsonb_array_elements(COALESCE(r.snapshot#>'{tour,corridor,intent,segments}','[]'::jsonb)) segment CROSS JOIN LATERAL jsonb_array_elements(COALESCE(segment#>'{network,ways}','[]'::jsonb)) way WHERE (way->>'id')::bigint=ANY($2::bigint[]))) ORDER BY st.tour_id LIMIT 201`, working.ID, ways, places)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// ApproveRouteCorrection rechecks the full comparison under serializable
// isolation, releases the prepared draft and notifies only the selected report.
func (s *Store) ApproveRouteCorrection(ctx context.Context, report, curator string, in models.ApproveRouteCorrectionRequest) (*models.RouteCorrection, error) {
	if !isUUID(report) || !isUUID(in.InvestigationID) {
		return nil, ErrNotFound
	}
	if len(in.Validate(time.Time{})) > 0 {
		return nil, ErrInvalidReference
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	requestHash := hex.EncodeToString(digest[:])
	tx, err := s.rawPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scoped := &Store{pool: transactionDB{tx}, rawPool: s.rawPool}
	result, err := scoped.approveRouteCorrection(ctx, report, curator, in, requestHash)
	if err == nil {
		err = tx.Commit(ctx)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01") {
		return nil, fmt.Errorf("%w: correction changed concurrently; reload before approving", ErrConflict)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) approveRouteCorrection(ctx context.Context, report, curator string, in models.ApproveRouteCorrectionRequest, requestHash string) (*models.RouteCorrection, error) {
	seed, err := s.GetRouteReport(ctx, report, "")
	if err != nil {
		return nil, err
	}
	var locked string
	if err = s.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, seed.Submitted.TourID).Scan(&locked); err != nil {
		return nil, mapErr(err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT id FROM route_reports WHERE id=$1 FOR UPDATE`, report).Scan(&locked); err != nil {
		return nil, mapErr(err)
	}
	var previousHash, previousAuthor string
	err = s.pool.QueryRow(ctx, `SELECT request_sha256,reviewed_by FROM route_corrections WHERE report_id=$1`, report).Scan(&previousHash, &previousAuthor)
	if err == nil {
		if previousHash != requestHash || previousAuthor != curator {
			return nil, fmt.Errorf("%w: report already released through a different review", ErrConflict)
		}
		return s.GetRouteCorrection(ctx, report)
	}
	if !errors.Is(mapErr(err), ErrNotFound) {
		return nil, err
	}
	if len(in.Validate(time.Now().UTC())) > 0 {
		return nil, ErrInvalidReference
	}
	p, err := s.routeCorrectionComparison(ctx, report, in.InvestigationID)
	if err != nil {
		return nil, err
	}
	if in.Version != p.Version || !p.ReadyToPublish {
		return nil, fmt.Errorf("%w: correction comparison changed or has unresolved tasks; reload before approval", ErrConflict)
	}
	tour := &p.Prepared.Tour
	revision, err := s.publishRevisionTx(ctx, tour.ID, curator, models.PublishRevisionRequest{DraftVersion: tour.DraftVersion, CurrentRevisionID: &p.Before.ID}, true)
	if err != nil {
		return nil, err
	}
	impacts, err := json.Marshal(p.PotentiallyAffectedTours)
	if err != nil {
		return nil, err
	}
	var review any
	if in.ReviewOn != nil {
		review = in.ReviewOn.Time
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO route_corrections(report_id,investigation_id,tour_id,previous_revision_id,revision_id,comparison_version,corridor_id,reviewed_by,note,temporary,review_on,potential_tours,request_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, report, in.InvestigationID, tour.ID, p.Before.ID, revision.ID, in.Version, tour.Corridor.ID, curator, in.Note, in.Temporary, review, impacts, requestHash)
	if err != nil {
		return nil, mapErr(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE route_reports SET status='accepted',reason=$2,version=version+1,updated_at=now() WHERE id=$1`, report, in.Note); err != nil {
		return nil, err
	}
	return s.GetRouteCorrection(ctx, report)
}

// GetRouteCorrection reads retained private provenance; transports enforce access.
func (s *Store) GetRouteCorrection(ctx context.Context, report string) (*models.RouteCorrection, error) {
	if !isUUID(report) {
		return nil, ErrNotFound
	}
	return scanCorrection(s.pool.QueryRow(ctx, `SELECT `+correctionCols+` FROM route_corrections WHERE report_id=$1`, report))
}

// ListRouteCorrections exposes reassessment tasks without changing any route.
func (s *Store) ListRouteCorrections(ctx context.Context, due bool, limit, offset int) ([]models.RouteCorrection, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+correctionCols+` FROM route_corrections WHERE (NOT $1 OR review_on<=CURRENT_DATE) ORDER BY review_on NULLS LAST,created_at,id LIMIT $2 OFFSET $3`, due, clamp(limit), max(0, offset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []models.RouteCorrection{}
	for rows.Next() {
		c, err := scanCorrection(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *c)
	}
	return result, rows.Err()
}
