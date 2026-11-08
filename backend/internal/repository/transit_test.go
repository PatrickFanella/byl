package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
)

func transitFixture(t *testing.T) (*Store, context.Context, string, *models.TourRevision, models.TransitSelectionRequest, *models.ApprovedTransit) {
	t.Helper()
	s, ctx, actor, original := revisionFixture(t)
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	intent := models.TransitIntent{Version: 1, Provider: "cta_gtfs_blue_v1", Mode: models.TravelModeWalk, FromStopID: working.Stops[0].ID, ToStopID: working.Stops[1].ID, BoardStopID: "30374", AlightStopID: "30077", WalkToStopSeconds: 300, WalkFromStopSeconds: 600, BoardingBufferSeconds: 180, WalkingAssumptions: "Explicitly synthetic persistence fixture; not walking access or field evidence."}
	request := models.TransitSelectionRequest{DraftVersion: working.DraftVersion, Intent: &intent, ReviewNote: "Synthetic curator selection for persistence tests"}
	verified := &models.ApprovedTransit{Intent: intent, Board: models.TransitPlatform{ID: "30374", Name: "Clark/Lake (Subway)", ParentID: "40380", Latitude: 41.885737, Longitude: -87.630886}, Alight: models.TransitPlatform{ID: "30077", Name: "Forest Park", ParentID: "40390", Latitude: 41.874257, Longitude: -87.817318}, Source: models.TransitSource{URL: "https://www.transitchicago.com/downloads/sch_data/google_transit.zip", Publisher: "Chicago Transit Authority", SHA256: strings.Repeat("a", 64), RetrievedAt: time.Now(), LicenseURL: "https://www.transitchicago.com/developers/terms/", Timezone: "America/Chicago"}}
	return s, ctx, actor, original, request, verified
}

func TestTransitRevisionRetainsSelectionAndNeverPublishesRailAsWalking(t *testing.T) {
	s, ctx, actor, original, in, verified := transitFixture(t)
	before, err := json.Marshal(original.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutProgress(ctx, models.TourProgress{AccountID: actor, TourID: original.TourID, Mode: models.TravelModeWalk, VisitedPositions: []int{1}}, false); err != nil {
		t.Fatal(err)
	}
	choice, err := s.SetTourTransit(ctx, original.TourID, actor, in, verified, time.Now())
	if err != nil || choice.Version != 1 {
		t.Fatalf("selection: %v", err)
	}
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	if working.DraftVersion != in.DraftVersion+1 || working.Journeys[models.TravelModeWalk].Usable {
		t.Fatal("draft/version or old continuous walking journey still usable")
	}
	if _, err := s.pool.Exec(ctx, `UPDATE tour_revision_state SET release_requested_version=draft_version WHERE tour_id=$1`, working.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Transaction(ctx, func(tx *Store) error { return tx.publishRequestedRevisionTx(ctx, working.ID) }); err != nil {
		t.Fatal(err)
	}
	var current string
	if err := s.pool.QueryRow(ctx, `SELECT current_revision_id FROM tour_revision_state WHERE tour_id=$1`, working.ID).Scan(&current); err != nil || current != original.ID {
		t.Fatal("legacy worker auto-published transit")
	}
	release, err := s.PublishTourRevision(ctx, working.ID, actor, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: working.RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	if release.Snapshot.Tour.Transit == nil || release.Snapshot.Tour.Transit.Version != 1 {
		t.Fatal("selection not frozen")
	}
	if _, ok := release.Snapshot.Routes[models.TravelModeWalk]; ok {
		t.Fatal("continuous walking line published for transit")
	}
	if _, ok := release.Snapshot.Tour.Routes[models.TravelModeWalk]; ok {
		t.Fatal("continuous walking summary published for transit")
	}
	if _, err := s.PutProgress(ctx, models.TourProgress{AccountID: actor, TourID: original.TourID, RevisionID: original.ID, Mode: models.TravelModeWalk}, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed reset target ignored expected revision: %v", err)
	}
	var pin string
	if err := s.pool.QueryRow(ctx, `SELECT revision_id FROM legacy_revision_pins WHERE account_id=$1 AND tour_id=$2`, actor, original.TourID).Scan(&pin); err != nil || pin != original.ID {
		t.Fatal("conflicting target committed a pin change")
	}
	for _, leg := range release.Snapshot.Tour.Transit.WalkingLegs {
		if leg.FromPosition == 1 && leg.ToPosition == 2 {
			t.Fatal("rail-replaced walking leg retained")
		}
	}
	old, err := s.GetTourRevision(ctx, original.TourID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(old.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("publishing transit changed older immutable snapshot")
	}
}

func TestTransitChoicesAreFencedImmutableAndRemovalIsExplicit(t *testing.T) {
	s, ctx, actor, original, in, verified := transitFixture(t)
	if _, err := s.SetTourTransit(ctx, original.TourID, actor, in, verified, time.Now()); err != nil {
		t.Fatal(err)
	}
	selected, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	mixed, err := s.PublishTourRevision(ctx, selected.ID, actor, models.PublishRevisionRequest{DraftVersion: selected.DraftVersion, CurrentRevisionID: selected.RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTourTransit(ctx, original.TourID, actor, in, verified, time.Now()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale draft replaced selection: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE tour_transit_choices SET review_note='rewritten' WHERE tour_id=$1`, original.TourID); err == nil {
		t.Fatal("reviewed selection could be rewritten")
	}
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := s.SetTourTransit(ctx, working.ID, actor, models.TransitSelectionRequest{DraftVersion: working.DraftVersion, Remove: true, ReviewNote: "Explicit synthetic removal"}, nil, time.Now())
	if err != nil || removed != nil {
		t.Fatalf("removal: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE tour_revision_state SET release_requested_version=draft_version WHERE tour_id=$1`, working.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Transaction(ctx, func(tx *Store) error { return tx.publishRequestedRevisionTx(ctx, working.ID) }); err != nil {
		t.Fatal(err)
	}
	var current string
	if err := s.pool.QueryRow(ctx, `SELECT current_revision_id FROM tour_revision_state WHERE tour_id=$1`, working.ID).Scan(&current); err != nil || current != mixed.ID {
		t.Fatalf("pending removal auto-published: %s %v", current, err)
	}
	var retained int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM tour_transit_choices WHERE tour_id=$1`, working.ID).Scan(&retained); err != nil || retained != 2 {
		t.Fatal("removal lost review history")
	}
}

func TestTransitRejectsInvalidStopsAndStaleSource(t *testing.T) {
	s, ctx, actor, original, in, verified := transitFixture(t)
	stale := *verified
	stale.Source.RetrievedAt = time.Now().Add(-48 * time.Hour)
	if _, err := s.SetTourTransit(ctx, original.TourID, actor, in, &stale, time.Now()); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("stale evidence: %v", err)
	}
	intent := *in.Intent
	intent.FromStopID, intent.ToStopID = intent.ToStopID, intent.FromStopID
	in.Intent = &intent
	if _, err := s.SetTourTransit(ctx, original.TourID, actor, in, verified, time.Now()); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("backward stops: %v", err)
	}
}

func TestTransitMigrationRetainsExistingRowsAndHashes(t *testing.T) {
	assertNavigationMigrationPreservesRows(t, 19)
}
