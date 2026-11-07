package itinerary

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
)

func fixture(id, title string, lat, lng float64) models.ItineraryMaterial {
	return models.ItineraryMaterial{RecordID: "record-" + id, ReviewVersion: 1, PlaceID: id, PlaceName: "Place " + id, CityID: "city", Latitude: lat, Longitude: lng, MomentID: "moment-" + id, MomentTitle: title, Narrative: title + " is described in this retained synthetic historical account.", EvidenceKind: "curated_record", ContentSHA256: strings.Repeat("a", 64), SourceURLs: []string{"https://example.org/fixture"}, EvidenceIDs: []string{}}
}
func material(items ...models.ItineraryMaterial) models.ItineraryMaterials {
	return models.ItineraryMaterials{Version: 1, Items: items, Gaps: []models.ItineraryGap{}}
}
func hasGap(p models.ItineraryProposal, code string) bool {
	return slices.ContainsFunc(p.ResearchGaps, func(g models.ItineraryGap) bool { return g.Code == code })
}
func TestCompactSelectionExcludesOutlyingThematicStopWithoutInventingTravel(t *testing.T) {
	minutes := 30
	in := models.RouteBriefInput{Topic: "Rail workers", Area: "Synthetic downtown", TripShape: "loop", DurationMinutes: &minutes, Modes: []models.TravelMode{models.TravelModeWalk}}
	input := material(fixture("a", "Rail workers organize", 41.8, -88), fixture("b", "Rail workers depot", 41.801, -88), fixture("c", "Rail workers outpost", 42.2, -88))
	p := Assemble(in, input, time.Unix(1, 0))
	if p.Status != "draft" || len(p.Stops) != 2 || len(p.Excluded) != 1 || p.Excluded[0].Material.PlaceID != "c" || p.Excluded[0].Reason != "geography_or_duration_requires_review" {
		t.Fatalf("selection %+v", p)
	}
	if p.TravelMinutes != nil || p.TravelState != "not_computed" || p.PlannedDwellMinutes != 6 || p.PlannedNarrationMinutes != 2 || !reflect.DeepEqual(p.TravelSequence, []int{0, 1, 0}) {
		t.Fatalf("travel/dwell conflated %+v", p)
	}
	if !strings.Contains(p.Stops[0].WhatToNotice, "research question") || p.Stops[0].Transition == "" || p.Introduction == "" {
		t.Fatal("missing honest proposal narrative")
	}
	if !reflect.DeepEqual(p, Assemble(in, input, time.Unix(1, 0))) {
		t.Fatal("identical captured inputs changed selection")
	}
}
func TestSingleHaymarketLocationKeepsHistoryAndNeedsResearch(t *testing.T) {
	a := fixture("haymarket", "Haymarket workers", 41.88, -87.64)
	b := a
	b.MomentID = "another-event"
	b.MomentTitle = "Haymarket meeting"
	p := Assemble(models.RouteBriefInput{Topic: "Haymarket", Area: "Chicago"}, material(a, b), time.Unix(1, 0))
	if p.Status != "needs_research" || len(p.Stops) != 1 || !hasGap(p, "insufficient_supported_locations") || p.Stops[0].Material.Narrative == "" {
		t.Fatalf("one place padded a tour %+v", p)
	}
}
func TestReusedPlaceSelectsDifferentFeaturedMomentForDifferentThemes(t *testing.T) {
	rail := fixture("shared", "Rail workers strike", 41.8, -88)
	music := rail
	music.MomentID = "music"
	music.MomentTitle = "Music performance"
	music.Narrative = "Music performance is documented in the retained source."
	other := fixture("other", "Rail workers and music", 41.801, -88)
	for _, tc := range []struct{ topic, id string }{{"rail", rail.MomentID}, {"music", music.MomentID}} {
		p := Assemble(models.RouteBriefInput{Topic: tc.topic, Area: "Fixture", StartPreference: rail.PlaceName}, material(rail, music, other), time.Unix(1, 0))
		if len(p.Stops) != 2 || p.Stops[0].Material.MomentID != tc.id {
			t.Fatalf("theme %s wrong featured history %+v", tc.topic, p)
		}
	}
}
func TestTripShapesReturnTravelWithoutDuplicateStories(t *testing.T) {
	input := material(fixture("a", "Rail depot", 41.8, -88), fixture("b", "Rail yard", 41.801, -88), fixture("c", "Rail crossing", 41.802, -88))
	for _, tc := range []struct {
		shape    string
		sequence []int
	}{{"point_to_point", []int{0, 1, 2}}, {"loop", []int{0, 1, 2, 0}}, {"out_and_back", []int{0, 1, 2, 1, 0}}} {
		p := Assemble(models.RouteBriefInput{Topic: "Rail", Area: "Fixture", TripShape: tc.shape, StartPreference: "Place a"}, input, time.Unix(1, 0))
		if len(p.Stops) != 3 || !reflect.DeepEqual(p.TravelSequence, tc.sequence) {
			t.Fatalf("shape %s %+v", tc.shape, p)
		}
	}
}
func TestUnresolvedEndpointsAndImpracticalGeographyRemainGaps(t *testing.T) {
	input := material(fixture("a", "Rail depot", 41.8, -88), fixture("b", "Rail outpost", 43, -88))
	p := Assemble(models.RouteBriefInput{Topic: "Rail", Area: "Fixture", StartPreference: "Unknown gate"}, input, time.Unix(1, 0))
	if p.Status != "needs_research" || !hasGap(p, "start_unresolved") || !hasGap(p, "insufficient_supported_locations") {
		t.Fatalf("fabricated feasible route %+v", p)
	}
}

func TestRequestedEndKeepsAPlaceWithinTheEightStopBound(t *testing.T) {
	input := material()
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("%02d", i)
		input.Items = append(input.Items, fixture(id, "Rail workers", 41.8+float64(i)*0.0001, -88))
	}
	p := Assemble(models.RouteBriefInput{Topic: "Rail", Area: "Fixture", TripShape: "point_to_point", StartPreference: "Place 00", EndPreference: "Place 09"}, input, time.Unix(1, 0))
	if p.Status != "draft" || len(p.Stops) != 8 || p.Stops[7].Material.PlaceID != "09" || len(p.TravelSequence) != 8 {
		t.Fatalf("end overran/was excluded from bounded proposal %+v", p)
	}
	for _, e := range p.Excluded {
		if e.Reason != "proposal_stop_limit" {
			t.Fatalf("incorrect bounded-stop exclusion %+v", e)
		}
	}
}
func TestReturnTripCanResolveItsStartFromAnExplicitEnd(t *testing.T) {
	input := material(fixture("a", "Rail depot", 41.8, -88), fixture("b", "Rail yard", 41.801, -88))
	p := Assemble(models.RouteBriefInput{Topic: "Rail", Area: "Fixture", TripShape: "out_and_back", EndPreference: "Place b"}, input, time.Unix(1, 0))
	if p.Status != "draft" || p.Stops[0].Material.PlaceID != "b" {
		t.Fatalf("ignored return endpoint %+v", p)
	}
}
