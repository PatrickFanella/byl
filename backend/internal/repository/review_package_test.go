package repository

import (
	"encoding/json"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestReviewPackageChangesComparePublicFieldsSemantically(t *testing.T) {
	current := json.RawMessage(`{"title":"Old title","body":"<unchanged>","latitude":1}`)
	proposed := json.RawMessage(`{"title":"New title","body":"\u003cunchanged\u003e","latitude":1.0,"notes":"private research metadata","generated_drafts":[]}`)
	changes := packageChanges(current, proposed)
	if len(changes) != 1 || changes[0].Field != "title" || string(changes[0].Before) != `"Old title"` || string(changes[0].After) != `"New title"` {
		t.Fatalf("wrong public field differences %+v", changes)
	}
}
func TestReviewPackageOutAndBackNeedsDirectedSymmetry(t *testing.T) {
	tour := &models.TourDetail{Corridor: &models.ApprovedCorridor{Intent: &models.CorridorIntent{Routes: []models.ModeCorridor{{Mode: models.TravelModeWalk, Path: []models.CorridorTraversal{{SegmentID: "a"}, {SegmentID: "b"}, {SegmentID: "b", Reverse: true}, {SegmentID: "a", Reverse: true}}}}}}}
	if !outAndBackReference(tour, models.TravelModeWalk) || outAndBackReference(tour, models.TravelModeBike) {
		t.Fatal("mode or return direction lost")
	}
	tour.Corridor.Intent.Routes[0].Path[3].Reverse = false
	if outAndBackReference(tour, models.TravelModeWalk) {
		t.Fatal("one-way traversal became a retraced return")
	}
}

func TestReviewPackageHeroDifferenceRetainsPreviousIdentity(t *testing.T) {
	changes := packageChanges(json.RawMessage(`{"hero_media":{"id":"previous-photo"}}`), json.RawMessage(`{"hero_media_id":"replacement-photo"}`))
	if len(changes) != 1 || string(changes[0].Before) != `"previous-photo"` || string(changes[0].After) != `"replacement-photo"` {
		t.Fatal("previous picture identity lost", changes)
	}
}
