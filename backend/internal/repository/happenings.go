package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"invisible-museum/backend/internal/models"
)

const happeningDraftCols = `h.id,d.id,d.version,d.content,c.published_revision_id,d.created_at`

func scanHappeningDraft(row interface{ Scan(...any) error }) (*models.HappeningDraft, error) {
	var d models.HappeningDraft
	var raw []byte
	if err := row.Scan(&d.ID, &d.DraftID, &d.Version, &raw, &d.PublishedRevisionID, &d.CreatedAt); err != nil {
		return nil, mapErr(err)
	}
	if err := json.Unmarshal(raw, &d.Input); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) GetHappeningDraft(ctx context.Context, id string) (*models.HappeningDraft, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanHappeningDraft(s.pool.QueryRow(ctx, `SELECT `+happeningDraftCols+` FROM happenings h JOIN happening_current c ON c.happening_id=h.id JOIN happening_drafts d ON d.id=c.draft_id WHERE h.id=$1`, id))
}

// SaveHappeningDraft appends a curator draft. No published content, place or
// historical moment is edited. Exact request retries retain the original draft.
func (s *Store) SaveHappeningDraft(ctx context.Context, id, actor, key string, in models.HappeningWriteRequest, now time.Time) (*models.HappeningDraft, error) {
	if !isUUID(actor) || !models.ValidContributionKey(key) || (id != "" && !isUUID(id)) || !isUUID(in.Input.PlaceID) || in.Version < 0 || (id == "" && in.Version != 0) {
		return nil, ErrInvalidReference
	}
	raw, err := json.Marshal(in.Input)
	if err != nil {
		return nil, err
	}
	request, _ := json.Marshal(struct {
		ID    string
		Input models.HappeningWriteRequest
	}{id, in})
	hash := CandidatePayloadHash(request)
	var out *models.HappeningDraft
	err = s.Transaction(ctx, func(tx *Store) error {
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,4646))`, actor); err != nil {
			return err
		}
		var existing, previous string
		err := tx.pool.QueryRow(ctx, `SELECT id,request_sha256 FROM happening_drafts WHERE created_by=$1 AND request_key=$2`, actor, key).Scan(&existing, &previous)
		if err == nil {
			if previous != hash {
				return ErrConflict
			}
			out, err = scanHappeningDraft(tx.pool.QueryRow(ctx, `SELECT `+happeningDraftCols+` FROM happening_drafts d JOIN happenings h ON h.id=d.happening_id JOIN happening_current c ON c.happening_id=h.id WHERE d.id=$1`, existing))
			return err
		}
		if !errors.Is(mapErr(err), ErrNotFound) {
			return err
		}
		place, err := scanPlace(tx.pool.QueryRow(ctx, `SELECT `+placeCols+` FROM places p WHERE p.id=$1 FOR UPDATE OF p`, in.Input.PlaceID), false)
		if err != nil {
			return err
		}
		city, err := tx.GetCity(ctx, place.CityID)
		if err != nil {
			return err
		}
		zone, err := time.LoadLocation(city.Timezone)
		if err != nil {
			return ErrInvalidReference
		}
		if len(in.Input.Validate(now.In(zone))) > 0 {
			return ErrInvalidReference
		}
		var count int
		if err := tx.pool.QueryRow(ctx, `SELECT count(*) FROM happening_drafts WHERE created_by=$1 AND created_at>$2`, actor, now.Add(-24*time.Hour)).Scan(&count); err != nil {
			return err
		}
		if count >= 40 {
			return ErrSubmissionLimit
		}
		if id == "" {
			if err := tx.pool.QueryRow(ctx, `SELECT count(*) FROM happenings h JOIN happening_current c ON c.happening_id=h.id JOIN happening_drafts d ON d.id=c.draft_id LEFT JOIN happening_revisions p ON p.id=c.published_revision_id WHERE h.place_id=$1 AND (p.public_snapshot->>'expires_on'>$2 OR (d.content->>'expires_on'>$2 AND NOT EXISTS(SELECT 1 FROM happening_reviews r WHERE r.draft_id=d.id AND r.action='reject')))`, place.ID, now.In(zone).Format("2006-01-02")).Scan(&count); err != nil {
				return err
			}
			if count >= 100 {
				return ErrSubmissionLimit
			}
			if err := tx.pool.QueryRow(ctx, `INSERT INTO happenings(place_id,created_by,created_at) VALUES($1,$2,$3) RETURNING id`, place.ID, actor, now.UTC()).Scan(&id); err != nil {
				return err
			}
		} else {
			var venue string
			if err := tx.pool.QueryRow(ctx, `SELECT place_id FROM happenings WHERE id=$1 FOR UPDATE`, id).Scan(&venue); err != nil {
				return mapErr(err)
			}
			current, err := tx.GetHappeningDraft(ctx, id)
			if err != nil {
				return err
			}
			if venue != place.ID || current.Version != in.Version {
				return fmt.Errorf("%w: happening changed or venue differs", ErrConflict)
			}
		}
		var draft string
		if err := tx.pool.QueryRow(ctx, `INSERT INTO happening_drafts(happening_id,version,content,created_by,request_key,request_sha256,created_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, id, in.Version+1, raw, actor, key, hash, now.UTC()).Scan(&draft); err != nil {
			return mapErr(err)
		}
		if _, err := tx.pool.Exec(ctx, `INSERT INTO happening_current(happening_id,draft_id) VALUES($1,$2) ON CONFLICT(happening_id) DO UPDATE SET draft_id=excluded.draft_id`, id, draft); err != nil {
			return err
		}
		out, err = tx.GetHappeningDraft(ctx, id)
		return err
	})
	return out, err
}

func (s *Store) ListHappeningDrafts(ctx context.Context, limit, offset int) ([]models.HappeningDraft, error) {
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `SELECT `+happeningDraftCols+` FROM happenings h JOIN happening_current c ON c.happening_id=h.id JOIN happening_drafts d ON d.id=c.draft_id ORDER BY d.created_at DESC,h.id LIMIT $1 OFFSET $2`, min(clamp(limit), 50), offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.HappeningDraft{}
	for rows.Next() {
		d, err := scanHappeningDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
