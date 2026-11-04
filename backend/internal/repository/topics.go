package repository

import (
	"context"
	"time"

	"invisible-museum/backend/internal/models"
)

const topicSelect = `SELECT t.id, t.city_id, t.slug, t.name, t.description,
	(SELECT count(*) FROM moment_topics mt JOIN moments m ON m.id = mt.moment_id WHERE mt.topic_id = t.id AND m.status = 'published')
	FROM topics t`

func scanTopic(row interface{ Scan(...any) error }) (models.Topic, error) {
	var t models.Topic
	err := row.Scan(&t.ID, &t.CityID, &t.Slug, &t.Name, &t.Description, &t.MomentCount)
	return t, err
}

// ListTopics returns a city's topics with published moment counts.
func (s *Store) ListTopics(ctx context.Context, cityID string) ([]models.Topic, error) {
	rows, err := s.pool.Query(ctx, topicSelect+` WHERE t.city_id = $1 ORDER BY t.name`, cityID)
	if err != nil {
		return nil, mapErr(err)
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

// GetTopic resolves a topic by id, or by slug within cityRef (id or slug).
func (s *Store) GetTopic(ctx context.Context, ref, cityRef string) (*models.Topic, error) {
	var t models.Topic
	var err error
	if isUUID(ref) {
		t, err = scanTopic(s.pool.QueryRow(ctx, topicSelect+` WHERE t.id = $1`, ref))
	} else {
		cityID, cerr := s.ResolveCityID(ctx, cityRef)
		if cerr != nil {
			return nil, cerr
		}
		t, err = scanTopic(s.pool.QueryRow(ctx, topicSelect+` WHERE t.slug = $1 AND t.city_id = $2`, ref, cityID))
	}
	if err != nil {
		return nil, mapErr(err)
	}
	return &t, nil
}

// CreateTopic inserts a topic.
func (s *Store) CreateTopic(ctx context.Context, req models.CreateTopicRequest) (*models.Topic, error) {
	slug, err := uniqueSlug(ctx, s.pool, "topics", "city_id", req.CityID, slugFor(req.Slug, req.Name))
	if err != nil {
		return nil, mapErr(err)
	}
	var id string
	err = s.pool.QueryRow(ctx, `INSERT INTO topics (city_id, slug, name, description) VALUES ($1,$2,$3,$4) RETURNING id`,
		req.CityID, slug, req.Name, req.Description).Scan(&id)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetTopic(ctx, id, "")
}

// UpdateTopic patches a topic.
func (s *Store) UpdateTopic(ctx context.Context, id string, req models.UpdateTopicRequest) (*models.Topic, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE topics SET name = COALESCE($2, name), description = COALESCE($3, description), updated_at = $4 WHERE id = $1`,
		id, req.Name, req.Description, time.Now())
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetTopic(ctx, id, "")
}
