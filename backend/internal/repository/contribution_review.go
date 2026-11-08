package repository

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"invisible-museum/backend/internal/models"
	"time"
)

type ContributionReviewFilter struct {
	RevisionID, SegmentID, Direction string
	Mode                             models.TravelMode
	WindowStart                      time.Time
	Limit, Offset                    int
}

// ContributionReviewWindow anchors nonoverlapping UTC weeks on Monday.
func ContributionReviewWindow(now time.Time) time.Time {
	d := models.ContributionDay(now)
	return d.AddDate(0, 0, -(int(d.Weekday())+6)%7)
}

// ListContributionReviews uses a single read snapshot and stores no rollup/task
// history. Withdrawal and expiry affect the next read, including stale deep links.
func (s *Store) ListContributionReviews(ctx context.Context, f ContributionReviewFilter, now time.Time) ([]models.ContributionReview, error) {
	detail := f.RevisionID != ""
	week := ContributionReviewWindow(now)
	if detail && (!isUUID(f.RevisionID) || len(f.SegmentID) == 0 || len(f.SegmentID) > 120 || !f.Mode.Valid() || (f.Direction != "forward" && f.Direction != "reverse") || !f.WindowStart.Equal(ContributionReviewWindow(f.WindowStart)) || f.WindowStart.Before(week.AddDate(0, 0, -21)) || f.WindowStart.After(week)) {
		return nil, ErrInvalidReference
	}
	if detail {
		f.Limit = 1
		f.Offset = 0
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	tx, err := s.rawPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scoped := &Store{pool: transactionDB{tx}, rawPool: s.rawPool}
	out, err := scoped.contributionReviews(ctx, f, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) contributionReviews(ctx context.Context, f ContributionReviewFilter, now time.Time) ([]models.ContributionReview, error) {
	today := models.ContributionDay(now)
	first := ContributionReviewWindow(now).AddDate(0, 0, -21)
	var window any
	if !f.WindowStart.IsZero() {
		window = f.WindowStart
	}
	thresholds := models.ReviewContributionThresholds()
	rows, err := s.pool.Query(ctx, `SELECT c.tour_id,c.revision_id,c.mode,r.segment_id,r.direction,date_trunc('week',r.observation_period)::date,
      count(*),sum(r.sustained_departure_rejoins),count(DISTINCT c.owner_scope),count(DISTINCT c.owner_scope) FILTER (WHERE r.sustained_departure_rejoins=1)
      FROM route_contributions r JOIN route_contribution_consents c ON c.id=r.consent_id
      JOIN tour_revision_state st ON st.tour_id=c.tour_id AND st.deleted_at IS NULL
      WHERE c.revoked_on IS NULL AND c.expires_on>$1 AND r.expires_on>$1
      AND r.observation_period BETWEEN $2 AND $1 AND r.observation_period>=c.created_on
      AND r.schema_version=1 AND r.detector_version='uncalibrated-v1' AND NOT r.calibrated AND r.qualified_passages=1
      AND ($3='' OR c.revision_id::text=$3) AND ($4='' OR r.segment_id=$4) AND ($5='' OR c.mode=$5) AND ($6='' OR r.direction=$6)
      AND ($7::date IS NULL OR date_trunc('week',r.observation_period)::date=$7)
      GROUP BY c.tour_id,c.revision_id,c.mode,r.segment_id,r.direction,date_trunc('week',r.observation_period)::date
      HAVING $3<>'' OR (count(DISTINCT c.owner_scope)>=$8 AND count(DISTINCT c.owner_scope) FILTER (WHERE r.sustained_departure_rejoins=1)>=$9 AND sum(r.sustained_departure_rejoins)*100>=count(*)*$10)
      ORDER BY date_trunc('week',r.observation_period)::date DESC,c.revision_id,c.mode,r.segment_id,r.direction LIMIT $11 OFFSET $12`,
		today, first, f.RevisionID, f.SegmentID, f.Mode, f.Direction, window, thresholds.MinimumScopes, thresholds.MinimumDepartureScopes, thresholds.MinimumDeparturePercent, min(clamp(f.Limit), 100), f.Offset)
	if err != nil {
		return nil, err
	}
	out := []models.ContributionReview{}
	for rows.Next() {
		var item models.ContributionReview
		var start time.Time
		if err := rows.Scan(&item.TourID, &item.RevisionID, &item.Mode, &item.SegmentID, &item.Direction, &start, &item.QualifiedPassages, &item.DepartureRejoins, &item.ContributorScopes, &item.DepartureScopes); err != nil {
			rows.Close()
			return nil, err
		}
		item.WindowStart = models.Date{Time: start}
		out = append(out, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if f.RevisionID != "" && len(out) == 0 {
		var tour string
		if err := s.pool.QueryRow(ctx, `SELECT r.tour_id FROM tour_revisions r JOIN tour_revision_state st ON st.tour_id=r.tour_id WHERE r.id=$1 AND st.deleted_at IS NULL`, f.RevisionID).Scan(&tour); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, models.ContributionReview{TourID: tour, RevisionID: f.RevisionID, SegmentID: f.SegmentID, Mode: f.Mode, Direction: f.Direction, WindowStart: models.Date{Time: f.WindowStart}})
	}
	for i := range out {
		item := &out[i]
		revision, err := s.GetTourRevision(ctx, item.TourID, item.RevisionID)
		if err != nil {
			return nil, err
		}
		// Reject guessed/nonexistent or wrong-direction references even for a
		// no-longer-flagged deep link; don't silently use today's working route.
		summary := models.ContributionSummary{SegmentID: item.SegmentID, Mode: item.Mode, Direction: item.Direction}
		if !contributionSegment(revision, summary) {
			return nil, ErrNotFound
		}
		item.TourTitle = revision.Snapshot.Tour.Title
		item.RevisionNumber = revision.Number
		item.WindowEnd = models.Date{Time: item.WindowStart.AddDate(0, 0, 7)}
		through := item.WindowEnd.AddDate(0, 0, -1)
		if through.After(today) {
			through = today
		}
		item.ObservedThrough = models.Date{Time: through}
		item.Thresholds = thresholds
		item.Flagged = item.MeetsThreshold()
		if corridor := revision.Snapshot.Tour.Corridor; corridor != nil && corridor.Intent != nil {
			for _, segment := range corridor.Intent.Segments {
				if segment.ID == item.SegmentID {
					line := segment.Geometry
					item.Geometry = &line
					item.SegmentKind = segment.Kind
					break
				}
			}
		}
		item.Reports = []models.ContributionReviewReport{}
		// Report locations are story legs, not corridor IDs. These are same-
		// revision/mode leads, never asserted to match this logical section.
		reports, err := s.pool.Query(ctx, `SELECT id,status,submitted FROM route_reports WHERE revision_id=$1 AND submitted->>'mode'=$2 ORDER BY created_at DESC,id LIMIT 21`, item.RevisionID, item.Mode)
		if err != nil {
			return nil, err
		}
		for reports.Next() {
			var r models.ContributionReviewReport
			var raw []byte
			var submitted models.RouteReportInput
			if err := reports.Scan(&r.ID, &r.Status, &raw); err != nil {
				reports.Close()
				return nil, err
			}
			if err := json.Unmarshal(raw, &submitted); err != nil {
				reports.Close()
				return nil, err
			}
			r.Reason = submitted.Reason
			r.Affected = submitted.Affected
			item.Reports = append(item.Reports, r)
		}
		err = reports.Err()
		reports.Close()
		if err != nil {
			return nil, err
		}
		if len(item.Reports) > 20 {
			item.ReportsTruncated = true
			item.Reports = item.Reports[:20]
		}
	}
	return out, nil
}
