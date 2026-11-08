package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"invisible-museum/backend/internal/models"
)

// PublishCuratorPackage rechecks the complete reviewed read set and releases only
// selected prepared dependencies. Candidate materialization remains an explicit
// draft-review step; contextual candidates and proposed shared edits are untouched.
func (s *Store) PublishCuratorPackage(ctx context.Context, brief, curator string, in models.PublishPackageRequest) (*models.TourRevision, error) {
	if !isUUID(brief) || !isUUID(in.TourID) || !isUUID(in.RunID) {
		return nil, ErrNotFound
	}
	tx, err := s.rawPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scoped := &Store{pool: transactionDB{tx}, rawPool: s.rawPool}
	revision, err := scoped.publishCuratorPackageTx(ctx, brief, curator, in)
	if err == nil {
		err = tx.Commit(ctx)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01") {
		return nil, fmt.Errorf("%w: package changed concurrently; reload before approving", ErrConflict)
	}
	if err != nil {
		return nil, err
	}
	return revision, nil
}

func (s *Store) publishCuratorPackageTx(ctx context.Context, brief, curator string, in models.PublishPackageRequest) (*models.TourRevision, error) {
	// Serialize approvals and ordinary tour edits before reading the package.
	var id string
	if err := s.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, in.TourID).Scan(&id); err != nil {
		return nil, mapErr(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT id FROM route_briefs WHERE id=$1 FOR UPDATE`, brief).Scan(&id); err != nil {
		return nil, mapErr(err)
	}
	p, err := s.curatorReviewPackage(ctx, brief, in.RunID, in.TourID)
	if err != nil {
		return nil, err
	}
	if in.Version == "" || p.Version != in.Version {
		return nil, fmt.Errorf("%w: review package changed; reload before approving", ErrConflict)
	}
	if !p.ReadyToPublish {
		return nil, fmt.Errorf("%w: package has unresolved review tasks", ErrConflict)
	}
	// The same snapshot supplies checks, selected IDs and immutable release text.
	// Serializable isolation also fences concurrent changes to associations and
	// other rows that have no individual version counter.
	for _, kind := range []string{"media", "moment", "place"} {
		for _, entity := range p.Entities {
			if !entity.Required || entity.Candidate != nil || entity.Kind != kind || entity.Disposition == "reused_published" {
				continue
			}
			switch kind {
			case "media":
				err = s.SetMediaStatus(ctx, entity.ID, models.EntityStatusPublished)
			case "moment":
				err = s.SetMomentStatus(ctx, entity.ID, models.EntityStatusPublished)
				if err == nil {
					err = s.Enqueue(ctx, models.JobKindGenerateNarration, models.GenerateNarrationPayload{MomentID: entity.ID})
				}
			case "place":
				err = s.SetPlaceStatus(ctx, entity.ID, models.EntityStatusPublished)
			}
			if err != nil {
				return nil, err
			}
		}
	}
	approval, err := json.Marshal(models.PackageApproval{BriefID: brief, RunID: in.RunID, Version: in.Version, ReviewedBy: curator, Note: in.Note})
	if err != nil {
		return nil, err
	}
	revision, err := s.publishRevisionWithApprovalTx(ctx, in.TourID, curator, models.PublishRevisionRequest{DraftVersion: p.Tour.DraftVersion, CurrentRevisionID: p.Tour.RevisionID}, true, approval)
	if err != nil {
		return nil, err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE route_briefs SET status='accepted',reason=$2,version=version+1,updated_at=now() WHERE id=$1`, brief, in.Note); err != nil {
		return nil, err
	}
	return revision, nil
}
