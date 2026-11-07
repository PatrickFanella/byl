package httpapi

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"invisible-museum/backend/internal/models"
)

// Keep captured API fixtures usable by downstream decoders as revisions change.
func TestRevisionClientFixtures(t *testing.T) {
	decode := func(name string, target any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("..", "..", "api", "fixtures", "tour-revisions-v1", name))
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			t.Fatal(err)
		}
	}

	var first, second models.TourRevision
	var progress models.RevisionProgress
	decode("revision-1.json", &first)
	decode("revision-2.json", &second)
	decode("progress-1.json", &progress)
	if first.TourID != second.TourID || first.ID == second.ID || second.Number <= first.Number || progress.RevisionID != first.ID {
		t.Fatal("fixtures do not describe successive releases and an original trip")
	}
	if len(first.Snapshot.Tour.Stops) != 2 || len(second.Snapshot.Tour.Stops) != 2 || len(progress.VisitedStopIDs) != 1 {
		t.Fatal("fixture itinerary or visit is missing")
	}
	oldStop, movedStop := first.Snapshot.Tour.Stops[0], second.Snapshot.Tour.Stops[1]
	if oldStop.ID != movedStop.ID || oldStop.Position == movedStop.Position || progress.VisitedStopIDs[0] != oldStop.ID {
		t.Fatal("stable stop visit was replaced by a positional visit")
	}
	if len(first.Snapshot.Moments) == 0 || first.Snapshot.Routes[models.TravelModeWalk].Geometry == nil {
		t.Fatal("fixture lacks reviewed text or geometry")
	}
}
