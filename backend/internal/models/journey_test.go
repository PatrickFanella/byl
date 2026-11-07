package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func journeyTour() *TourDetail {
	return &TourDetail{Stops: []TourStop{{ID: "first", Position: 1, FeaturedMoment: &MomentSummary{}}, {ID: "last", Position: 2, FeaturedMoment: &MomentSummary{}}}}
}
func journeyRoute() *TourRoute {
	return &TourRoute{Mode: TravelModeWalk, Status: RouteStatusReady, Provider: "valhalla", DistanceM: 1400, DurationS: 1200, Geometry: &LineString{Type: "LineString", Coordinates: [][2]float64{{-88, 41}, {-88.001, 41}}}, Quality: &PathQualitySummary{Version: 1, Status: "pass", InputCurrent: true}}
}
func TestJourneySeparatesSelectedTravelStopsAndReturn(t *testing.T) {
	tour, route := journeyTour(), journeyRoute()
	route.Legs = []RouteLeg{{FromPosition: 1, ToPosition: 2, DistanceM: 1000, DurationS: 900}, {FromPosition: 2, ToPosition: 1, Return: true, DistanceM: 400, DurationS: 300}}
	j := DescribeJourney(tour, route)
	if !j.Usable || j.State != "usable" || *j.TravelDistanceM != 1400 || *j.TravelDurationS != 1200 || j.DwellDurationS != 360 || j.NarrationDurationS != 240 || *j.ItineraryDurationS != 1800 || *j.RoundTripDurationS != 1800 || *j.Return.DurationS != 300 || *j.Return.DistanceM != 400 || j.Return.Status != "included" {
		t.Fatalf("wrong trip estimates %+v", j)
	}
	// Legacy estimated_minutes remains travel-only. The projection changes no inputs.
	fingerprint := route.RoutingFingerprint()
	route.Journey = j
	if route.RoutingFingerprint() != fingerprint || *route.Summary().EstimatedMinutes != 20 {
		t.Fatal("explanation changed legacy estimate or routing identity")
	}
	route.Legs = nil
	j = DescribeJourney(tour, route)
	if j.Return.Status != "unplanned" || j.Return.DurationS != nil || j.RoundTripDurationS != nil || *j.ItineraryDurationS != 1800 {
		t.Fatal("invented or double-counted a return")
	}
}
func TestJourneyDoesNotTurnAnIntermediateLegIntoAReturn(t *testing.T) {
	route := journeyRoute()
	route.Legs = []RouteLeg{{FromPosition: 1, ToPosition: 2, Return: true, DistanceM: 400, DurationS: 300}}
	j := DescribeJourney(journeyTour(), route)
	if j.Return.Status != "unplanned" || j.RoundTripDurationS != nil {
		t.Fatal("intermediate travel became a round trip")
	}
}

func TestJourneyReadinessAndUnavailableModes(t *testing.T) {
	for _, tc := range []struct {
		name, status, quality string
		current               bool
		geometry              bool
		state                 string
		metrics               bool
	}{
		{"passing", "ready", "pass", true, true, "usable", true},
		{"unreviewed", "ready", "", false, true, "unreviewed", true},
		{"blocked", "ready", "blocked", true, true, "blocked", true},
		{"partial proof", "ready", "needs_research", true, true, "needs_research", true},
		{"stale", "ready", "pass", false, true, "stale", true},
		{"pending", "pending", "pass", true, false, "pending", false},
		{"failed", "failed", "blocked", true, false, "failed", false},
		{"missing geometry", "ready", "pass", true, false, "unavailable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			route := journeyRoute()
			route.Status = RouteStatus(tc.status)
			if tc.quality == "" {
				route.Quality = nil
			} else {
				route.Quality = &PathQualitySummary{Version: 1, Status: tc.quality, InputCurrent: tc.current}
			}
			if !tc.geometry {
				route.Geometry = nil
			}
			j := DescribeJourney(journeyTour(), route)
			if j.State != tc.state || j.Usable != (tc.state == "usable") || (j.TravelDurationS != nil) != tc.metrics || (j.ItineraryDurationS != nil) != tc.metrics {
				t.Fatalf("unexpected readiness %+v", j)
			}
		})
	}
	j := DescribeJourney(journeyTour(), nil)
	if j.State != "missing" || j.Usable || j.ItineraryDurationS != nil {
		t.Fatal("missing mode gained a computed estimate")
	}
}
func TestJourneySegmentsUseOnlyApprovedPublicFields(t *testing.T) {
	tour, route := journeyTour(), journeyRoute()
	source := CorridorSource{URL: "https://example.org/map", Title: "Trail reference", Publisher: "Operator", Version: "v1", License: "ODbL", LicenseURL: "https://example.org/license", RetrievedOn: "2026-11-07", Note: "private marker not projected"}
	tour.Corridor = &ApprovedCorridor{Intent: &CorridorIntent{Source: source, Segments: []CorridorSegment{
		{ID: "trail", Kind: "trail", Modes: []TravelMode{TravelModeWalk, TravelModeBike}, Surface: "asphalt", Network: &CorridorNetwork{FromNodeID: 123}, Access: []CorridorAccess{{Mode: TravelModeWalk, Status: "allowed", Notes: "private retained quote"}}},
		{ID: "crossing", Kind: "road_connector", Modes: []TravelMode{TravelModeWalk}, Source: &source},
	}, Routes: []ModeCorridor{{Mode: TravelModeWalk, Path: []CorridorTraversal{{SegmentID: "trail"}, {SegmentID: "crossing", Reverse: true}, {SegmentID: "trail", Reverse: true}}}}}}
	route.Quality.Issues = []PathQualityIssue{{Code: "review_required", Detail: "private synthetic payload", SegmentID: "trail"}}
	j := DescribeJourney(tour, route)
	if len(j.Segments) != 3 || j.Segments[1].Kind != "road_connector" || !j.Segments[2].Reverse || *j.Segments[0].Surface != "asphalt" || j.Segments[1].Surface != nil || j.Segments[0].Access != "allowed" || j.Segments[1].Access != "unknown" || j.Segments[0].Source.URL != source.URL {
		t.Fatalf("lost approved segment semantics %+v", j)
	}
	raw, _ := json.Marshal(j)
	for _, forbidden := range []string{"private", "from_node_id", "inspections", "report_id", "attribution", "quote"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("private field leaked: %s", raw)
		}
	}
}

func TestJourneyDoesNotQualifyAnUnsupportedQualityVersion(t *testing.T) {
	route := journeyRoute()
	route.Quality.Version = 2
	j := DescribeJourney(journeyTour(), route)
	if j.State != "unreviewed" || j.Quality != "unreviewed" || j.Usable {
		t.Fatal("unsupported quality qualified a path")
	}
}
