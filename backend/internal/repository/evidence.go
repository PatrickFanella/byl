package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"invisible-museum/backend/internal/models"
)

const evidenceCols = `id,brief_id,brief_version,COALESCE(job_id::text,''),source_key,url,reference,source_kind,result,created_at`

func scanEvidence(row interface{ Scan(...any) error }) (*models.SourceEvidence, error) {
	e := &models.SourceEvidence{}
	var raw []byte
	if err := row.Scan(&e.ID, &e.BriefID, &e.BriefVersion, &e.JobID, &e.SourceKey, &e.URL, &e.Reference, &e.SourceKind, &raw, &e.CreatedAt); err != nil {
		return nil, mapErr(err)
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &e.Result); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// QueueSourceEvidence snapshots the requested brief version and atomically queues
// one bounded retrieval. Idempotent retries retain the original record and job.
func (s *Store) QueueSourceEvidence(ctx context.Context, briefID string, in models.EvidenceRequest, reference, sourceKey, key string) (*models.SourceEvidence, error) {
	var id string
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(append(raw, []byte(reference)...))
	hash := hex.EncodeToString(digest[:])
	err = s.Transaction(ctx, func(tx *Store) error {
		if !isUUID(briefID) {
			return ErrNotFound
		}
		b, err := scanBrief(tx.pool.QueryRow(ctx, `SELECT `+briefCols+` FROM route_briefs WHERE id=$1 FOR UPDATE`, briefID))
		if err != nil {
			return err
		}
		if key != "" {
			var previous string
			err := tx.pool.QueryRow(ctx, `SELECT id,request_hash FROM source_evidence WHERE brief_id=$1 AND request_key=$2`, briefID, key).Scan(&id, &previous)
			if err == nil {
				if previous != hash {
					return fmt.Errorf("%w: idempotency key was used for another retrieval", ErrConflict)
				}
				return nil
			}
			if !errors.Is(mapErr(err), ErrNotFound) {
				return err
			}
		}
		if b.Version != in.Version {
			return fmt.Errorf("%w: brief changed; reload before requesting evidence", ErrConflict)
		}
		var count int
		if err := tx.pool.QueryRow(ctx, `SELECT count(*) FROM source_evidence WHERE brief_id=$1 AND created_at>now()-interval '24 hours'`, briefID).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return ErrSubmissionLimit
		}
		var requestKey any
		if key != "" {
			requestKey = key
		}
		if err := tx.pool.QueryRow(ctx, `INSERT INTO source_evidence (brief_id,brief_version,source_key,url,reference,source_kind,request_key,request_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, briefID, in.Version, sourceKey, in.URL, reference, in.SourceKind, requestKey, hash).Scan(&id); err != nil {
			return mapErr(err)
		}
		job, err := tx.EnqueueReturning(ctx, models.JobKindFetchEvidence, models.FetchEvidencePayload{EvidenceID: id})
		if err != nil {
			return err
		}
		_, err = tx.pool.Exec(ctx, `UPDATE source_evidence SET job_id=$2 WHERE id=$1`, id, job.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetSourceEvidence(ctx, id)
}

// GetSourceEvidence is internal; HTTP handlers must enforce curator authorization.
func (s *Store) GetSourceEvidence(ctx context.Context, id string) (*models.SourceEvidence, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanEvidence(s.pool.QueryRow(ctx, `SELECT `+evidenceCols+` FROM source_evidence WHERE id=$1`, id))
}

// ListSourceEvidence returns private records at their original submitted versions.
func (s *Store) ListSourceEvidence(ctx context.Context, briefID string, limit, offset int) ([]models.SourceEvidence, error) {
	if _, err := s.GetRouteBrief(ctx, briefID, ""); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+evidenceCols+` FROM source_evidence WHERE brief_id=$1 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, briefID, min(clamp(limit), 50), offset)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.SourceEvidence{}
	for rows.Next() {
		e, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// CompleteSourceEvidence retains the first committed snapshot. Claim fencing
// prevents a reclaimed worker from replacing newer evidence or its job outcome.
func (s *Store) CompleteSourceEvidence(ctx context.Context, id string, claim *models.Job, result models.EvidenceResult) error {
	return s.Transaction(ctx, func(tx *Store) error {
		e, err := scanEvidence(tx.pool.QueryRow(ctx, `SELECT `+evidenceCols+` FROM source_evidence WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if e.Result != nil {
			return nil
		}
		var attempts int
		var status models.JobStatus
		if err := tx.pool.QueryRow(ctx, `SELECT attempts,status FROM jobs WHERE id=$1 FOR UPDATE`, e.JobID).Scan(&attempts, &status); err != nil {
			return mapErr(err)
		}
		if claim.ID != e.JobID || claim.Attempts != attempts || status != models.JobStatusRunning {
			return ErrResearchInactive
		}
		result.Untrusted = true
		result.MediaReuse = "unresolved"
		if result.RetrievedAt.IsZero() {
			result.RetrievedAt = time.Now().UTC()
		}
		if result.Passages == nil {
			result.Passages = []models.EvidencePassage{}
		}
		if !result.Valid() {
			return fmt.Errorf("%w: invalid bounded evidence result", ErrInvalidReference)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if len(raw) > 262144 {
			return fmt.Errorf("%w: evidence exceeds storage bound", ErrInvalidReference)
		}
		_, err = tx.pool.Exec(ctx, `UPDATE source_evidence SET result=$2 WHERE id=$1`, id, raw)
		return err
	})
}
