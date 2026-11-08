package models

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSignalReferencesRequireExplicitBoundedReview(t *testing.T) {
	fixture := func() CorridorIntent {
		c := corridorFixture(t)
		c.Segments[0].Geometry.Coordinates[0][1] = 41.797
		c.Segments[0].SignalReferences = map[TravelMode]SignalReference{TravelModeWalk: {Version: 1, AmbiguityReviewed: true, CompetingPaths: []ReferenceLine{}, StopAreas: []SignalStopArea{}, Source: c.Source}}
		return c
	}
	c := fixture()
	if f := c.Validate(); len(f) > 0 {
		t.Fatal(f)
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var restored CorridorIntent
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if f := restored.Validate(); len(f) > 0 {
		t.Fatal(f)
	}
	for name, change := range map[string]func(*CorridorSegment, *SignalReference){
		"missing review":                   func(_ *CorridorSegment, r *SignalReference) { r.AmbiguityReviewed = false },
		"missing explicit alternatives":    func(_ *CorridorSegment, r *SignalReference) { r.CompetingPaths = nil },
		"missing explicit stop exclusions": func(_ *CorridorSegment, r *SignalReference) { r.StopAreas = nil },
		"source missing":                   func(_ *CorridorSegment, r *SignalReference) { r.Source.Version = "" },
		"wrong version":                    func(_ *CorridorSegment, r *SignalReference) { r.Version = 2 },
		"malformed alternative": func(_ *CorridorSegment, r *SignalReference) {
			r.CompetingPaths = []ReferenceLine{{Type: "LineString", Coordinates: [][]float64{{-88, 41}, {-88, 41, 0}}}}
		},
		"remote exclusion": func(_ *CorridorSegment, r *SignalReference) {
			r.StopAreas = []SignalStopArea{{Latitude: 0, Longitude: 0, RadiusM: 40}}
		},
		"invalid radius": func(_ *CorridorSegment, r *SignalReference) {
			r.StopAreas = []SignalStopArea{{Latitude: 41.7956, Longitude: -88.0093, RadiusM: math.NaN()}}
		},
		"short segment": func(s *CorridorSegment, _ *SignalReference) {
			s.Geometry.Coordinates = [][]float64{{-88.0093, 41.7956}, {-88.0093, 41.79561}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := fixture()
			s := &c.Segments[0]
			r := s.SignalReferences[TravelModeWalk]
			change(s, &r)
			s.SignalReferences[TravelModeWalk] = r
			if len(c.Validate()) == 0 {
				t.Fatal("invalid signal reference accepted")
			}
		})
	}
	// Older reviewed routes remain valid, without becoming sampling eligible.
	if f := corridorFixture(t).Validate(); len(f) > 0 {
		t.Fatal(f)
	}
}
