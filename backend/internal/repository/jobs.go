package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"invisible-museum/backend/internal/models"
)

const jobCols = `id, kind, payload, status, attempts, max_attempts, run_after, last_error, started_at, finished_at, created_at`

func scanJob(row interface{ Scan(...any) error }) (models.Job, error) {
	var j models.Job
	err := row.Scan(&j.ID, &j.Kind, &j.Payload, &j.Status, &j.Attempts, &j.MaxAttempts, &j.RunAfter, &j.LastError, &j.StartedAt, &j.FinishedAt, &j.CreatedAt)
	return j, err
}

func enqueueTx(ctx context.Context, q dbtx, kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO jobs (kind, payload) VALUES ($1, $2)`, kind, raw)
	return err
}

// Enqueue adds a job to the queue.
func (s *Store) Enqueue(ctx context.Context, kind string, payload any) error {
	return mapErr(enqueueTx(ctx, s.pool, kind, payload))
}

// ClaimJob atomically takes the next runnable job, or returns nil when there is none.
func (s *Store) ClaimJob(ctx context.Context, now time.Time) (*models.Job, error) {
	j, err := scanJob(s.pool.QueryRow(ctx, `UPDATE jobs SET status = 'running', attempts = attempts + 1, started_at = $1, updated_at = $1
		WHERE id = (SELECT id FROM jobs WHERE status = 'queued' AND run_after <= $1 ORDER BY run_after, created_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING `+jobCols, now))
	if err != nil {
		if mapErr(err) == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &j, nil
}

// CompleteJob marks a job done.
func (s *Store) CompleteJob(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status = 'done', finished_at = now(), updated_at = now(), last_error = '' WHERE id = $1`, id)
	return mapErr(err)
}

// FailJob records an error; with retryAfter it requeues, otherwise the job is failed for good.
func (s *Store) FailJob(ctx context.Context, id, errMsg string, retryAfter *time.Time) error {
	var err error
	if retryAfter != nil {
		_, err = s.pool.Exec(ctx, `UPDATE jobs SET status = 'queued', run_after = $2, last_error = $3, updated_at = now() WHERE id = $1`, id, *retryAfter, errMsg)
	} else {
		_, err = s.pool.Exec(ctx, `UPDATE jobs SET status = 'failed', finished_at = now(), last_error = $2, updated_at = now() WHERE id = $1`, id, errMsg)
	}
	return mapErr(err)
}

// GetJob returns one job.
func (s *Store) GetJob(ctx context.Context, id string) (*models.Job, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	j, err := scanJob(s.pool.QueryRow(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = $1`, id))
	if err != nil {
		return nil, mapErr(err)
	}
	return &j, nil
}

// ListJobs returns jobs, optionally filtered by status, newest first.
func (s *Store) ListJobs(ctx context.Context, status models.JobStatus, limit, offset int) ([]models.Job, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+jobCols+` FROM jobs WHERE ($1 = '' OR status::text = $1) ORDER BY created_at DESC LIMIT $2 OFFSET $3`, string(status), clamp(limit), offset)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// RetryJob requeues a failed job from scratch.
func (s *Store) RetryJob(ctx context.Context, id string) error {
	job, err := s.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if job.Kind == models.JobKindResearchBrief {
		return fmt.Errorf("%w: use the brief research resume action to retain budgets", ErrConflict)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE jobs SET status = 'queued', attempts = 0, run_after = now(), finished_at = NULL, updated_at = now() WHERE id = $1 AND status = 'failed'`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FinishResearchJob fences completion/retry from cancelled or reclaimed claims.
func (s *Store) FinishResearchJob(ctx context.Context, job *models.Job, note string, retryAfter *time.Time, failed bool) error {
	status := "done"
	if failed {
		status = "failed"
	}
	if retryAfter != nil {
		status = "queued"
	}
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status=$3::job_status,last_error=$4,run_after=COALESCE($5,run_after),finished_at=CASE WHEN $3='queued' THEN NULL ELSE now() END,updated_at=now() WHERE id=$1 AND attempts=$2 AND status='running'`, job.ID, job.Attempts, status, note, retryAfter)
	return mapErr(err)
}

// ReclaimStaleJobs requeues jobs that have been running longer than olderThan (a worker died mid-job).
func (s *Store) ReclaimStaleJobs(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE jobs SET status = 'queued', run_after = now(), last_error = 'reclaimed: worker did not finish', updated_at = now()
		WHERE status = 'running' AND started_at < now() - $1::interval`, olderThan)
	if err != nil {
		return 0, mapErr(err)
	}
	return tag.RowsAffected(), nil
}

// SetJobNote stores an informational note on a job (the last_error column doubles as the result line).
func (s *Store) SetJobNote(ctx context.Context, id, note string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `UPDATE jobs SET last_error = $2, updated_at = now() WHERE id = $1`, id, note)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EnqueueReturning adds a job and returns the row.
func (s *Store) EnqueueReturning(ctx context.Context, kind string, payload any) (*models.Job, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	j, err := scanJob(s.pool.QueryRow(ctx, `INSERT INTO jobs (kind, payload) VALUES ($1, $2) RETURNING `+jobCols, kind, raw))
	if err != nil {
		return nil, mapErr(err)
	}
	return &j, nil
}

// Stats returns the dashboard counts.
func (s *Store) Stats(ctx context.Context) (*models.Stats, error) {
	st := &models.Stats{PendingCandidates: map[string]int{}, Jobs: map[string]int{}}
	rows, err := s.pool.Query(ctx, `SELECT kind::text, count(*) FROM candidates WHERE status = 'pending' GROUP BY kind`)
	if err != nil {
		return nil, mapErr(err)
	}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			rows.Close()
			return nil, err
		}
		st.PendingCandidates[k] = n
	}
	rows.Close()
	jrows, err := s.pool.Query(ctx, `SELECT status::text, count(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, mapErr(err)
	}
	for jrows.Next() {
		var k string
		var n int
		if err := jrows.Scan(&k, &n); err != nil {
			jrows.Close()
			return nil, err
		}
		st.Jobs[k] = n
	}
	jrows.Close()
	err = s.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM places WHERE status = 'published'),
		(SELECT count(*) FROM moments WHERE status = 'published'),
		(SELECT count(*) FROM tours WHERE status = 'published' AND visibility = 'public' AND owner_account_id IS NULL),
		(SELECT count(*) FROM tours WHERE owner_account_id IS NOT NULL),
		(SELECT count(*) FROM accounts)`).Scan(&st.Published.Places, &st.Published.Moments, &st.Published.Tours, &st.UserRoutes, &st.Accounts)
	if err != nil {
		return nil, mapErr(err)
	}
	return st, nil
}
