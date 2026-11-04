package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"invisible-museum/backend/internal/models"
)

const momentCols = `m.id, m.place_id, m.slug, m.title, m.body, m.date_start, m.date_end, m.date_precision, m.date_display, m.hero_media_id, m.narration_media_id, m.status, m.created_at, m.updated_at`

type momentRow struct {
	models.MomentDetail
	heroMediaID      *string
	narrationMediaID *string
}

func scanMomentRow(row interface{ Scan(...any) error }) (momentRow, error) {
	var r momentRow
	var start, end *time.Time
	err := row.Scan(&r.ID, &r.PlaceID, &r.Slug, &r.Title, &r.Body, &start, &end, &r.DatePrecision, &r.DateDisplay,
		&r.heroMediaID, &r.narrationMediaID, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, err
	}
	r.DateStart = models.DateFromTime(start)
	r.DateEnd = models.DateFromTime(end)
	r.Excerpt = models.Excerpt(r.Body)
	r.HasNarration = r.narrationMediaID != nil
	r.TopicIDs = []string{}
	return r, nil
}

// queryMoments runs a moment query and attaches hero media and topic ids.
func (s *Store) queryMoments(ctx context.Context, q dbtx, where string, args ...any) ([]momentRow, error) {
	rows, err := q.Query(ctx, `SELECT `+momentCols+` FROM moments m `+where, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []momentRow
	var ids, mediaIDs []string
	for rows.Next() {
		r, err := scanMomentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
		ids = append(ids, r.ID)
		if r.heroMediaID != nil {
			mediaIDs = append(mediaIDs, *r.heroMediaID)
		}
		if r.narrationMediaID != nil {
			mediaIDs = append(mediaIDs, *r.narrationMediaID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	media, err := s.mediaByIDs(ctx, q, mediaIDs)
	if err != nil {
		return nil, err
	}
	topicRows, err := q.Query(ctx, `SELECT mt.moment_id, mt.topic_id FROM moment_topics mt JOIN topics t ON t.id = mt.topic_id WHERE mt.moment_id = ANY($1) ORDER BY t.name`, ids)
	if err != nil {
		return nil, err
	}
	defer topicRows.Close()
	topicsByMoment := map[string][]string{}
	for topicRows.Next() {
		var mid, tid string
		if err := topicRows.Scan(&mid, &tid); err != nil {
			return nil, err
		}
		topicsByMoment[mid] = append(topicsByMoment[mid], tid)
	}
	for i := range out {
		if ids := topicsByMoment[out[i].ID]; ids != nil {
			out[i].TopicIDs = ids
		}
		if out[i].heroMediaID != nil {
			if m, ok := media[*out[i].heroMediaID]; ok {
				out[i].HeroMedia = &m
			}
		}
		if out[i].narrationMediaID != nil {
			if m, ok := media[*out[i].narrationMediaID]; ok {
				out[i].Narration = &m
			}
		}
	}
	return out, nil
}

// ListMoments returns a place's moments in date order.
func (s *Store) ListMoments(ctx context.Context, placeID string, includeUnpublished bool) ([]models.MomentSummary, error) {
	rows, err := s.queryMoments(ctx, s.pool, `WHERE m.place_id = $1 AND (m.status = 'published' OR $2) ORDER BY m.date_start NULLS LAST, m.title`, placeID, includeUnpublished)
	if err != nil {
		return nil, err
	}
	out := make([]models.MomentSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.MomentSummary)
	}
	return out, nil
}

// GetMoment returns a full moment.
func (s *Store) GetMoment(ctx context.Context, id string, includeUnpublished bool) (*models.MomentDetail, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	rows, err := s.queryMoments(ctx, s.pool, `WHERE m.id = $1 AND (m.status = 'published' OR $2)`, id, includeUnpublished)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	m := rows[0].MomentDetail
	if m.Media, err = s.momentMedia(ctx, id); err != nil {
		return nil, err
	}
	if m.Sources, err = s.momentSources(ctx, id); err != nil {
		return nil, err
	}
	if m.Topics, err = s.momentTopics(ctx, id); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) momentMedia(ctx context.Context, momentID string) ([]models.MediaAsset, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+mediaCols+` FROM media_assets a JOIN moment_media mm ON mm.media_id = a.id WHERE mm.moment_id = $1 ORDER BY mm.position`, momentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MediaAsset{}
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) momentSources(ctx context.Context, momentID string) ([]models.Source, error) {
	rows, err := s.pool.Query(ctx, `SELECT title, url, publisher, retrieved_at, license, note FROM moment_sources WHERE moment_id = $1 ORDER BY position`, momentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Source{}
	for rows.Next() {
		var src models.Source
		var retrieved *time.Time
		if err := rows.Scan(&src.Title, &src.URL, &src.Publisher, &retrieved, &src.License, &src.Note); err != nil {
			return nil, err
		}
		src.RetrievedAt = models.DateFromTime(retrieved)
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) momentTopics(ctx context.Context, momentID string) ([]models.Topic, error) {
	rows, err := s.pool.Query(ctx, topicSelect+` JOIN moment_topics mt ON mt.topic_id = t.id WHERE mt.moment_id = $1 ORDER BY t.name`, momentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Topic{}
	for rows.Next() {
		t, err := scanTopic(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateMoment inserts a draft moment with its sources and topics.
func (s *Store) CreateMoment(ctx context.Context, req models.CreateMomentRequest) (*models.MomentDetail, error) {
	start, _ := models.ParseDate(req.DateStart)
	end, _ := models.ParseDate(req.DateEnd)
	precision := req.DatePrecision
	if precision == "" {
		precision = models.DatePrecisionYear
	}
	var id string
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		slug, err := uniqueSlug(ctx, tx, "moments", "place_id", req.PlaceID, slugFor(req.Slug, req.Title))
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO moments (place_id, slug, title, body, date_start, date_end, date_precision, date_display, hero_media_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			req.PlaceID, slug, strings.TrimSpace(req.Title), req.Body, dateArg(start), dateArg(end), precision, req.DateDisplay, nullable(req.HeroMediaID)).Scan(&id); err != nil {
			return err
		}
		if err := replaceSources(ctx, tx, id, req.Sources); err != nil {
			return err
		}
		return replaceTopics(ctx, tx, id, req.TopicIDs)
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetMoment(ctx, id, true)
}

func dateArg(d *models.Date) any {
	if d == nil {
		return nil
	}
	return d.Time
}

// UpdateMoment patches a moment's scalar fields.
func (s *Store) UpdateMoment(ctx context.Context, id string, req models.UpdateMomentRequest) (*models.MomentDetail, error) {
	var startArg, endArg any
	setStart, setEnd := req.DateStart != nil, req.DateEnd != nil
	if setStart {
		d, _ := models.ParseDate(*req.DateStart)
		startArg = dateArg(d)
	}
	if setEnd {
		d, _ := models.ParseDate(*req.DateEnd)
		endArg = dateArg(d)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE moments SET title = COALESCE($2, title), body = COALESCE($3, body),
		date_start = CASE WHEN $4 THEN $5::date ELSE date_start END, date_end = CASE WHEN $6 THEN $7::date ELSE date_end END,
		date_precision = COALESCE($8, date_precision), date_display = COALESCE($9, date_display),
		hero_media_id = CASE WHEN $10 THEN $11::uuid ELSE hero_media_id END, updated_at = $12 WHERE id = $1`,
		id, req.Title, req.Body, setStart, startArg, setEnd, endArg, req.DatePrecision, req.DateDisplay,
		req.HeroMediaID != nil, nullable(req.HeroMediaID), time.Now())
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetMoment(ctx, id, true)
}

// SetMomentStatus changes a moment's status.
func (s *Store) SetMomentStatus(ctx context.Context, id string, status models.EntityStatus) error {
	tag, err := s.pool.Exec(ctx, `UPDATE moments SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMomentNarration links a narration asset.
func (s *Store) SetMomentNarration(ctx context.Context, id string, mediaID *string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE moments SET narration_media_id = $2, updated_at = now() WHERE id = $1`, id, nullable(mediaID))
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceMomentMedia sets the ordered media list.
func (s *Store) ReplaceMomentMedia(ctx context.Context, momentID string, mediaIDs []string) error {
	return mapErr(s.withTx(ctx, func(tx pgx.Tx) error {
		if err := requireMoment(ctx, tx, momentID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM moment_media WHERE moment_id = $1`, momentID); err != nil {
			return err
		}
		for i, mid := range mediaIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO moment_media (moment_id, media_id, position) VALUES ($1,$2,$3)`, momentID, mid, i+1); err != nil {
				return err
			}
		}
		return nil
	}))
}

// ReplaceMomentSources sets the ordered sources.
func (s *Store) ReplaceMomentSources(ctx context.Context, momentID string, sources []models.Source) error {
	return mapErr(s.withTx(ctx, func(tx pgx.Tx) error {
		if err := requireMoment(ctx, tx, momentID); err != nil {
			return err
		}
		return replaceSources(ctx, tx, momentID, sources)
	}))
}

// ReplaceMomentTopics sets the topic set.
func (s *Store) ReplaceMomentTopics(ctx context.Context, momentID string, topicIDs []string) error {
	return mapErr(s.withTx(ctx, func(tx pgx.Tx) error {
		if err := requireMoment(ctx, tx, momentID); err != nil {
			return err
		}
		return replaceTopics(ctx, tx, momentID, topicIDs)
	}))
}

func requireMoment(ctx context.Context, q dbtx, id string) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM moments WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func replaceSources(ctx context.Context, q dbtx, momentID string, sources []models.Source) error {
	if _, err := q.Exec(ctx, `DELETE FROM moment_sources WHERE moment_id = $1`, momentID); err != nil {
		return err
	}
	for i, src := range sources {
		if _, err := q.Exec(ctx, `INSERT INTO moment_sources (moment_id, position, title, url, publisher, retrieved_at, license, note) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			momentID, i+1, src.Title, src.URL, src.Publisher, dateArg(src.RetrievedAt), src.License, src.Note); err != nil {
			return err
		}
	}
	return nil
}

func replaceTopics(ctx context.Context, q dbtx, momentID string, topicIDs []string) error {
	if _, err := q.Exec(ctx, `DELETE FROM moment_topics WHERE moment_id = $1`, momentID); err != nil {
		return err
	}
	for _, tid := range topicIDs {
		if _, err := q.Exec(ctx, `INSERT INTO moment_topics (moment_id, topic_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, momentID, tid); err != nil {
			return err
		}
	}
	return nil
}

// MomentPublishBlockers lists reasons a moment cannot be published.
func (s *Store) MomentPublishBlockers(ctx context.Context, momentID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.id, a.caption FROM media_assets a
		WHERE a.license = '' AND (a.id IN (SELECT media_id FROM moment_media WHERE moment_id = $1) OR a.id = (SELECT hero_media_id FROM moments WHERE id = $1))`, momentID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, caption string
		if err := rows.Scan(&id, &caption); err != nil {
			return nil, err
		}
		label := id
		if caption != "" {
			label = fmt.Sprintf("%s (%s)", id, caption)
		}
		out = append(out, "media "+label+" has no license")
	}
	return out, rows.Err()
}
