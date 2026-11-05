package repository

import (
	"context"
	"time"

	"invisible-museum/backend/internal/models"
)

const cityCols = `id, slug, name, region, country_code, center_lat, center_lng, bbox_min_lat, bbox_min_lng, bbox_max_lat, bbox_max_lng, timezone, created_at, updated_at`

func scanCity(row interface{ Scan(...any) error }) (models.City, error) {
	var c models.City
	err := row.Scan(&c.ID, &c.Slug, &c.Name, &c.Region, &c.CountryCode, &c.CenterLat, &c.CenterLng,
		&c.BBox.MinLat, &c.BBox.MinLng, &c.BBox.MaxLat, &c.BBox.MaxLng, &c.Timezone, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// ListCities returns every city.
func (s *Store) ListCities(ctx context.Context) ([]models.City, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+cityCols+` FROM cities ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.City{}
	for rows.Next() {
		c, err := scanCity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ResolveCityID turns an id or slug into an id.
func (s *Store) ResolveCityID(ctx context.Context, ref string) (string, error) {
	var id string
	var err error
	if isUUID(ref) {
		err = s.pool.QueryRow(ctx, `SELECT id FROM cities WHERE id = $1`, ref).Scan(&id)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT id FROM cities WHERE slug = $1`, ref).Scan(&id)
	}
	return id, mapErr(err)
}

// GetCity returns a city with its topics.
func (s *Store) GetCity(ctx context.Context, ref string) (*models.CityDetail, error) {
	id, err := s.ResolveCityID(ctx, ref)
	if err != nil {
		return nil, err
	}
	c, err := scanCity(s.pool.QueryRow(ctx, `SELECT `+cityCols+` FROM cities WHERE id = $1`, id))
	if err != nil {
		return nil, mapErr(err)
	}
	topics, err := s.ListTopics(ctx, id)
	if err != nil {
		return nil, err
	}
	return &models.CityDetail{City: c, Topics: topics}, nil
}

// CreateCity inserts a city.
func (s *Store) CreateCity(ctx context.Context, req models.CreateCityRequest) (*models.CityDetail, error) {
	slug, err := uniqueSlug(ctx, s.pool, "cities", "", "", slugFor(req.Slug, req.Name))
	if err != nil {
		return nil, err
	}
	tz := req.Timezone
	if tz == "" {
		tz = "America/Chicago"
	}
	cc := req.CountryCode
	if cc == "" {
		cc = "US"
	}
	var id string
	err = s.pool.QueryRow(ctx, `INSERT INTO cities (slug, name, region, country_code, center_lat, center_lng, bbox_min_lat, bbox_min_lng, bbox_max_lat, bbox_max_lng, timezone)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		slug, req.Name, req.Region, cc, req.CenterLat, req.CenterLng, req.BBoxMinLat, req.BBoxMinLng, req.BBoxMaxLat, req.BBoxMaxLng, tz).Scan(&id)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetCity(ctx, id)
}

// UpdateCity patches a city.
func (s *Store) UpdateCity(ctx context.Context, id string, req models.UpdateCityRequest) (*models.CityDetail, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE cities SET
		name = COALESCE($2, name), region = COALESCE($3, region), country_code = COALESCE($4, country_code),
		center_lat = COALESCE($5, center_lat), center_lng = COALESCE($6, center_lng),
		bbox_min_lat = COALESCE($7, bbox_min_lat), bbox_min_lng = COALESCE($8, bbox_min_lng),
		bbox_max_lat = COALESCE($9, bbox_max_lat), bbox_max_lng = COALESCE($10, bbox_max_lng),
		timezone = COALESCE($11, timezone), updated_at = $12
		WHERE id = $1`,
		id, req.Name, req.Region, req.CountryCode, req.CenterLat, req.CenterLng,
		req.BBoxMinLat, req.BBoxMinLng, req.BBoxMaxLat, req.BBoxMaxLng, req.Timezone, time.Now())
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetCity(ctx, id)
}
