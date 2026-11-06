package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"invisible-museum/backend/internal/models"
)

// ErrSubmissionLimit means an account reached the intake quota.
var ErrSubmissionLimit = errors.New("route brief submission limit reached")

const briefCols = `id, owner_account_id, submitted, status, reason, version, created_at, updated_at`

func scanBrief(row interface{ Scan(...any) error }) (*models.RouteBrief, error) {
	b := &models.RouteBrief{}
	var raw []byte
	if err := row.Scan(&b.ID, &b.OwnerAccountID, &raw, &b.Status, &b.Reason, &b.Version, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, mapErr(err)
	}
	if err := json.Unmarshal(raw, &b.Submitted); err != nil {
		return nil, err
	}
	return b, nil
}

// CreateRouteBrief serializes per-account creation, including idempotent retries
// and quotas. The original hash stays fixed even after later curator edits.
func (s *Store) CreateRouteBrief(ctx context.Context, owner string, in models.RouteBriefInput, key string) (*models.RouteBrief, bool, error) {
	in.Normalize()
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, false, err
	}
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	var out *models.RouteBrief
	created := false
	err = s.Transaction(ctx, func(tx *Store) error {
		var accountID string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM accounts WHERE id = $1 FOR UPDATE`, owner).Scan(&accountID); err != nil {
			return mapErr(err)
		}
		if key != "" {
			var id, previous string
			err := tx.pool.QueryRow(ctx, `SELECT id, submission_hash FROM route_briefs WHERE owner_account_id = $1 AND submission_key = $2`, owner, key).Scan(&id, &previous)
			if err == nil {
				if previous != digest {
					return fmt.Errorf("%w: idempotency key was used for a different brief", ErrConflict)
				}
				out, err = tx.GetRouteBrief(ctx, id, owner)
				return err
			}
			if !errors.Is(mapErr(err), ErrNotFound) {
				return err
			}
		}
		var active, recent int
		if err := tx.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status NOT IN ('accepted','declined')), count(*) FILTER (WHERE created_at > now() - interval '24 hours') FROM route_briefs WHERE owner_account_id = $1`, owner).Scan(&active, &recent); err != nil {
			return err
		}
		if active >= 20 || recent >= 10 {
			return ErrSubmissionLimit
		}
		var submissionKey any
		if key != "" {
			submissionKey = key
		}
		out, err = scanBrief(tx.pool.QueryRow(ctx, `INSERT INTO route_briefs (owner_account_id, submitted, submission_key, submission_hash) VALUES ($1,$2,$3,$4) RETURNING `+briefCols, owner, raw, submissionKey, digest))
		created = err == nil
		return err
	})
	return out, created, err
}

// GetRouteBrief restricts visibility in SQL when owner is nonempty. Curator
// handlers pass an empty owner only after server-side role authorization.
func (s *Store) GetRouteBrief(ctx context.Context, id, owner string) (*models.RouteBrief, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanBrief(s.pool.QueryRow(ctx, `SELECT `+briefCols+` FROM route_briefs WHERE id = $1 AND ($2 = '' OR owner_account_id::text = $2)`, id, owner))
}

// ListRouteBriefs returns private records, with an optional owner restriction.
func (s *Store) ListRouteBriefs(ctx context.Context, owner string, status models.BriefStatus, limit, offset int) ([]models.RouteBrief, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+briefCols+` FROM route_briefs WHERE ($1 = '' OR owner_account_id::text = $1) AND ($2 = '' OR status = $2) ORDER BY created_at DESC, id LIMIT $3 OFFSET $4`, owner, status, clamp(limit), offset)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.RouteBrief{}
	for rows.Next() {
		b, err := scanBrief(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// UpdateRouteBrief replaces submitted input only; a changed brief requires new
// review. The expected version protects edits and lifecycle decisions alike.
func (s *Store) UpdateRouteBrief(ctx context.Context, id, owner string, in models.UpdateRouteBriefRequest) (*models.RouteBrief, error) {
	var out *models.RouteBrief
	err := s.Transaction(ctx, func(tx *Store) error {
		b, err := tx.lockBrief(ctx, id, owner, in.Version)
		if err != nil {
			return err
		}
		if b.Status == models.BriefProcessing {
			return fmt.Errorf("%w: stop processing before editing this brief", ErrConflict)
		}
		in.Submitted.Normalize()
		raw, err := json.Marshal(in.Submitted)
		if err != nil {
			return err
		}
		out, err = scanBrief(tx.pool.QueryRow(ctx, `UPDATE route_briefs SET submitted = $2, status = 'draft', reason = 'Brief edited; review required.', version = version + 1, updated_at = now() WHERE id = $1 RETURNING `+briefCols, id, raw))
		return err
	})
	return out, err
}

func (s *Store) lockBrief(ctx context.Context, id, owner string, version int) (*models.RouteBrief, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	b, err := scanBrief(s.pool.QueryRow(ctx, `SELECT `+briefCols+` FROM route_briefs WHERE id = $1 AND ($2 = '' OR owner_account_id::text = $2) FOR UPDATE`, id, owner))
	if err != nil {
		return nil, err
	}
	if b.Version != version {
		return nil, fmt.Errorf("%w: brief changed; reload before saving", ErrConflict)
	}
	return b, nil
}

// SetRouteBriefState applies a curator decision without publishing any content.
func (s *Store) SetRouteBriefState(ctx context.Context, id string, in models.BriefStateRequest) (*models.RouteBrief, error) {
	var out *models.RouteBrief
	err := s.Transaction(ctx, func(tx *Store) error {
		b, err := tx.lockBrief(ctx, id, "", in.Version)
		if err != nil {
			return err
		}
		if !b.Status.CanTransitionTo(in.Status) {
			return fmt.Errorf("%w: cannot change brief from %s to %s", ErrConflict, b.Status, in.Status)
		}
		out, err = scanBrief(tx.pool.QueryRow(ctx, `UPDATE route_briefs SET status = $2, reason = $3, version = version + 1, updated_at = now() WHERE id = $1 RETURNING `+briefCols, id, in.Status, in.Reason))
		return err
	})
	return out, err
}
