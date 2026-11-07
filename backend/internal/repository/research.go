package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"invisible-museum/backend/internal/models"
)

// ErrResearchInactive fences cancelled runs and superseded worker claims.
var ErrResearchInactive = errors.New("research run or worker claim is no longer active")

// ResearchTokenReservation is the maximum model output/input budget per stage call.
// Reservations remain charged after crashes, since usage may be unknown.
const ResearchTokenReservation = 2000

const researchCols = `id, brief_id, COALESCE(job_id::text,''), submitted, source_version, brief_version, status, reason, requests_used, tokens_reserved, request_limit, token_limit, deadline_at, created_at`

func scanResearch(row interface{ Scan(...any) error }) (*models.ResearchRun, error) {
	r := &models.ResearchRun{}
	var input []byte
	if err := row.Scan(&r.ID, &r.BriefID, &r.JobID, &input, &r.SourceVersion, &r.BriefVersion, &r.Status, &r.Reason, &r.RequestsUsed, &r.TokensReserved, &r.RequestLimit, &r.TokenLimit, &r.DeadlineAt, &r.CreatedAt); err != nil {
		return nil, mapErr(err)
	}
	if err := json.Unmarshal(input, &r.Submitted); err != nil {
		return nil, err
	}
	return r, nil
}

// GetResearchRun loads the internal snapshot and checkpoints. Output and submitted
// source content have no JSON representation on the progress contract.
func (s *Store) GetResearchRun(ctx context.Context, id string) (*models.ResearchRun, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	r, err := scanResearch(s.pool.QueryRow(ctx, `SELECT `+researchCols+` FROM brief_research_runs WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT stage,status,attempts,reason,output,candidate_ids FROM brief_research_stages WHERE run_id = $1 ORDER BY CASE stage WHEN 'research' THEN 1 WHEN 'reconciliation' THEN 2 WHEN 'drafting' THEN 3 ELSE 4 END`, id)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	r.Stages = []models.ResearchStage{}
	for rows.Next() {
		var stage models.ResearchStage
		stage.Outcomes = []models.ResearchAttempt{}
		if err := rows.Scan(&stage.Stage, &stage.Status, &stage.Attempts, &stage.Reason, &stage.Output, &stage.CandidateIDs); err != nil {
			return nil, err
		}
		r.Stages = append(r.Stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	attempts, err := s.pool.Query(ctx, `SELECT stage,attempt,status,reason FROM brief_research_attempts WHERE run_id=$1 ORDER BY stage,attempt`, id)
	if err != nil {
		return nil, err
	}
	defer attempts.Close()
	for attempts.Next() {
		var stage string
		var outcome models.ResearchAttempt
		if err := attempts.Scan(&stage, &outcome.Attempt, &outcome.Status, &outcome.Reason); err != nil {
			return nil, err
		}
		for i := range r.Stages {
			if r.Stages[i].Stage == stage {
				r.Stages[i].Outcomes = append(r.Stages[i].Outcomes, outcome)
			}
		}
	}
	return r, attempts.Err()
}

// LatestBriefResearch checks private ownership before returning progress.
func (s *Store) LatestBriefResearch(ctx context.Context, briefID, owner string) (*models.ResearchRun, error) {
	if _, err := s.GetRouteBrief(ctx, briefID, owner); err != nil {
		return nil, err
	}
	var id string
	if err := s.pool.QueryRow(ctx, `SELECT id FROM brief_research_runs WHERE brief_id = $1 ORDER BY created_at DESC,id LIMIT 1`, briefID).Scan(&id); err != nil {
		return nil, mapErr(err)
	}
	return s.GetResearchRun(ctx, id)
}

// StartBriefResearch atomically snapshots the brief, queues its job and marks it
// processing. Retrying the same source version returns the original run.
func (s *Store) StartBriefResearch(ctx context.Context, id string, version int) (*models.ResearchRun, error) {
	var runID string
	err := s.Transaction(ctx, func(tx *Store) error {
		if !isUUID(id) {
			return ErrNotFound
		}
		b, err := scanBrief(tx.pool.QueryRow(ctx, `SELECT `+briefCols+` FROM route_briefs WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		err = tx.pool.QueryRow(ctx, `SELECT id FROM brief_research_runs WHERE brief_id = $1 AND source_version = $2`, id, version).Scan(&runID)
		if err == nil {
			return nil
		}
		if !errors.Is(mapErr(err), ErrNotFound) {
			return err
		}
		if version != b.Version || (b.Status != models.BriefDraft && b.Status != models.BriefNeedsResearch) {
			return fmt.Errorf("%w: reload a draft or needs-research brief before starting research", ErrConflict)
		}
		raw, err := json.Marshal(b.Submitted)
		if err != nil {
			return err
		}
		if err := tx.pool.QueryRow(ctx, `INSERT INTO brief_research_runs (brief_id,submitted,source_version,brief_version) VALUES ($1,$2,$3,$3+1) RETURNING id`, id, raw, version).Scan(&runID); err != nil {
			return mapErr(err)
		}
		job, err := tx.EnqueueReturning(ctx, models.JobKindResearchBrief, models.ResearchBriefPayload{RunID: runID})
		if err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE jobs SET max_attempts=12 WHERE id=$1`, job.ID); err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_runs SET job_id = $2 WHERE id = $1`, runID, job.ID); err != nil {
			return err
		}
		for _, stage := range models.ResearchStages {
			if _, err := tx.pool.Exec(ctx, `INSERT INTO brief_research_stages (run_id,stage) VALUES ($1,$2)`, runID, stage); err != nil {
				return err
			}
		}
		_, err = tx.pool.Exec(ctx, `UPDATE route_briefs SET status = 'processing', reason = 'Research queued.', version = version + 1, updated_at = now() WHERE id = $1`, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetResearchRun(ctx, runID)
}

// lockResearch uses one lock order for workers, cancellation and resume:
// brief, run, job. The attempt number is a fence against a reclaimed worker.
func (s *Store) lockResearch(ctx context.Context, runID string, claim *models.Job) (*models.ResearchRun, *models.RouteBrief, error) {
	if !isUUID(runID) {
		return nil, nil, ErrNotFound
	}
	var briefID string
	if err := s.pool.QueryRow(ctx, `SELECT brief_id FROM brief_research_runs WHERE id = $1`, runID).Scan(&briefID); err != nil {
		return nil, nil, mapErr(err)
	}
	b, err := scanBrief(s.pool.QueryRow(ctx, `SELECT `+briefCols+` FROM route_briefs WHERE id = $1 FOR UPDATE`, briefID))
	if err != nil {
		return nil, nil, err
	}
	r, err := scanResearch(s.pool.QueryRow(ctx, `SELECT `+researchCols+` FROM brief_research_runs WHERE id = $1 FOR UPDATE`, runID))
	if err != nil {
		return nil, nil, err
	}
	if claim != nil {
		var attempts int
		var status models.JobStatus
		if err := s.pool.QueryRow(ctx, `SELECT attempts,status FROM jobs WHERE id = $1 FOR UPDATE`, r.JobID).Scan(&attempts, &status); err != nil {
			return nil, nil, mapErr(err)
		}
		if claim.ID != r.JobID || attempts != claim.Attempts || status != models.JobStatusRunning {
			return nil, nil, ErrResearchInactive
		}
	}
	return r, b, nil
}

// stopResearchTx records a safe reason code; it never clobbers a newer curator decision.
func (s *Store) stopResearchTx(ctx context.Context, r *models.ResearchRun, b *models.RouteBrief, status, code string) error {
	if r.Status != "active" {
		return ErrResearchInactive
	}
	if models.ResearchReasonMessage(code) == "" {
		code = "provider_failure"
	}
	if _, err := s.pool.Exec(ctx, `UPDATE brief_research_stages SET status='needs_research',reason=$2,updated_at=now() WHERE run_id=$1 AND stage=(SELECT stage FROM brief_research_stages WHERE run_id=$1 AND status<>'completed' ORDER BY CASE stage WHEN 'research' THEN 1 WHEN 'reconciliation' THEN 2 WHEN 'drafting' THEN 3 ELSE 4 END LIMIT 1)`, r.ID, code); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE brief_research_attempts SET status=CASE WHEN $2='cancelled' THEN 'cancelled' ELSE 'failed' END,reason=$3,finished_at=now() WHERE run_id=$1 AND status='running'`, r.ID, status, code); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE brief_research_runs SET status = $2, reason = $3, updated_at = now() WHERE id = $1`, r.ID, status, code); err != nil {
		return err
	}
	if b.Version == r.BriefVersion && b.Status == models.BriefProcessing {
		_, err := s.pool.Exec(ctx, `UPDATE route_briefs SET status = 'needs_research', reason = $2, version = version + 1, updated_at = now() WHERE id = $1`, b.ID, code+": "+models.ResearchReasonMessage(code))
		return err
	}
	return nil
}

// StopBriefResearch is used for bounded failures and safely ignores obsolete claims.
func (s *Store) StopBriefResearch(ctx context.Context, runID string, claim *models.Job, code string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		r, b, err := tx.lockResearch(ctx, runID, claim)
		if err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_stages SET status = 'needs_research', reason = $2, updated_at = now() WHERE run_id = $1 AND status = 'running'`, runID, code); err != nil {
			return err
		}
		return tx.stopResearchTx(ctx, r, b, "needs_research", code)
	})
}

// ChangeBriefResearch cancels or resumes a run at an expected brief version.
// Resume retains all checkpoints, attempts, reservations and the original deadline.
func (s *Store) ChangeBriefResearch(ctx context.Context, briefID string, in models.ResearchActionRequest, resume bool) (*models.ResearchRun, error) {
	err := s.Transaction(ctx, func(tx *Store) error {
		r, b, err := tx.lockResearch(ctx, in.RunID, nil)
		if err != nil {
			return err
		}
		if b.ID != briefID {
			return ErrNotFound
		}
		if b.Version != in.Version {
			return fmt.Errorf("%w: brief changed; reload before changing research", ErrConflict)
		}
		if !resume {
			if err := tx.stopResearchTx(ctx, r, b, "cancelled", "cancelled"); err != nil {
				return err
			}
			_, err = tx.pool.Exec(ctx, `UPDATE jobs SET status='done',finished_at=now(),last_error='research_cancelled',updated_at=now() WHERE id=$1`, r.JobID)
			return err
		}
		if (r.Status != "cancelled" && r.Status != "needs_research") || b.Status != models.BriefNeedsResearch {
			return fmt.Errorf("%w: run cannot be resumed", ErrConflict)
		}
		var unchanged bool
		raw, err := json.Marshal(r.Submitted)
		if err != nil {
			return err
		}
		if err := tx.pool.QueryRow(ctx, `SELECT submitted = $2::jsonb FROM route_briefs WHERE id=$1`, b.ID, raw).Scan(&unchanged); err != nil {
			return err
		}
		if !unchanged {
			return fmt.Errorf("%w: submitted brief changed; start a new run", ErrConflict)
		}
		if time.Now().After(r.DeadlineAt) || r.RequestsUsed >= r.RequestLimit || r.TokensReserved+ResearchTokenReservation > r.TokenLimit {
			return fmt.Errorf("%w: run budget exhausted; review before starting another run", ErrConflict)
		}
		var exhausted bool
		if err := tx.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brief_research_stages WHERE run_id=$1 AND status<>'completed' AND attempts>=3)`, r.ID).Scan(&exhausted); err != nil {
			return err
		}
		if exhausted {
			return fmt.Errorf("%w: stage retry budget exhausted", ErrConflict)
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE route_briefs SET status='processing',reason='Research resumed.',version=version+1,updated_at=now() WHERE id=$1`, b.ID); err != nil {
			return err
		}
		// A fresh job ID fences a cancelled worker even if its attempt number matches.
		job, err := tx.EnqueueReturning(ctx, models.JobKindResearchBrief, models.ResearchBriefPayload{RunID: r.ID})
		if err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE jobs SET max_attempts=12 WHERE id=$1`, job.ID); err != nil {
			return err
		}
		_, err = tx.pool.Exec(ctx, `UPDATE brief_research_runs SET status='active',reason='',brief_version=$2,job_id=$3,updated_at=now() WHERE id=$1`, r.ID, b.Version+1, job.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetResearchRun(ctx, in.RunID)
}

// BeginResearchStage durably reserves work before invoking a provider. It returns
// false for an already completed stage or a run stopped by a named budget.
func (s *Store) BeginResearchStage(ctx context.Context, runID, stage string, claim *models.Job) (bool, error) {
	started := false
	err := s.Transaction(ctx, func(tx *Store) error {
		r, b, err := tx.lockResearch(ctx, runID, claim)
		if err != nil {
			return err
		}
		if r.Status != "active" {
			return ErrResearchInactive
		}
		var status string
		var attempts int
		if err := tx.pool.QueryRow(ctx, `SELECT status,attempts FROM brief_research_stages WHERE run_id=$1 AND stage=$2`, runID, stage).Scan(&status, &attempts); err != nil {
			return mapErr(err)
		}
		if status == "completed" {
			return nil
		}
		code := ""
		switch {
		case b.Version != r.BriefVersion || b.Status != models.BriefProcessing:
			code = "brief_changed"
		case time.Now().After(r.DeadlineAt):
			code = "elapsed_budget"
		case attempts >= 3:
			code = "retry_budget"
		case r.RequestsUsed >= r.RequestLimit:
			code = "request_budget"
		case r.TokensReserved+ResearchTokenReservation > r.TokenLimit:
			code = "model_budget"
		}
		if code != "" {
			if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_stages SET status='needs_research',reason=$3,updated_at=now() WHERE run_id=$1 AND stage=$2`, runID, stage, code); err != nil {
				return err
			}
			return tx.stopResearchTx(ctx, r, b, "needs_research", code)
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_runs SET requests_used=requests_used+1,tokens_reserved=tokens_reserved+$2,updated_at=now() WHERE id=$1`, runID, ResearchTokenReservation); err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_attempts SET status='interrupted',reason='worker_reclaimed',finished_at=now() WHERE run_id=$1 AND stage=$2 AND status='running'`, runID, stage); err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `INSERT INTO brief_research_attempts (run_id,stage,attempt) VALUES ($1,$2,$3)`, runID, stage, attempts+1); err != nil {
			return err
		}
		_, err = tx.pool.Exec(ctx, `UPDATE brief_research_stages SET status='running',reason='',attempts=attempts+1,updated_at=now() WHERE run_id=$1 AND stage=$2`, runID, stage)
		started = err == nil
		return err
	})
	return started, err
}

// CommitResearchStage saves the output and pending candidates in one transaction.
// A replay skips completed checkpoints; existing candidate payloads are never updated.
func (s *Store) CommitResearchStage(ctx context.Context, runID, stage string, claim *models.Job, out models.ResearchOutput) error {
	return s.Transaction(ctx, func(tx *Store) error {
		r, b, err := tx.lockResearch(ctx, runID, claim)
		if err != nil {
			return err
		}
		if r.Status != "active" {
			return ErrResearchInactive
		}
		if b.Version != r.BriefVersion || b.Status != models.BriefProcessing {
			return tx.stopResearchTx(ctx, r, b, "needs_research", "brief_changed")
		}
		if time.Now().After(r.DeadlineAt) {
			return tx.stopResearchTx(ctx, r, b, "needs_research", "elapsed_budget")
		}
		var status string
		if err := tx.pool.QueryRow(ctx, `SELECT status FROM brief_research_stages WHERE run_id=$1 AND stage=$2`, runID, stage).Scan(&status); err != nil {
			return mapErr(err)
		}
		if status == "completed" {
			return nil
		}
		if status != "running" {
			return ErrResearchInactive
		}
		if len(out.Data) == 0 {
			out.Data = json.RawMessage(`{}`)
		}
		if !json.Valid(out.Data) || len(out.Data) > 262144 || len(out.Candidates) > 20 {
			return tx.stopResearchTx(ctx, r, b, "needs_research", "invalid_output")
		}
		for _, c := range out.Candidates {
			if !c.Kind.Valid() || !json.Valid(c.Payload) || len(c.Payload) > 65536 {
				return tx.stopResearchTx(ctx, r, b, "needs_research", "invalid_output")
			}
		}
		ids := []string{}
		if out.Reconciliations != nil {
			if stage != "reconciliation" {
				return tx.stopResearchTx(ctx, r, b, "needs_research", "invalid_output")
			}
			records, err := tx.commitReconciliation(ctx, b, runID, out.Reconciliations)
			if err != nil {
				return err
			}
			if time.Now().After(r.DeadlineAt) {
				return ErrResearchInactive
			}
			out.Data, err = json.Marshal(models.ReconciliationReport{Records: records})
			if err != nil {
				return err
			}
			if len(out.Data) > 262144 {
				return ErrInvalidReference
			} // roll back candidates and checkpoint together
			for _, record := range records {
				if record.PlaceCandidateID != nil {
					ids = append(ids, *record.PlaceCandidateID)
				}
				if record.MomentCandidateID != nil {
					ids = append(ids, *record.MomentCandidateID)
				}
			}
		}
		for i, c := range out.Candidates {
			externalID := fmt.Sprintf("%s:%s:%d", runID, stage, i)
			var id string
			if err := tx.pool.QueryRow(ctx, `INSERT INTO candidates (kind,origin,source_name,external_id,submitted_by_account_id,target_place_id,target_moment_id,payload) VALUES ($1,'draft','brief_research',$2,$3,$4,$5,$6) ON CONFLICT (source_name,external_id) WHERE source_name IS NOT NULL AND external_id IS NOT NULL DO NOTHING RETURNING id`, c.Kind, externalID, b.OwnerAccountID, nullable(c.TargetPlaceID), nullable(c.TargetMomentID), c.Payload).Scan(&id); err != nil {
				if !errors.Is(mapErr(err), ErrNotFound) {
					return mapErr(err)
				}
				if err := tx.pool.QueryRow(ctx, `SELECT id FROM candidates WHERE source_name='brief_research' AND external_id=$1`, externalID).Scan(&id); err != nil {
					return err
				}
			}
			ids = append(ids, id)
		}
		stageStatus := "completed"
		if out.NeedsResearch != "" {
			stageStatus = "needs_research"
			if models.ResearchReasonMessage(out.NeedsResearch) == "" {
				out.NeedsResearch = "insufficient_evidence"
			}
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_stages SET status=$5,reason=$6,output=$3,candidate_ids=$4,updated_at=now() WHERE run_id=$1 AND stage=$2`, runID, stage, out.Data, ids, stageStatus, out.NeedsResearch); err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_attempts SET status='completed',reason=$3,finished_at=now() WHERE run_id=$1 AND stage=$2 AND status='running'`, runID, stage, out.NeedsResearch); err != nil {
			return err
		}
		if out.NeedsResearch != "" {
			return tx.stopResearchTx(ctx, r, b, "needs_research", out.NeedsResearch)
		}
		var pending int
		if err := tx.pool.QueryRow(ctx, `SELECT count(*) FROM brief_research_stages WHERE run_id=$1 AND status<>'completed'`, runID).Scan(&pending); err != nil {
			return err
		}
		if pending == 0 {
			if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_runs SET status='completed',reason='',updated_at=now() WHERE id=$1`, runID); err != nil {
				return err
			}
			_, err = tx.pool.Exec(ctx, `UPDATE route_briefs SET status='review',reason='Research stages completed; curator review required.',version=version+1,updated_at=now() WHERE id=$1`, b.ID)
			return err
		}
		return nil
	})
}

// RecordResearchFailure stores only a safe failure code, never a provider error body.
func (s *Store) RecordResearchFailure(ctx context.Context, runID, stage string, claim *models.Job, code string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		r, _, err := tx.lockResearch(ctx, runID, claim)
		if err != nil {
			return err
		}
		if r.Status != "active" {
			return ErrResearchInactive
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE brief_research_attempts SET status='failed',reason=$3,finished_at=now() WHERE run_id=$1 AND stage=$2 AND status='running'`, runID, stage, code); err != nil {
			return err
		}
		_, err = tx.pool.Exec(ctx, `UPDATE brief_research_stages SET reason=$3,updated_at=now() WHERE run_id=$1 AND stage=$2 AND status='running'`, runID, stage, code)
		return err
	})
}
