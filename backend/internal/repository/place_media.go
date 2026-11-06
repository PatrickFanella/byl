package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	"invisible-museum/backend/internal/models"
)

func publishableMedia(assets []models.MediaAsset) []models.MediaAsset {
	out := []models.MediaAsset{}
	for _, asset := range assets {
		if asset.PublicationIssue() == "" {
			out = append(out, asset)
		}
	}
	return out
}

func (s *Store) PlaceMedia(ctx context.Context, placeID string, includeUnpublished bool) ([]models.MediaAsset, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+mediaCols+` FROM media_assets a JOIN place_media pm ON pm.media_id = a.id WHERE pm.place_id = $1 ORDER BY pm.position`, placeID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.MediaAsset{}
	for rows.Next() {
		asset, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		if includeUnpublished || asset.PublicationIssue() == "" {
			out = append(out, asset)
		}
	}
	return out, rows.Err()
}

func (s *Store) ReplacePlaceMedia(ctx context.Context, placeID string, ids []string) error {
	return mapErr(s.withTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM places WHERE id = $1)`, placeID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM place_media WHERE place_id = $1`, placeID); err != nil {
			return err
		}
		for i, id := range ids {
			if _, err := tx.Exec(ctx, `INSERT INTO place_media (place_id, media_id, position) VALUES ($1,$2,$3)`, placeID, id, i+1); err != nil {
				return err
			}
		}
		return nil
	}))
}

func (s *Store) MediaPublishBlockers(ctx context.Context, ids []string) ([]string, error) {
	out := []string{}
	for _, id := range ids {
		asset, err := s.GetMedia(ctx, id)
		if err != nil {
			return nil, err
		}
		if issue := asset.PublicationIssue(); issue != "" {
			out = append(out, "media "+id+": "+issue)
		}
	}
	return out, nil
}

func (s *Store) PublishedPlacesUsingMedia(ctx context.Context, id string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM places p JOIN place_media pm ON pm.place_id = p.id WHERE p.status = 'published' AND pm.media_id = $1`, id).Scan(&n)
	return n, mapErr(err)
}
