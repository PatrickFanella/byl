package repository

import (
	"context"

	"invisible-museum/backend/internal/models"
)

// Visitor accounts share one job queue with curator work, so their tours are bounded: at most
// UserToursPerDay new tours in 24 hours and UserToursOwned in total, and no new routing work
// while UserRouteBacklog of their route computations are still waiting or running.
const (
	UserToursPerDay  = 20
	UserToursOwned   = 100
	UserRouteBacklog = 20
)

// CheckUserTourCreate returns ErrSubmissionLimit when owner may not create another tour now.
func (s *Store) CheckUserTourCreate(ctx context.Context, owner string) error {
	var owned, recent int
	if err := s.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE created_at > now() - interval '24 hours') FROM tours
		WHERE owner_account_id = $1`, owner).Scan(&owned, &recent); err != nil {
		return mapErr(err)
	}
	if owned >= UserToursOwned || recent >= UserToursPerDay {
		return ErrSubmissionLimit
	}
	return s.CheckUserRouteBacklog(ctx, owner)
}

// CheckUserRouteBacklog returns ErrSubmissionLimit while owner's tours already have
// UserRouteBacklog route computations queued or running.
func (s *Store) CheckUserRouteBacklog(ctx context.Context, owner string) error {
	var pending int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM jobs j JOIN tours t ON t.id::text = j.payload->>'tour_id'
		WHERE j.kind = $2 AND j.status IN ('queued', 'running') AND t.owner_account_id = $1`, owner, models.JobKindComputeRoute).Scan(&pending); err != nil {
		return mapErr(err)
	}
	if pending >= UserRouteBacklog {
		return ErrSubmissionLimit
	}
	return nil
}
