package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

// PlaceFilter narrows ListPlaces.
type PlaceFilter struct {
	CityID             string
	IncludeUnpublished bool
	Near               *geo.Point
	RadiusM            float64
	BBox               *geo.BBox
	TopicID            string
	Limit              int
	Cursor             string
}

const placeCols = `p.id, p.city_id, p.slug, p.name, p.summary, p.kind, p.latitude, p.longitude, p.arrival_radius_m, p.address, p.status, p.primary_moment_id,
	(SELECT count(*) FROM moments m WHERE m.place_id = p.id AND m.status = 'published'), p.created_at, p.updated_at`

func scanPlace(row interface{ Scan(...any) error }, withDistance bool) (models.PlaceSummary, error) {
	var p models.PlaceSummary
	dest := []any{&p.ID, &p.CityID, &p.Slug, &p.Name, &p.Summary, &p.Kind, &p.Latitude, &p.Longitude, &p.ArrivalRadiusM, &p.Address, &p.Status, &p.PrimaryMomentID, &p.MomentCount, &p.CreatedAt, &p.UpdatedAt}
	if withDistance {
		dest = append(dest, &p.DistanceM)
	}
	err := row.Scan(dest...)
	return p, err
}

// ListPlaces returns places matching the filter plus the cursor for the next page ("" when done).
func (s *Store) ListPlaces(ctx context.Context, f PlaceFilter) ([]models.PlaceSummary, string, error) {
	args := []any{}
	where := []string{}
	add := func(v any) int { args = append(args, v); return len(args) }

	if f.CityID != "" {
		where = append(where, fmt.Sprintf("p.city_id = $%d", add(f.CityID)))
	}
	if !f.IncludeUnpublished {
		where = append(where, "p.status = 'published'")
	}
	if f.BBox != nil {
		where = append(where, fmt.Sprintf("p.latitude BETWEEN $%d AND $%d AND p.longitude BETWEEN $%d AND $%d",
			add(f.BBox.MinLat), add(f.BBox.MaxLat), add(f.BBox.MinLng), add(f.BBox.MaxLng)))
	}
	if f.TopicID != "" {
		where = append(where, fmt.Sprintf(`EXISTS (SELECT 1 FROM moments m JOIN moment_topics mt ON mt.moment_id = m.id WHERE m.place_id = p.id AND mt.topic_id = $%d AND (m.status = 'published' OR $%d))`, add(f.TopicID), add(f.IncludeUnpublished)))
	}

	cols := placeCols
	order := "p.name"
	withDistance := false
	if f.Near != nil {
		withDistance = true
		box := geo.BoundingBox(f.Near.Lat, f.Near.Lng, f.RadiusM)
		if f.RadiusM > 0 {
			where = append(where, fmt.Sprintf("p.latitude BETWEEN $%d AND $%d AND p.longitude BETWEEN $%d AND $%d",
				add(box.MinLat), add(box.MaxLat), add(box.MinLng), add(box.MaxLng)))
		}
		dist := distanceExpr(add(f.Near.Lat), add(f.Near.Lng), "p.latitude", "p.longitude")
		cols += ", " + dist + " AS distance_m"
		if f.RadiusM > 0 {
			where = append(where, fmt.Sprintf("%s <= $%d", dist, add(f.RadiusM)))
		}
		order = "distance_m, p.name"
	}

	limit := clamp(f.Limit)
	offset, _ := strconv.Atoi(f.Cursor)
	if offset < 0 {
		offset = 0
	}
	q := "SELECT " + cols + " FROM places p"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", order, add(limit+1), add(offset))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, "", mapErr(err)
	}
	defer rows.Close()
	out := []models.PlaceSummary{}
	for rows.Next() {
		p, err := scanPlace(rows, withDistance)
		if err != nil {
			return nil, "", err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = strconv.Itoa(offset + limit)
	}
	return out, next, nil
}

// ResolvePlaceID turns an id, or a slug within cityRef, into an id.
func (s *Store) ResolvePlaceID(ctx context.Context, ref, cityRef string) (string, error) {
	if isUUID(ref) {
		var id string
		err := s.pool.QueryRow(ctx, `SELECT id FROM places WHERE id = $1`, ref).Scan(&id)
		return id, mapErr(err)
	}
	var id string
	var err error
	if cityRef == "" {
		err = s.pool.QueryRow(ctx, `SELECT id FROM places WHERE slug = $1 ORDER BY created_at LIMIT 1`, ref).Scan(&id)
	} else {
		cityID, cerr := s.ResolveCityID(ctx, cityRef)
		if cerr != nil {
			return "", cerr
		}
		err = s.pool.QueryRow(ctx, `SELECT id FROM places WHERE slug = $1 AND city_id = $2`, ref, cityID).Scan(&id)
	}
	return id, mapErr(err)
}

// GetPlace returns a place with its moments. Unpublished places and moments are hidden unless includeUnpublished.
func (s *Store) GetPlace(ctx context.Context, ref, cityRef string, includeUnpublished bool) (*models.PlaceDetail, error) {
	id, err := s.ResolvePlaceID(ctx, ref, cityRef)
	if err != nil {
		return nil, err
	}
	p, err := scanPlace(s.pool.QueryRow(ctx, `SELECT `+placeCols+` FROM places p WHERE p.id = $1`, id), false)
	if err != nil {
		return nil, mapErr(err)
	}
	if !includeUnpublished && p.Status != models.EntityStatusPublished {
		return nil, ErrNotFound
	}
	moments, err := s.ListMoments(ctx, id, includeUnpublished)
	if err != nil {
		return nil, err
	}
	detail := &models.PlaceDetail{PlaceSummary: p, Moments: moments}
	if p.PrimaryMomentID != nil {
		for i := range moments {
			if moments[i].ID == *p.PrimaryMomentID {
				m := moments[i]
				detail.PrimaryMoment = &m
			}
		}
	}
	return detail, nil
}

// CreatePlace inserts a draft place.
func (s *Store) CreatePlace(ctx context.Context, req models.CreatePlaceRequest) (*models.PlaceDetail, error) {
	slug, err := uniqueSlug(ctx, s.pool, "places", "city_id", req.CityID, slugFor(req.Slug, req.Name))
	if err != nil {
		return nil, mapErr(err)
	}
	kind := req.Kind
	if kind == "" {
		kind = "site"
	}
	radius := req.ArrivalRadiusM
	if radius == 0 {
		radius = 40
	}
	var id string
	err = s.pool.QueryRow(ctx, `INSERT INTO places (city_id, slug, name, summary, kind, latitude, longitude, arrival_radius_m, address)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		req.CityID, slug, strings.TrimSpace(req.Name), req.Summary, kind, req.Latitude, req.Longitude, radius, req.Address).Scan(&id)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetPlace(ctx, id, "", true)
}

// UpdatePlace patches a place.
func (s *Store) UpdatePlace(ctx context.Context, id string, req models.UpdatePlaceRequest) (*models.PlaceDetail, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE places SET name = COALESCE($2, name), summary = COALESCE($3, summary), kind = COALESCE($4, kind),
		latitude = COALESCE($5, latitude), longitude = COALESCE($6, longitude), arrival_radius_m = COALESCE($7, arrival_radius_m),
		address = COALESCE($8, address), updated_at = $9 WHERE id = $1`,
		id, req.Name, req.Summary, req.Kind, req.Latitude, req.Longitude, req.ArrivalRadiusM, req.Address, time.Now())
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	if req.PrimaryMomentID != nil {
		if err := s.SetPrimaryMoment(ctx, id, req.PrimaryMomentID); err != nil {
			return nil, err
		}
	}
	return s.GetPlace(ctx, id, "", true)
}

// SetPlaceStatus changes a place's status.
func (s *Store) SetPlaceStatus(ctx context.Context, id string, status models.EntityStatus) error {
	tag, err := s.pool.Exec(ctx, `UPDATE places SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPrimaryMoment sets or clears (nil / "") the place's primary moment; the moment must belong to the place.
func (s *Store) SetPrimaryMoment(ctx context.Context, placeID string, momentID *string) error {
	if momentID == nil || strings.TrimSpace(*momentID) == "" {
		_, err := s.pool.Exec(ctx, `UPDATE places SET primary_moment_id = NULL, updated_at = now() WHERE id = $1`, placeID)
		return mapErr(err)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE places SET primary_moment_id = $2, updated_at = now()
		WHERE id = $1 AND EXISTS (SELECT 1 FROM moments WHERE id = $2 AND place_id = $1)`, placeID, *momentID)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: moment does not belong to place", ErrInvalidReference)
	}
	return nil
}

// UnpublishedOrForeign returns the 1-based stop positions whose place is not published or not in cityID.
func (s *Store) UnpublishedOrForeign(ctx context.Context, cityID string, placeIDs []string) ([]int, error) {
	var bad []int
	for i, id := range placeIDs {
		if !isUUID(id) {
			bad = append(bad, i+1)
			continue
		}
		var ok bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM places WHERE id = $1 AND city_id = $2 AND status = 'published')`, id, cityID).Scan(&ok); err != nil {
			return nil, mapErr(err)
		}
		if !ok {
			bad = append(bad, i+1)
		}
	}
	return bad, nil
}
