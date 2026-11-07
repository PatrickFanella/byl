package repository

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

// Compare the complete public projection, including fields derived from joins,
// rather than just the underlying stored rows copied into a snapshot.
func TestRevisionSnapshotPreservesPublicMetadata(t *testing.T) {
	s, ctx, author, previous := revisionFixture(t)
	audio, err := s.CreateMedia(ctx, models.MediaAsset{Kind: models.MediaKindAudio, PublicURL: "https://example.com/fixture.mp3", MimeType: "audio/mpeg", License: "CC0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMomentNarration(ctx, testdb.SeedMomentBurlingtonID, &audio.ID); err != nil {
		t.Fatal(err)
	}
	working, err := s.GetTour(ctx, previous.TourID, true)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]*models.MomentDetail{}
	for _, stop := range working.Stops {
		if stop.FeaturedMoment != nil {
			m, err := s.GetMoment(ctx, stop.FeaturedMoment.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			expected[m.ID] = m
		}
	}
	released, err := s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &previous.ID})
	if err != nil {
		t.Fatal(err)
	}
	working.RevisionID = &released.ID
	working.RevisionNumber = released.Number
	working.UpdatedAt = released.Snapshot.Tour.UpdatedAt
	if !reflect.DeepEqual(publicProjection(t, working), publicProjection(t, released.Snapshot.Tour)) {
		t.Error("published tour projection differs from the public working view")
	}
	for id, want := range expected {
		if !reflect.DeepEqual(publicProjection(t, want), publicProjection(t, released.Snapshot.Moments[id])) {
			t.Errorf("published story projection differs for %s", id)
		}
	}
	before := publicProjection(t, released)
	if err := s.SetMomentNarration(ctx, testdb.SeedMomentBurlingtonID, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceMomentTopics(ctx, testdb.SeedMomentBurlingtonID, nil); err != nil {
		t.Fatal(err)
	}
	again, err := s.GetTourRevision(ctx, working.ID, released.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, publicProjection(t, again)) {
		t.Fatal("source metadata edits changed an existing snapshot")
	}
}

func publicProjection(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	var normalize func(any) any
	normalize = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for k, item := range x {
				x[k] = normalize(item)
			}
		case []any:
			for i, item := range x {
				x[i] = normalize(item)
			}
		case string:
			if instant, err := time.Parse(time.RFC3339Nano, x); err == nil {
				return instant.UTC().Format(time.RFC3339Nano)
			}
		}
		return v
	}
	return normalize(decoded)
}
