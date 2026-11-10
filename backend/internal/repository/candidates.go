package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"invisible-museum/backend/internal/models"
)

// CandidateFilter narrows ListCandidates.
type CandidateFilter struct {
	Status models.CandidateStatus
	Kind   models.CandidateKind
	Origin models.CandidateOrigin
	Source string
	Limit  int
	Cursor string
}

const candidateCols = `id, kind, origin, source_name, external_id, submitted_by_account_id, target_place_id, target_moment_id, payload, status, reviewer_account_id, review_note, approved_entity_id, reviewed_at, created_at, updated_at`

func scanCandidate(row interface{ Scan(...any) error }) (models.Candidate, error) {
	var c models.Candidate
	err := row.Scan(&c.ID, &c.Kind, &c.Origin, &c.SourceName, &c.ExternalID, &c.SubmittedByAccountID, &c.TargetPlaceID, &c.TargetMomentID,
		&c.Payload, &c.Status, &c.ReviewerAccountID, &c.ReviewNote, &c.ApprovedEntityID, &c.ReviewedAt, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// ListCandidates returns review-queue entries, oldest first, with a cursor for the next page.
func (s *Store) ListCandidates(ctx context.Context, f CandidateFilter) ([]models.Candidate, string, error) {
	where := []string{"TRUE"}
	args := []any{}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Kind != "" {
		args = append(args, f.Kind)
		where = append(where, fmt.Sprintf("kind = $%d", len(args)))
	}
	if f.Origin != "" {
		args = append(args, f.Origin)
		where = append(where, fmt.Sprintf("origin = $%d", len(args)))
	}
	if f.Source != "" {
		args = append(args, f.Source)
		where = append(where, fmt.Sprintf("source_name = $%d", len(args)))
	}
	limit := clamp(f.Limit)
	offset, _ := strconv.Atoi(f.Cursor)
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit+1, offset)
	rows, err := s.pool.Query(ctx, `SELECT `+candidateCols+` FROM candidates WHERE `+strings.Join(where, " AND ")+
		fmt.Sprintf(` ORDER BY created_at, id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, "", mapErr(err)
	}
	defer rows.Close()
	out := []models.Candidate{}
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = strconv.Itoa(offset + limit)
	}
	return out, next, nil
}

// GetCandidate returns one entry.
func (s *Store) GetCandidate(ctx context.Context, id string) (*models.Candidate, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	c, err := scanCandidate(s.pool.QueryRow(ctx, `SELECT `+candidateCols+` FROM candidates WHERE id = $1`, id))
	if err != nil {
		return nil, mapErr(err)
	}
	return &c, nil
}

// ReviewPendingCandidate locks an entry while its entity and review are written in one transaction.
func (s *Store) ReviewPendingCandidate(ctx context.Context, id string, fn func(*Store, *models.Candidate) error) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	return s.Transaction(ctx, func(tx *Store) error {
		c, err := scanCandidate(tx.pool.QueryRow(ctx, `SELECT `+candidateCols+` FROM candidates WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return mapErr(err)
		}
		if c.Status != models.CandidateStatusPending {
			return fmt.Errorf("%w: candidate already reviewed", ErrConflict)
		}
		return fn(tx, &c)
	})
}

// CreateCandidate adds a pending entry.
func (s *Store) CreateCandidate(ctx context.Context, c models.Candidate) (*models.Candidate, error) {
	if len(c.Payload) == 0 {
		c.Payload = []byte(`{}`)
	}
	if c.Origin == "" {
		c.Origin = models.CandidateOriginDraft
	}
	var id string
	err := s.pool.QueryRow(ctx, `INSERT INTO candidates (kind, origin, source_name, external_id, submitted_by_account_id, target_place_id, target_moment_id, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		c.Kind, c.Origin, nullable(c.SourceName), nullable(c.ExternalID), nullable(c.SubmittedByAccountID), nullable(c.TargetPlaceID), nullable(c.TargetMomentID), c.Payload).Scan(&id)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetCandidate(ctx, id)
}

// ReviewCandidate records an approval or rejection. Only pending entries can be reviewed.
func (s *Store) ReviewCandidate(ctx context.Context, id, reviewerID string, status models.CandidateStatus, note string, approvedEntityID *string) (*models.Candidate, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE candidates SET status = $2, reviewer_account_id = $3, review_note = $4, approved_entity_id = $5, reviewed_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'pending'`, id, status, reviewerID, note, nullable(approvedEntityID))
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.GetCandidate(ctx, id); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: candidate already reviewed", ErrConflict)
	}
	return s.GetCandidate(ctx, id)
}

// CandidateExists reports whether a candidate with this source and external id was ever created.
func (s *Store) CandidateExists(ctx context.Context, source, externalID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM candidates WHERE source_name = $1 AND external_id = $2)`, source, externalID).Scan(&ok)
	return ok, mapErr(err)
}

// UpdateCandidatePayload replaces a candidate's payload and optionally its origin.
func (s *Store) UpdateCandidatePayload(ctx context.Context, id string, payload json.RawMessage, origin *models.CandidateOrigin) error {
	tag, err := s.pool.Exec(ctx, `UPDATE candidates SET payload = $2, origin = COALESCE($3, origin), updated_at = now() WHERE id = $1`, id, payload, origin)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
