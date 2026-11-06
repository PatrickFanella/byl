package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/sources"
)

// nearestPlaceMaxM is how close a find must be to a published place to target it automatically.
const nearestPlaceMaxM = 150

// importSource runs one adapter and turns its finds into pending candidates.
// It returns the completion note ("imported N, skipped M").
func (w *Worker) importSource(ctx context.Context, p models.ImportSourcePayload) (string, error) {
	if w.Sources == nil {
		return "", fmt.Errorf("%w: sources are not configured", ErrNoRetry)
	}
	adapter, ok := w.Sources.Get(p.Source)
	if !ok {
		return "", fmt.Errorf("%w: unknown source %q", ErrNoRetry, p.Source)
	}
	city, err := w.store.GetCity(ctx, p.CityID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", fmt.Errorf("%w: unknown city %q", ErrNoRetry, p.CityID)
		}
		return "", err
	}
	req := sources.Request{CityID: city.ID, Center: geo.Point{Lat: city.CenterLat, Lng: city.CenterLng}, RadiusM: p.RadiusM, Query: p.Query, Limit: p.Limit}
	if p.PlaceID != nil && *p.PlaceID != "" {
		place, err := w.store.GetPlace(ctx, *p.PlaceID, "", true)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return "", fmt.Errorf("%w: unknown place %q", ErrNoRetry, *p.PlaceID)
			}
			return "", err
		}
		req.Center = geo.Point{Lat: place.Latitude, Lng: place.Longitude}
	} else if p.Lat != nil && p.Lng != nil {
		req.Center = geo.Point{Lat: *p.Lat, Lng: *p.Lng}
	}

	var finds []sources.Found
	var notes []string
	if detailed, ok := adapter.(interface {
		DiscoverWithNotes(context.Context, sources.Request) ([]sources.Found, []string, error)
	}); ok {
		finds, notes, err = detailed.DiscoverWithNotes(ctx, req)
	} else {
		finds, err = adapter.Discover(ctx, req)
	}
	if err != nil {
		return "", err
	}
	imported, skipped := 0, 0
	seen := map[string]bool{}
	for _, f := range finds {
		if f.ExternalID == "" || seen[f.ExternalID] {
			skipped++
			continue
		}
		seen[f.ExternalID] = true
		exists, err := w.store.CandidateExists(ctx, adapter.Name(), f.ExternalID)
		if err != nil {
			return "", err
		}
		if exists {
			skipped++
			continue
		}
		cand, err := w.candidateFor(ctx, city.ID, p.PlaceID, adapter.Name(), f)
		if err != nil {
			return "", err
		}
		if _, err := w.store.CreateCandidate(ctx, cand); err != nil {
			if errors.Is(err, repository.ErrConflict) {
				skipped++
				continue
			}
			return "", err
		}
		imported++
	}
	note := fmt.Sprintf("imported %d, skipped %d", imported, skipped)
	if len(notes) > 0 {
		note += "; pages skipped: " + strings.Join(notes, "; ")
	}
	return note, nil
}

// candidateFor shapes a find into a candidate row. Moment finds are attached to the
// explicit place, else the nearest published place within 150 m, else left for the curator.
func (w *Worker) candidateFor(ctx context.Context, cityID string, explicitPlace *string, source string, f sources.Found) (models.Candidate, error) {
	ext := f.ExternalID
	src := source
	cand := models.Candidate{Kind: f.Kind, Origin: models.CandidateOriginImport, SourceName: &src, ExternalID: &ext}
	raw := f.Raw
	if len(raw) == 0 {
		raw = json.RawMessage(`null`)
	}
	switch f.Kind {
	case models.CandidateKindPlace:
		payload := map[string]any{
			"city_id": cityID, "name": f.Title, "summary": f.Text, "kind": "site",
			"latitude": deref(f.Lat), "longitude": deref(f.Lng), "arrival_radius_m": 40,
			"source_url": f.URL, "source_license": f.License, "source_credit": f.Credit, "image_url": f.MediaURL, "raw": raw,
		}
		cand.Payload = mustJSON(payload)
	case models.CandidateKindMoment:
		var placeID *string
		if explicitPlace != nil && *explicitPlace != "" {
			placeID = explicitPlace
		} else if f.Lat != nil && f.Lng != nil {
			if near, err := w.store.NearestPublishedPlace(ctx, cityID, geo.Point{Lat: *f.Lat, Lng: *f.Lng}, nearestPlaceMaxM); err == nil {
				placeID = &near.ID
			} else if !errors.Is(err, repository.ErrNotFound) {
				return cand, err
			}
		}
		cand.TargetPlaceID = placeID
		payload := map[string]any{
			"title": f.Title, "raw_text": f.Text, "source_url": f.URL, "source_title": f.Title,
			"source_publisher": f.Credit, "source_license": f.License, "source_license_url": f.LicenseURL,
			"lat": f.Lat, "lng": f.Lng, "image_url": f.MediaURL, "raw": raw,
		}
		if placeID != nil {
			payload["place_id"] = *placeID
		}
		cand.Payload = mustJSON(payload)
	case models.CandidateKindMedia:
		// Keep an explicitly requested location available to the image reviewer.
		// Proximity alone does not establish that a photograph depicts that place.
		cand.TargetPlaceID = explicitPlace
		kind := models.MediaKindImage
		switch {
		case len(f.MimeType) >= 5 && f.MimeType[:5] == "video":
			kind = models.MediaKindVideo
		case f.MimeType == "application/pdf" || f.MimeType == "text/plain":
			kind = models.MediaKindDocument
		case len(f.MimeType) >= 5 && f.MimeType[:5] == "audio":
			kind = models.MediaKindAudio
		}
		payload := map[string]any{
			"kind": kind, "media_url": f.MediaURL, "mime_type": f.MimeType, "width": f.Width, "height": f.Height,
			"caption": firstNonEmpty(f.Text, f.Title), "credit": f.Credit, "license": f.License, "license_url": f.LicenseURL,
			"source_url": f.URL, "lat": f.Lat, "lng": f.Lng, "raw": raw,
		}
		if kind == models.MediaKindVideo {
			payload["external"] = true
			payload["public_url"] = f.URL
			delete(payload, "media_url")
		}
		cand.Payload = mustJSON(payload)
	default:
		return cand, fmt.Errorf("%w: unsupported find kind %q", ErrNoRetry, f.Kind)
	}
	return cand, nil
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
