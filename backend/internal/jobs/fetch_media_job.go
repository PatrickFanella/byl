package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"

	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/sources"
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
	resp, err := w.Sources.Client().GetPublic(ctx, payload.MediaURL)
	if err != nil {
		if errors.Is(err, sources.ErrPrivateHost) {
			return fmt.Errorf("%w: %v", ErrNoRetry, err)
		}
		return fmt.Errorf("download media: %w", err)
	}
	defer resp.Body.Close()
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = strings.ToLower(strings.TrimSpace(payload.MimeType))
	}
	prefix, want := "images", "image/"
	switch asset.Kind {
	case models.MediaKindVideo:
		prefix, want = "videos", "video/"
	case models.MediaKindAudio:
		prefix, want = "audio", "audio/"
	}
	if !strings.HasPrefix(contentType, want) {
		return fmt.Errorf("%w: %s asset but the download is %q", ErrNoRetry, asset.Kind, contentType)
	}
	name := path.Base(u.Path)
	if name == "" || name == "/" || name == "." {
		name = asset.ID
	}
	name = withExtension(name, contentType)
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

// preferredExt maps common media types to the extension the file server should see; the
// mime package's first choice is not always the familiar one (.jpe for image/jpeg).
var preferredExt = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp", "image/tiff": ".tif",
	"image/svg+xml": ".svg", "video/mp4": ".mp4", "video/webm": ".webm", "video/quicktime": ".mov", "video/ogg": ".ogv",
	"audio/mpeg": ".mp3", "audio/mp4": ".m4a", "audio/ogg": ".ogg", "audio/wav": ".wav",
}

// withExtension makes sure name ends with an extension that matches contentType.
func withExtension(name, contentType string) string {
	ext := preferredExt[contentType]
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		return name
	}
	if cur := strings.ToLower(path.Ext(name)); cur != "" {
		if cur == ext {
			return name
		}
		if t := mime.TypeByExtension(cur); strings.Split(t, ";")[0] == contentType {
			return name
		}
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	return name + ext
}
