package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"invisible-museum/backend/internal/models"
)

const corridorCols = `id,tour_id,base_version,draft_version,intent,remove,content_sha256,status,created_by,reviewed_by,review_note,created_at,reviewed_at`

func scanCorridor(row interface{ Scan(...any) error }) (*models.CorridorProposal, error) {
	p := &models.CorridorProposal{}
	var raw []byte
	if err := row.Scan(&p.ID, &p.TourID, &p.BaseVersion, &p.DraftVersion, &raw, &p.Remove, &p.ContentSHA256, &p.Status, &p.CreatedBy, &p.ReviewedBy, &p.ReviewNote, &p.CreatedAt, &p.ReviewedAt); err != nil {
		return nil, mapErr(err)
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p.Intent); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// GetApprovedCorridor exposes no private review fields. Nil means no intent was
// ever approved; an approved removal retains its version with a null intent.
func (s *Store) GetApprovedCorridor(ctx context.Context, tour string) (*models.ApprovedCorridor, error) {
	var p models.ApprovedCorridor
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT p.id,c.version,p.intent,p.content_sha256,p.reviewed_at FROM tour_corridor_current c JOIN tour_corridor_proposals p ON p.id=c.proposal_id AND p.tour_id=c.tour_id WHERE c.tour_id=$1 AND p.status='approved'`, tour).Scan(&p.ID, &p.Version, &raw, &p.ContentSHA256, &p.ReviewedAt)
	if errors.Is(mapErr(err), ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p.Intent); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

// ProposeCorridor imports bounded typed reference data into private review.
func (s *Store) ProposeCorridor(ctx context.Context, tour, author string, in models.CorridorProposalRequest) (*models.CorridorProposal, error) {
	if in.Version < 0 || (in.Remove == (in.Intent != nil)) {
		return nil, ErrInvalidReference
	}
	if in.Intent != nil && len(in.Intent.Validate()) > 0 {
		return nil, ErrInvalidReference
	}
	raw, err := json.Marshal(in.Intent)
	if err != nil || len(raw) > 262144 {
		return nil, ErrInvalidReference
	}
	hash := sha256.Sum256(raw)
	var id string
	err = s.Transaction(ctx, func(tx *Store) error {
		if !isUUID(tour) {
			return ErrNotFound
		}
		var locked string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, tour).Scan(&locked); err != nil {
			return mapErr(err)
		}
		working, err := tx.GetTour(ctx, tour, false)
		if err != nil {
			return err
		}
		current := int64(0)
		if working.Corridor != nil {
			current = working.Corridor.Version
		}
		if current != in.Version {
			return fmt.Errorf("%w: approved corridor changed; reload before proposing", ErrConflict)
		}
		if in.Intent != nil {
			if err := in.Intent.ValidateStops(working); err != nil {
				return fmt.Errorf("%w: %s", ErrInvalidReference, err)
			}
		}
		var intent any
		if !in.Remove {
			intent = raw
		}
		return tx.pool.QueryRow(ctx, `INSERT INTO tour_corridor_proposals(tour_id,base_version,draft_version,intent,remove,content_sha256,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, tour, in.Version, working.DraftVersion, intent, in.Remove, hex.EncodeToString(hash[:]), author).Scan(&id)
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetCorridorProposal(ctx, tour, id)
}

// GetCorridorProposal is private; the transport must require curator access.
func (s *Store) GetCorridorProposal(ctx context.Context, tour, id string) (*models.CorridorProposal, error) {
	if !isUUID(tour) || !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanCorridor(s.pool.QueryRow(ctx, `SELECT `+corridorCols+` FROM tour_corridor_proposals WHERE tour_id=$1 AND id=$2`, tour, id))
}

// ListCorridorProposals keeps completed/private proposals separate from public intent.
func (s *Store) ListCorridorProposals(ctx context.Context, tour string, limit, offset int) ([]models.CorridorProposal, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+corridorCols+` FROM tour_corridor_proposals WHERE tour_id=$1 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, tour, clamp(limit), max(0, offset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CorridorProposal{}
	for rows.Next() {
		p, err := scanCorridor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ReviewCorridor changes only the working intent. A separate authorized tour
// publication releases it; personal legacy auto-release is deliberately cleared.
func (s *Store) ReviewCorridor(ctx context.Context, tour, id, author string, in models.ArrivalReviewRequest) (*models.CorridorProposal, error) {
	if !isUUID(tour) || !isUUID(id) || len(in.Validate()) > 0 {
		return nil, ErrInvalidReference
	}
	err := s.Transaction(ctx, func(tx *Store) error {
		var locked string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, tour).Scan(&locked); err != nil {
			return mapErr(err)
		}
		working, err := tx.GetTour(ctx, tour, false)
		if err != nil {
			return err
		}
		p, err := scanCorridor(tx.pool.QueryRow(ctx, `SELECT `+corridorCols+` FROM tour_corridor_proposals WHERE tour_id=$1 AND id=$2 FOR UPDATE`, tour, id))
		if err != nil {
			return err
		}
		if p.Status != "pending" || p.BaseVersion != int64(in.Version) {
			return fmt.Errorf("%w: proposal was already reviewed or its version changed", ErrConflict)
		}
		status := "rejected"
		if in.Action == "approve" {
			current := int64(0)
			if working.Corridor != nil {
				current = working.Corridor.Version
			}
			if current != p.BaseVersion || working.DraftVersion != p.DraftVersion {
				return fmt.Errorf("%w: corridor or tour draft changed; create a fresh proposal", ErrConflict)
			}
			if p.Intent != nil {
				if err := p.Intent.ValidateStops(working); err != nil {
					return fmt.Errorf("%w: %s", ErrConflict, err)
				}
			}
			status = "approved"
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE tour_corridor_proposals SET status=$3,reviewed_by=$4,review_note=$5,reviewed_at=now() WHERE tour_id=$1 AND id=$2`, tour, id, status, author, in.Note); err != nil {
			return err
		}
		if status == "approved" {
			if _, err := tx.pool.Exec(ctx, `INSERT INTO tour_corridor_current(tour_id,proposal_id,version) VALUES($1,$2,$3) ON CONFLICT(tour_id) DO UPDATE SET proposal_id=excluded.proposal_id,version=excluded.version`, tour, id, p.BaseVersion+1); err != nil {
				return err
			}
			// Keep the same lock/version rules as ordinary draft edits.
			return tx.withTx(ctx, func(inner pgx.Tx) error {
				if err := bumpTourDraftTx(ctx, inner, tour, true, &working.DraftVersion); err != nil {
					return err
				}
				if _, err := inner.Exec(ctx, `UPDATE tour_revision_state SET release_requested_version=NULL WHERE tour_id=$1`, tour); err != nil {
					return err
				}
				if _, err := inner.Exec(ctx, `UPDATE tours SET updated_at=now() WHERE id=$1`, tour); err != nil {
					return err
				}
				return syncRoutesTx(ctx, inner, tour, modesArg(working.Modes), true)
			})
		}
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetCorridorProposal(ctx, tour, id)
}
