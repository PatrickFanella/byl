package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

const fetchMediaMaxBytes = 64 << 20

// fetchMedia downloads an approved media candidate's file into blob storage and records it on the asset.
func (w *Worker) fetchMedia(ctx context.Context, p models.FetchMediaPayload) error {
	if w.Sources == nil || w.Sources.Client() == nil {
		return fmt.Errorf("%w: sources are not configured", ErrNoRetry)
	}
	cand, err := w.store.GetCandidate(ctx, p.CandidateID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%w: unknown candidate", ErrNoRetry)
		}
		return err
	}
	asset, err := w.store.GetMedia(ctx, p.AssetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%w: unknown asset", ErrNoRetry)
		}
		return err
	}
	if asset.StorageKey != "" {
		return nil // already fetched
	}
	var payload struct {
		MediaURL string `json:"media_url"`
		MimeType string `json:"mime_type"`
		Width    *int   `json:"width"`
		Height   *int   `json:"height"`
	}
	if err := json.Unmarshal(cand.Payload, &payload); err != nil || payload.MediaURL == "" {
		return fmt.Errorf("%w: candidate has no media_url", ErrNoRetry)
	}
	u, err := url.Parse(payload.MediaURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: media_url is not http(s)", ErrNoRetry)
	}
	resp, err := w.Sources.Client().Get(ctx, payload.MediaURL)
	if err != nil {
		return fmt.Errorf("download media: %w", err)
	}
	defer resp.Body.Close()
	contentType := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = payload.MimeType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	prefix := "images"
	switch asset.Kind {
	case models.MediaKindVideo:
		prefix = "videos"
	case models.MediaKindAudio:
		prefix = "audio"
	}
	name := path.Base(u.Path)
	if name == "" || name == "/" || name == "." {
		name = asset.ID
	}
	key := blob.KeyFor(prefix, name)
	body := io.LimitReader(resp.Body, fetchMediaMaxBytes+1)
	if err := w.blob.Put(ctx, key, contentType, body, resp.ContentLength); err != nil {
		return fmt.Errorf("store media: %w", err)
	}
	if width := payload.Width; width != nil && *width <= 0 {
		payload.Width = nil
	}
	if height := payload.Height; height != nil && *height <= 0 {
		payload.Height = nil
	}
	return w.store.UpdateMediaFile(ctx, asset.ID, key, w.blob.PublicURL(key), contentType, payload.Width, payload.Height)
}
