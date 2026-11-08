package repository

import (
	"context"
	"fmt"
	"sort"

	"invisible-museum/backend/internal/models"
)

const revisionProgressCols = `p.account_id,r.tour_id,p.revision_id,p.mode,p.visited_stop_ids,p.started_at,p.completed_at,p.updated_at`

func scanRevisionProgress(row interface{ Scan(...any) error }) (*models.RevisionProgress, error) {
	p := &models.RevisionProgress{}
	if err := row.Scan(&p.AccountID, &p.TourID, &p.RevisionID, &p.Mode, &p.VisitedStopIDs, &p.StartedAt, &p.CompletedAt, &p.UpdatedAt); err != nil {
		return nil, mapErr(err)
	}
	if p.VisitedStopIDs == nil {
		p.VisitedStopIDs = []string{}
	}
	return p, nil
}

// GetRevisionProgress reads only the requested account and immutable revision.
func (s *Store) GetRevisionProgress(ctx context.Context, account, revision string) (*models.RevisionProgress, error) {
	if !isUUID(revision) {
		return nil, ErrNotFound
	}
	return scanRevisionProgress(s.pool.QueryRow(ctx, `SELECT `+revisionProgressCols+` FROM tour_revision_progress p JOIN tour_revisions r ON r.id=p.revision_id WHERE p.account_id=$1 AND p.revision_id=$2`, account, revision))
}

// PutRevisionProgress merges stable IDs only within one revision; it never carries
// position-based visits to a new release. An explicit reset clears that trip.
func (s *Store) PutRevisionProgress(ctx context.Context, in models.RevisionProgress, reset bool) (*models.RevisionProgress, error) {
	revision, err := s.GetTourRevision(ctx, in.TourID, in.RevisionID)
	if err != nil {
		return nil, err
	}
	if err := ValidateRevisionVisits(revision, in.Mode, in.VisitedStopIDs); err != nil {
		return nil, err
	}
	if in.VisitedStopIDs == nil {
		in.VisitedStopIDs = []string{}
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO tour_revision_progress(account_id,revision_id,mode,visited_stop_ids,completed_at)
	 VALUES($1,$2,$3,ARRAY(SELECT DISTINCT id FROM unnest($4::uuid[]) id ORDER BY id),$5)
	 ON CONFLICT(account_id,revision_id) DO UPDATE SET mode=excluded.mode,
	 visited_stop_ids=CASE WHEN $6 THEN excluded.visited_stop_ids ELSE ARRAY(SELECT DISTINCT id FROM unnest(tour_revision_progress.visited_stop_ids||excluded.visited_stop_ids) id ORDER BY id) END,
	 completed_at=CASE WHEN $6 THEN excluded.completed_at ELSE COALESCE(tour_revision_progress.completed_at,excluded.completed_at) END,
	 started_at=CASE WHEN $6 THEN now() ELSE tour_revision_progress.started_at END,updated_at=now()`, in.AccountID, in.RevisionID, in.Mode, in.VisitedStopIDs, in.CompletedAt, reset)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetRevisionProgress(ctx, in.AccountID, in.RevisionID)
}

func (s *Store) legacyProgressPin(ctx context.Context, account, tour string, reset bool) (string, error) {
	var id string
	err := s.Transaction(ctx, func(tx *Store) error {
		// Serialize the first visit/reset as well as the pin choice. Otherwise an
		// INSERT waiting on a competing first visit can miss its progress row.
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1||':'||$2,1731))`, account, tour); err != nil {
			return err
		}
		var err error
		id, err = tx.legacyProgressPinTx(ctx, account, tour, reset)
		return err
	})
	return id, err
}

func (s *Store) legacyProgressPinTx(ctx context.Context, account, tour string, reset bool) (string, error) {
	if !isUUID(tour) {
		return "", ErrNotFound
	}
	if _, err := s.ResolveTourID(ctx, tour); err != nil {
		return "", err
	}
	query := `INSERT INTO legacy_revision_pins(account_id,tour_id,revision_id) SELECT $1,tour_id,current_revision_id FROM tour_revision_state WHERE tour_id=$2 AND current_revision_id IS NOT NULL AND deleted_at IS NULL ON CONFLICT(account_id,tour_id) DO UPDATE SET revision_id=excluded.revision_id WHERE NOT EXISTS(SELECT 1 FROM tour_revision_progress p WHERE p.account_id=legacy_revision_pins.account_id AND p.revision_id=legacy_revision_pins.revision_id)`
	if reset {
		query = `INSERT INTO legacy_revision_pins(account_id,tour_id,revision_id) SELECT $1,tour_id,current_revision_id FROM tour_revision_state WHERE tour_id=$2 AND current_revision_id IS NOT NULL AND deleted_at IS NULL ON CONFLICT(account_id,tour_id) DO UPDATE SET revision_id=excluded.revision_id`
	}
	if _, err := s.pool.Exec(ctx, query, account, tour); err != nil {
		return "", mapErr(err)
	}
	var id string
	if err := s.pool.QueryRow(ctx, `SELECT revision_id FROM legacy_revision_pins WHERE account_id=$1 AND tour_id=$2`, account, tour).Scan(&id); err != nil {
		if mapErr(err) == ErrNotFound {
			return "", fmt.Errorf("%w: a published tour revision is not ready", ErrConflict)
		}
		return "", err
	}
	return id, nil
}

func legacyProjection(p *models.RevisionProgress, r *models.TourRevision) *models.TourProgress {
	visited := map[string]bool{}
	for _, id := range p.VisitedStopIDs {
		visited[id] = true
	}
	positions := []int{}
	for _, stop := range r.Snapshot.Tour.Stops {
		if visited[stop.ID] {
			positions = append(positions, stop.Position)
		}
	}
	sort.Ints(positions)
	return &models.TourProgress{AccountID: p.AccountID, TourID: p.TourID, RevisionID: p.RevisionID, Mode: p.Mode, VisitedPositions: positions, StartedAt: p.StartedAt, CompletedAt: p.CompletedAt, UpdatedAt: p.UpdatedAt}
}

// GetProgress projects a legacy trip against its pinned revision, not current stop positions.
func (s *Store) GetProgress(ctx context.Context, account, tour string) (*models.TourProgress, error) {
	id, err := s.legacyProgressPin(ctx, account, tour, false)
	if err != nil {
		return nil, err
	}
	p, err := s.GetRevisionProgress(ctx, account, id)
	if err != nil {
		return nil, err
	}
	r, err := s.GetTourRevision(ctx, tour, id)
	if err != nil {
		return nil, err
	}
	return legacyProjection(p, r), nil
}

// PutProgress translates legacy positions only through the trip's pinned snapshot.
// Reset explicitly moves the legacy trip to the current published revision.
func (s *Store) PutProgress(ctx context.Context, in models.TourProgress, reset bool) (*models.TourProgress, error) {
	var result *models.TourProgress
	err := s.Transaction(ctx, func(tx *Store) error {
		id, err := tx.legacyProgressPin(ctx, in.AccountID, in.TourID, reset)
		if err != nil {
			return err
		}
		if in.RevisionID != "" && id != in.RevisionID {
			return fmt.Errorf("%w: selected progress revision changed; reload before retrying", ErrConflict)
		}
		r, err := tx.GetTourRevision(ctx, in.TourID, id)
		if err != nil {
			return err
		}
		ids := []string{}
		for _, position := range in.VisitedPositions {
			found := false
			for _, stop := range r.Snapshot.Tour.Stops {
				if stop.Position == position {
					ids = append(ids, stop.ID)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: position does not belong to the pinned revision", ErrInvalidReference)
			}
		}
		p, err := tx.PutRevisionProgress(ctx, models.RevisionProgress{AccountID: in.AccountID, TourID: in.TourID, RevisionID: id, Mode: in.Mode, VisitedStopIDs: ids, CompletedAt: in.CompletedAt}, reset)
		if err != nil {
			return err
		}
		result = legacyProjection(p, r)
		return nil
	})
	return result, err
}

// ListProgress retains the legacy one-active-trip-per-tour view for this account.
func (s *Store) ListProgress(ctx context.Context, account string) ([]models.TourProgress, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+revisionProgressCols+` FROM legacy_revision_pins pin JOIN tour_revision_progress p ON p.account_id=pin.account_id AND p.revision_id=pin.revision_id JOIN tour_revisions r ON r.id=p.revision_id WHERE pin.account_id=$1 ORDER BY p.updated_at DESC`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	progress := []*models.RevisionProgress{}
	for rows.Next() {
		p, err := scanRevisionProgress(rows)
		if err != nil {
			return nil, err
		}
		progress = append(progress, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	out := []models.TourProgress{}
	for _, p := range progress {
		r, err := s.GetTourRevision(ctx, p.TourID, p.RevisionID)
		if err != nil {
			return nil, err
		}
		out = append(out, *legacyProjection(p, r))
	}
	return out, nil
}
