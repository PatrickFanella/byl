package models

import (
	"strings"
	"testing"
	"time"
)

func TestPathInspectionRequiresActualRetainedPageAndExactQuote(t *testing.T) {
	ref := CorridorEvidenceRef{SegmentID: "trail", Purpose: "access", Mode: TravelModeWalk, EvidenceID: "retained", Reference: "p1", Quote: "Synthetic path access."}
	evidence := SourceEvidence{ID: "retained", Result: &EvidenceResult{Outcome: "inspected_page", FinalURL: "https://example.org/access", RetrievedAt: time.Unix(1, 0), HTTPStatus: 200, ContentSHA256: strings.Repeat("a", 64), Passages: []EvidencePassage{{Reference: "p1", Text: "Synthetic path access. Additional fixture context."}}}}
	if got, err := InspectPathEvidence(ref, &evidence); err != nil || got.Quote != ref.Quote || got.URL != evidence.Result.FinalURL {
		t.Fatalf("retained inspection %+v %v", got, err)
	}
	for _, edit := range []func(*SourceEvidence){
		func(e *SourceEvidence) { e.Result.Outcome = "catalog_only" },
		func(e *SourceEvidence) { e.Result.HTTPStatus = 403 },
		func(e *SourceEvidence) { e.Result.Passages[0].Text = "A different access claim." },
		func(e *SourceEvidence) { e.Result.ContentSHA256 = strings.Repeat("z", 64) },
	} {
		result := *evidence.Result
		result.Passages = append([]EvidencePassage(nil), evidence.Result.Passages...)
		copy := evidence
		copy.Result = &result
		edit(&copy)
		if _, err := InspectPathEvidence(ref, &copy); err == nil {
			t.Fatal("uninspected or mismatched source accepted")
		}
	}
}
