package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"invisible-museum/backend/internal/models"
)

// ReviewHappening compares the complete inspected package under serializable
// isolation. No historical entity or route publication participates in this write.
func (s *Store) ReviewHappening(ctx context.Context, id, actor string, in models.HappeningReviewRequest, now time.Time) (*models.HappeningDecision, error) {
	if !isUUID(id) || !isUUID(actor) || len(in.Version) != 64 || (in.Action != "approve" && in.Action != "reject") || strings.TrimSpace(in.Note) == "" || utf8.RuneCountInString(in.Note) > 4000 {
		return nil, ErrInvalidReference
	}
	tx, err := s.rawPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scoped := &Store{pool: transactionDB{tx}, rawPool: s.rawPool}
	out, err := scoped.reviewHappeningTx(ctx, id, actor, in, now)
	if err == nil {
		err = tx.Commit(ctx)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01") {
		return nil, fmt.Errorf("%w: happening changed concurrently; reload the review package", ErrConflict)
	}
	return out, err
}

func (s *Store) reviewHappeningTx(ctx context.Context, id, actor string, in models.HappeningReviewRequest, now time.Time) (*models.HappeningDecision, error) {
	// Match draft editing's place-before-happening lock order.
	var place string
	if err := s.pool.QueryRow(ctx, `SELECT place_id FROM happenings WHERE id=$1`, id).Scan(&place); err != nil {
		return nil, mapErr(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT id FROM places WHERE id=$1 FOR UPDATE`, place).Scan(&place); err != nil {
		return nil, mapErr(err)
	}
	var locked string
	if err := s.pool.QueryRow(ctx, `SELECT id FROM happenings WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return nil, mapErr(err)
	}
	var draft, previousAction, previousNote string
	var revision *string
	err := s.pool.QueryRow(ctx, `SELECT d.id,r.action,r.note,p.id FROM happening_reviews r JOIN happening_drafts d ON d.id=r.draft_id LEFT JOIN happening_revisions p ON p.draft_id=d.id WHERE d.happening_id=$1 AND r.package_sha256=$2`, id, in.Version).Scan(&draft, &previousAction, &previousNote, &revision)
	if err == nil {
		if previousAction != in.Action || previousNote != in.Note {
			return nil, ErrConflict
		}
		out := &models.HappeningDecision{DraftID: draft, Action: previousAction}
		if revision != nil {
			out.Revision, err = s.happeningRevision(ctx, *revision, now)
		}
		return out, err
	}
	if !errors.Is(mapErr(err), ErrNotFound) {
		return nil, err
	}
	p, err := s.happeningReviewPackage(ctx, id, now)
	if err != nil {
		return nil, err
	}
	if p.Version != in.Version {
		return nil, fmt.Errorf("%w: review package changed; reload before deciding", ErrConflict)
	}
	if p.ReviewedAction != "" || (in.Action == "approve" && !p.ReadyToPublish) {
		return nil, fmt.Errorf("%w: review tasks remain or a decision already exists", ErrConflict)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO happening_reviews(draft_id,reviewed_by,action,package_sha256,note,reviewed_at) VALUES($1,$2,$3,$4,$5,$6)`, p.Draft.DraftID, actor, in.Action, p.Version, in.Note, now.UTC()); err != nil {
		return nil, mapErr(err)
	}
	out := &models.HappeningDecision{DraftID: p.Draft.DraftID, Action: in.Action}
	if in.Action == "reject" {
		return out, nil
	}
	var revisionID string
	if err := s.pool.QueryRow(ctx, `SELECT gen_random_uuid()`).Scan(&revisionID); err != nil {
		return nil, err
	}
	public := *p.Preview
	public.RevisionID = revisionID
	raw, err := json.Marshal(public)
	if err != nil {
		return nil, err
	}
	private, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO happening_revisions(id,happening_id,draft_id,public_snapshot,private_package,published_at) VALUES($1,$2,$3,$4,$5,$6)`, revisionID, id, p.Draft.DraftID, raw, private, now.UTC()); err != nil {
		return nil, mapErr(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE happening_current SET published_revision_id=$2 WHERE happening_id=$1 AND draft_id=$3`, id, revisionID, p.Draft.DraftID); err != nil {
		return nil, err
	}
	out.Revision = &public
	return out, nil
}

func (s *Store) happeningRevision(ctx context.Context, revision string, now time.Time) (*models.Happening, error) {
	var raw, private []byte
	if err := s.pool.QueryRow(ctx, `SELECT public_snapshot,private_package FROM happening_revisions WHERE id=$1`, revision).Scan(&raw, &private); err != nil {
		return nil, mapErr(err)
	}
	return s.currentHappeningProjection(ctx, raw, private, now)
}

func (s *Store) currentHappeningProjection(ctx context.Context, raw, private []byte, now time.Time) (*models.Happening, error) {
	var h models.Happening
	var p models.HappeningReviewPackage
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(private, &p); err != nil {
		return nil, err
	}
	_, health, last, err := s.happeningSourceHealth(ctx, p.Evidence)
	if err != nil {
		return nil, err
	}
	h.SourceHealth = health
	h.LastCheckedAt = last
	h.Issues = []string{}
	if health != "unchanged" {
		h.Issues = append(h.Issues, "source_"+health)
	}
	h.RefreshStatus(now, health == "unavailable")
	return &h, nil
}

// PublicHappenings reads only approved notice snapshots for an already-public
// place. It performs no discovery, retrieval, historical edit or trip mutation.
func (s *Store) PublicHappenings(ctx context.Context, place string, limit, offset int, now time.Time) ([]models.Happening, error) {
	id, err := s.ResolvePlaceID(ctx, place, "")
	if err != nil {
		return nil, err
	}
	var visible bool
	if err := s.pool.QueryRow(ctx, `SELECT status='published' FROM places WHERE id=$1`, id).Scan(&visible); err != nil {
		return nil, mapErr(err)
	}
	if !visible {
		return nil, ErrNotFound
	}
	if offset < 0 || offset > 10000 {
		return nil, ErrInvalidReference
	}
	rows, err := s.pool.Query(ctx, `SELECT r.public_snapshot,r.private_package FROM happenings h JOIN happening_current c ON c.happening_id=h.id JOIN happening_revisions r ON r.id=c.published_revision_id WHERE h.place_id=$1 ORDER BY r.published_at DESC,h.id LIMIT $2 OFFSET $3`, id, min(clamp(limit), 51), offset)
	if err != nil {
		return nil, err
	}
	// Close rows before source-health queries, including with a small pool.
	type snapshot struct{ public, private []byte }
	snapshots := []snapshot{}
	for rows.Next() {
		var r snapshot
		if err := rows.Scan(&r.public, &r.private); err != nil {
			rows.Close()
			return nil, err
		}
		snapshots = append(snapshots, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []models.Happening{}
	for _, r := range snapshots {
		h, err := s.currentHappeningProjection(ctx, r.public, r.private, now)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, nil
}
