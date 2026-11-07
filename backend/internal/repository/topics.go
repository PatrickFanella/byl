package repository

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"invisible-museum/backend/internal/models"
)

const topicSelect = `SELECT t.id, t.city_id,
	ARRAY(SELECT city_id::text FROM (SELECT t.city_id UNION SELECT tc.city_id FROM topic_cities tc WHERE tc.topic_id=t.id) coverage ORDER BY city_id),
	t.slug, t.name, t.description,
	(SELECT count(*) FROM moment_topics mt JOIN moments m ON m.id = mt.moment_id WHERE mt.topic_id = t.id AND m.status = 'published')
	FROM topics t`

func scanTopic(row interface{ Scan(...any) error }) (models.Topic, error) {
	var t models.Topic
	err := row.Scan(&t.ID, &t.CityID, &t.CityIDs, &t.Slug, &t.Name, &t.Description, &t.MomentCount)
	return t, err
}

// ListTopics returns a city's topics with published moment counts.
func (s *Store) ListTopics(ctx context.Context, cityID string) ([]models.Topic, error) {
	rows, err := s.pool.Query(ctx, topicSelect+` WHERE t.city_id = $1 OR EXISTS (SELECT 1 FROM topic_cities tc WHERE tc.topic_id=t.id AND tc.city_id=$1) ORDER BY t.name,t.id`, cityID)
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
		// Original city/slug references keep precedence. A shared slug must be unambiguous.
		rows, qerr := s.pool.Query(ctx, topicSelect+` WHERE t.slug=$1 AND (t.city_id=$2 OR EXISTS (SELECT 1 FROM topic_cities tc WHERE tc.topic_id=t.id AND tc.city_id=$2)) ORDER BY (t.city_id=$2) DESC,t.id LIMIT 2`, ref, cityID)
		if qerr != nil {
			return nil, mapErr(qerr)
		}
		defer rows.Close()
		if !rows.Next() {
			if rows.Err() != nil {
				return nil, rows.Err()
			}
			return nil, ErrNotFound
		}
		t, err = scanTopic(rows)
		if err == nil && t.CityID != cityID && rows.Next() {
			return nil, fmt.Errorf("%w: shared topic slug is ambiguous; use its ID", ErrConflict)
		}
		if err == nil {
			err = rows.Err()
		}
	}
	if err != nil {
		return nil, mapErr(err)
	}
	return &t, nil
}

// CreateTopic inserts a topic.
func (s *Store) CreateTopic(ctx context.Context, req models.CreateTopicRequest) (*models.Topic, error) {
	var id string
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		slug, err := uniqueSlug(ctx, tx, "topics", "city_id", req.CityID, slugFor(req.Slug, req.Name))
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO topics (city_id,slug,name,description) VALUES ($1,$2,$3,$4) RETURNING id`, req.CityID, slug, req.Name, req.Description).Scan(&id); err != nil {
			return err
		}
		return replaceTopicCities(ctx, tx, id, req.CityID, req.CityIDs)
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetTopic(ctx, id, "")
}

// UpdateTopic patches a topic.
func (s *Store) UpdateTopic(ctx context.Context, id string, req models.UpdateTopicRequest) (*models.Topic, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var cityID string
		if err := tx.QueryRow(ctx, `SELECT city_id FROM topics WHERE id=$1 FOR UPDATE`, id).Scan(&cityID); err != nil {
			return err
		}
		if req.CityIDs != nil {
			if err := replaceTopicCities(ctx, tx, id, cityID, req.CityIDs); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE topics SET name=COALESCE($2,name),description=COALESCE($3,description),updated_at=$4 WHERE id=$1`, id, req.Name, req.Description, time.Now())
		return err
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetTopic(ctx, id, "")
}

// TopicsBySlugs returns the city's topics matching any of slugs, in name order. Unknown slugs are ignored.
func (s *Store) TopicsBySlugs(ctx context.Context, cityID string, slugs []string) ([]models.Topic, error) {
	if len(slugs) == 0 {
		return []models.Topic{}, nil
	}
	rows, err := s.pool.Query(ctx, topicSelect+` WHERE (t.city_id=$1 OR EXISTS (SELECT 1 FROM topic_cities tc WHERE tc.topic_id=t.id AND tc.city_id=$1)) AND t.slug=ANY($2) ORDER BY t.name,t.id`, cityID, slugs)
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

func replaceTopicCities(ctx context.Context, q dbtx, id, primary string, given []string) error {
	cities := append([]string{primary}, given...)
	for i := range cities {
		cities[i] = strings.ToLower(cities[i])
	}
	slices.Sort(cities)
	cities = slices.Compact(cities)
	if len(cities) > 30 {
		return fmt.Errorf("%w: topic covers at most 30 municipalities", ErrInvalidReference)
	}
	for _, city := range cities {
		if !isUUID(city) {
			return fmt.Errorf("%w: unknown topic municipality", ErrInvalidReference)
		}
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM cities WHERE id=$1)`, city).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: unknown topic municipality", ErrInvalidReference)
		}
	}
	var used bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (
	 SELECT 1 FROM moment_topics mt JOIN moments m ON m.id=mt.moment_id JOIN places p ON p.id=m.place_id WHERE mt.topic_id=$1 AND NOT(p.city_id::text=ANY($2))
	 UNION ALL SELECT 1 FROM tours t WHERE t.topic_id=$1 AND NOT(t.city_id::text=ANY($2))
	 UNION ALL SELECT 1 FROM tours t JOIN tour_stops ts ON ts.tour_id=t.id JOIN places p ON p.id=ts.place_id WHERE t.topic_id=$1 AND NOT(p.city_id::text=ANY($2)))`, id, cities).Scan(&used); err != nil {
		return err
	}
	if used {
		return fmt.Errorf("%w: municipality is still used by a topic's moments or tours", ErrInvalidReference)
	}
	if _, err := q.Exec(ctx, `DELETE FROM topic_cities WHERE topic_id=$1`, id); err != nil {
		return err
	}
	for _, city := range cities {
		if _, err := q.Exec(ctx, `INSERT INTO topic_cities (topic_id,city_id) VALUES ($1,$2)`, id, city); err != nil {
			return err
		}
	}
	return nil
}

// requireTopicCities locks topic coverage against concurrent curator changes.
func requireTopicCities(ctx context.Context, q dbtx, id string, cities []string) error {
	if !isUUID(id) {
		return fmt.Errorf("%w: unknown topic", ErrInvalidReference)
	}
	var primary string
	if err := q.QueryRow(ctx, `SELECT city_id FROM topics WHERE id=$1 FOR SHARE`, id).Scan(&primary); err != nil {
		if mapErr(err) == ErrNotFound {
			return fmt.Errorf("%w: unknown topic", ErrInvalidReference)
		}
		return err
	}
	for _, city := range cities {
		if city == primary {
			continue
		}
		var covered bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM topic_cities WHERE topic_id=$1 AND city_id=$2)`, id, city).Scan(&covered); err != nil {
			return err
		}
		if !covered {
			return fmt.Errorf("%w: topic does not cover the referenced municipality", ErrInvalidReference)
		}
	}
	return nil
}
