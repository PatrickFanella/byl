package repository

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"invisible-museum/backend/internal/models"
)

func (s *Store) GetTourTransit(ctx context.Context, tour string) (*models.ApprovedTransit, error) {
	var raw []byte
	var version int64
	err := s.pool.QueryRow(ctx, `SELECT c.choice,c.version FROM tour_transit_current p JOIN tour_transit_choices c ON c.id=p.choice_id AND c.tour_id=p.tour_id WHERE p.tour_id=$1`, tour).Scan(&raw, &version)
	if errors.Is(mapErr(err), ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var choice models.ApprovedTransit
	if err = json.Unmarshal(raw, &choice); err != nil {
		return nil, err
	}
	choice.Version = version
	return &choice, nil
}

func validTransitPlatform(p models.TransitPlatform) bool {
	return p.ID != "" && p.ParentID != "" && strings.TrimSpace(p.Name) != "" && math.Abs(p.Latitude) <= 90 && math.Abs(p.Longitude) <= 180 && !math.IsNaN(p.Latitude) && !math.IsNaN(p.Longitude)
}

// SetTourTransit is an explicit curator selection and review, fenced against the
// complete tour draft. Completed choices are append-only; removal is a new choice.
func (s *Store) SetTourTransit(ctx context.Context, tour, actor string, in models.TransitSelectionRequest, verified *models.ApprovedTransit, now time.Time) (*models.ApprovedTransit, error) {
	if !isUUID(tour) || !isUUID(actor) || in.DraftVersion < 0 || in.Remove == (in.Intent != nil) || strings.TrimSpace(in.ReviewNote) == "" || len(in.ReviewNote) > 4000 {
		return nil, ErrInvalidReference
	}
	var raw any
	if in.Intent != nil {
		if len(in.Intent.Validate()) > 0 || !isUUID(in.Intent.FromStopID) || !isUUID(in.Intent.ToStopID) || verified == nil {
			return nil, ErrInvalidReference
		}
		hash, err := hex.DecodeString(verified.Source.SHA256)
		if err != nil || len(hash) != 32 || verified.Source.URL != "https://www.transitchicago.com/downloads/sch_data/google_transit.zip" || verified.Source.LicenseURL != "https://www.transitchicago.com/developers/terms/" || verified.Source.Timezone != "America/Chicago" || now.Sub(verified.Source.RetrievedAt) > 24*time.Hour || verified.Source.RetrievedAt.After(now.Add(5*time.Minute)) || !validTransitPlatform(verified.Board) || !validTransitPlatform(verified.Alight) || verified.Board.ID != in.Intent.BoardStopID || verified.Alight.ID != in.Intent.AlightStopID {
			return nil, ErrInvalidReference
		}
		choice := *verified
		choice.Intent = *in.Intent
		choice.Version = 0
		choice.WalkingLegs = nil
		choice.ReviewedAt = now.UTC()
		encoded, err := json.Marshal(choice)
		if err != nil {
			return nil, err
		}
		raw = encoded
	}
	err := s.Transaction(ctx, func(tx *Store) error {
		var id string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, tour).Scan(&id); err != nil {
			return mapErr(err)
		}
		working, err := tx.GetTour(ctx, tour, false)
		if err != nil {
			return err
		}
		if working.DraftVersion != in.DraftVersion {
			return ErrConflict
		}
		if in.Intent != nil && !in.Intent.MatchesStops(working) {
			return ErrInvalidReference
		}
		var choiceID string
		var version int64
		err = tx.pool.QueryRow(ctx, `INSERT INTO tour_transit_choices(tour_id,version,draft_version,choice,reviewed_by,review_note,reviewed_at) VALUES($1,COALESCE((SELECT version FROM tour_transit_current WHERE tour_id=$1),0)+1,$2,$3,$4,$5,$6) RETURNING id,version`, tour, in.DraftVersion, raw, actor, strings.TrimSpace(in.ReviewNote), now.UTC()).Scan(&choiceID, &version)
		if err != nil {
			return err
		}
		if _, err = tx.pool.Exec(ctx, `INSERT INTO tour_transit_current(tour_id,choice_id,version) VALUES($1,$2,$3) ON CONFLICT(tour_id) DO UPDATE SET choice_id=excluded.choice_id,version=excluded.version`, tour, choiceID, version); err != nil {
			return err
		}
		// A selected transit option is released only by an explicit publication;
		// ordinary routing workers must not auto-publish it on legacy save/share.
		_, err = tx.pool.Exec(ctx, `UPDATE tour_revision_state SET draft_version=draft_version+1,release_requested_version=NULL WHERE tour_id=$1 AND deleted_at IS NULL`, tour)
		return err
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetTourTransit(ctx, tour)
}
