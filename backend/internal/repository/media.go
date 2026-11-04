package repository

import (
	"context"
	"time"

	"invisible-museum/backend/internal/models"
)

const mediaCols = `id, kind, storage_key, public_url, mime_type, width, height, duration_seconds, caption, credit, license, license_url, source_url, status, created_at, updated_at`

func scanMedia(row interface{ Scan(...any) error }) (models.MediaAsset, error) {
	var m models.MediaAsset
	err := row.Scan(&m.ID, &m.Kind, &m.StorageKey, &m.PublicURL, &m.MimeType, &m.Width, &m.Height, &m.DurationSeconds,
		&m.Caption, &m.Credit, &m.License, &m.LicenseURL, &m.SourceURL, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

// CreateMedia inserts an asset row.
func (s *Store) CreateMedia(ctx context.Context, m models.MediaAsset) (*models.MediaAsset, error) {
	if m.Status == "" {
		m.Status = models.EntityStatusDraft
	}
	var id string
	err := s.pool.QueryRow(ctx, `INSERT INTO media_assets (kind, storage_key, public_url, mime_type, width, height, duration_seconds, caption, credit, license, license_url, source_url, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		m.Kind, m.StorageKey, m.PublicURL, m.MimeType, m.Width, m.Height, m.DurationSeconds, m.Caption, m.Credit, m.License, m.LicenseURL, m.SourceURL, m.Status).Scan(&id)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetMedia(ctx, id)
}

// GetMedia returns one asset.
func (s *Store) GetMedia(ctx context.Context, id string) (*models.MediaAsset, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	m, err := scanMedia(s.pool.QueryRow(ctx, `SELECT `+mediaCols+` FROM media_assets WHERE id = $1`, id))
	if err != nil {
		return nil, mapErr(err)
	}
	return &m, nil
}

// ListMedia returns assets newest first.
func (s *Store) ListMedia(ctx context.Context, limit, offset int) ([]models.MediaAsset, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+mediaCols+` FROM media_assets ORDER BY created_at DESC LIMIT $1 OFFSET $2`, clamp(limit), offset)
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

// UpdateMediaMetadata patches caption, credit, and licence fields.
func (s *Store) UpdateMediaMetadata(ctx context.Context, id string, req models.MediaMetadataRequest) (*models.MediaAsset, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE media_assets SET caption = COALESCE($2, caption), credit = COALESCE($3, credit),
		license = COALESCE($4, license), license_url = COALESCE($5, license_url), source_url = COALESCE($6, source_url), updated_at = $7 WHERE id = $1`,
		id, req.Caption, req.Credit, req.License, req.LicenseURL, req.SourceURL, time.Now())
	if err != nil {
		return nil, mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetMedia(ctx, id)
}

// SetMediaStatus changes an asset's status.
func (s *Store) SetMediaStatus(ctx context.Context, id string, status models.EntityStatus) error {
	tag, err := s.pool.Exec(ctx, `UPDATE media_assets SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) mediaByIDs(ctx context.Context, q dbtx, ids []string) (map[string]models.MediaAsset, error) {
	out := map[string]models.MediaAsset{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT `+mediaCols+` FROM media_assets WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out[m.ID] = m
	}
	return out, rows.Err()
}

// UpdateMediaFile records where an asset's file landed.
func (s *Store) UpdateMediaFile(ctx context.Context, id, storageKey, publicURL, mimeType string, width, height *int) error {
	tag, err := s.pool.Exec(ctx, `UPDATE media_assets SET storage_key = $2, public_url = $3, mime_type = $4, width = COALESCE($5, width), height = COALESCE($6, height), updated_at = now() WHERE id = $1`,
		id, storageKey, publicURL, mimeType, width, height)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
