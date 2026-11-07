package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"invisible-museum/backend/internal/models"
)

const arrivalCols = `id,place_id,base_version,point,status,created_by,reviewed_by,review_note,created_at,reviewed_at`

func scanArrival(row interface{ Scan(...any) error }) (*models.ArrivalProposal, error) {
	a := &models.ArrivalProposal{}
	var raw []byte
	if err := row.Scan(&a.ID, &a.PlaceID, &a.BaseVersion, &raw, &a.Status, &a.CreatedBy, &a.ReviewedBy, &a.ReviewNote, &a.CreatedAt, &a.ReviewedAt); err != nil {
		return nil, mapErr(err)
	}
	if err := json.Unmarshal(raw, &a.Point); err != nil {
		return nil, err
	}
	return a, nil
}

// GetArrivalProposal returns one private immutable proposal under its place.
func (s *Store) GetArrivalProposal(ctx context.Context, placeID, id string) (*models.ArrivalProposal, error) {
	if !isUUID(placeID) || !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanArrival(s.pool.QueryRow(ctx, `SELECT `+arrivalCols+` FROM place_arrival_proposals WHERE id=$1 AND place_id=$2`, id, placeID))
}

// ListArrivalProposals pages private input and review history, newest first.
func (s *Store) ListArrivalProposals(ctx context.Context, placeID string, limit, offset int) ([]models.ArrivalProposal, error) {
	resolved, err := s.ResolvePlaceID(ctx, placeID, "")
	if err != nil {
		return nil, err
	}
	placeID = resolved
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `SELECT `+arrivalCols+` FROM place_arrival_proposals WHERE place_id=$1 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, placeID, clamp(limit), offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ArrivalProposal{}
	for rows.Next() {
		a, err := scanArrival(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// ProposeArrival keeps the current approved point while a new location/access
// assessment awaits review. Historical place coordinates are never updated.
func (s *Store) ProposeArrival(ctx context.Context, placeID, author string, in models.ArrivalProposalRequest) (*models.ArrivalProposal, error) {
	if !isUUID(placeID) {
		return nil, ErrNotFound
	}
	if len(in.Validate()) > 0 {
		return nil, ErrInvalidReference
	}
	var id string
	err := s.Transaction(ctx, func(tx *Store) error {
		var place string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM places WHERE id=$1 FOR UPDATE`, placeID).Scan(&place); err != nil {
			return mapErr(err)
		}
		var version int
		if err := tx.pool.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM place_arrival_current WHERE place_id=$1),0)`, placeID).Scan(&version); err != nil {
			return err
		}
		if version != in.Version {
			return fmt.Errorf("%w: approved arrival changed; reload before proposing", ErrConflict)
		}
		raw, err := json.Marshal(in.Point)
		if err != nil {
			return err
		}
		return mapErr(tx.pool.QueryRow(ctx, `INSERT INTO place_arrival_proposals(place_id,base_version,point,created_by) VALUES($1,$2,$3,$4) RETURNING id`, placeID, version, raw, author).Scan(&id))
	})
	if err != nil {
		return nil, err
	}
	return s.GetArrivalProposal(ctx, placeID, id)
}

// ReviewArrival serializes the current pointer through the place row. An old
// proposal cannot overwrite a newer approved entrance or its mode restrictions.
func (s *Store) ReviewArrival(ctx context.Context, placeID, id, reviewer string, in models.ArrivalReviewRequest) (*models.ArrivalProposal, error) {
	if !isUUID(placeID) || !isUUID(id) {
		return nil, ErrNotFound
	}
	if len(in.Validate()) > 0 {
		return nil, ErrInvalidReference
	}
	err := s.Transaction(ctx, func(tx *Store) error {
		var place string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM places WHERE id=$1 FOR UPDATE`, placeID).Scan(&place); err != nil {
			return mapErr(err)
		}
		a, err := scanArrival(tx.pool.QueryRow(ctx, `SELECT `+arrivalCols+` FROM place_arrival_proposals WHERE id=$1 AND place_id=$2 FOR UPDATE`, id, placeID))
		if err != nil {
			return err
		}
		if a.Status != "pending" || in.Version != a.BaseVersion {
			return fmt.Errorf("%w: arrival proposal already reviewed or version changed", ErrConflict)
		}
		status := "rejected"
		if in.Action == "approve" {
			var version int
			if err := tx.pool.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM place_arrival_current WHERE place_id=$1),0)`, placeID).Scan(&version); err != nil {
				return err
			}
			if version != a.BaseVersion {
				return fmt.Errorf("%w: approved arrival changed; propose against its latest version", ErrConflict)
			}
			if len((models.ArrivalProposalRequest{Version: version, Point: a.Point}).Validate()) > 0 {
				return ErrInvalidReference
			}
			status = "approved"
			if _, err := tx.pool.Exec(ctx, `INSERT INTO place_arrival_current(place_id,proposal_id,version) VALUES($1,$2,$3) ON CONFLICT(place_id) DO UPDATE SET proposal_id=excluded.proposal_id,version=excluded.version`, placeID, id, version+1); err != nil {
				return mapErr(err)
			}
		}
		_, err = tx.pool.Exec(ctx, `UPDATE place_arrival_proposals SET status=$2,reviewed_by=$3,review_note=$4,reviewed_at=now() WHERE id=$1`, id, status, reviewer, in.Note)
		return mapErr(err)
	})
	if err != nil {
		return nil, err
	}
	return s.GetArrivalProposal(ctx, placeID, id)
}
