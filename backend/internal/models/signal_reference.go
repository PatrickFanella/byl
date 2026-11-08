package models

import (
	"fmt"
	"math"
	"slices"

	"invisible-museum/backend/internal/geo"
)

// SignalReference is an explicit, independently sourced ambiguity review for
// one mode on the segment's canonical geometry. Missing means unsupported.
// Existing corridor proposal review and immutable publication retain this data.
type SignalReference struct {
	Version           int              `json:"version"`
	AmbiguityReviewed bool             `json:"ambiguity_reviewed"`
	CompetingPaths    []ReferenceLine  `json:"competing_paths"`
	StopAreas         []SignalStopArea `json:"stop_areas"`
	Source            CorridorSource   `json:"source"`
}

type SignalStopArea struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	RadiusM   float64 `json:"radius_m"`
}

func (s CorridorSegment) validateSignalReferences() error {
	if len(s.SignalReferences) == 0 {
		return nil
	}
	if len(s.SignalReferences) > 2 || !signalLineValid(s.Geometry, s.Geometry) {
		return fmt.Errorf("signal references need a local 50–5000 m segment with at most 512 vertices")
	}
	length := 0.0
	for i := 1; i < len(s.Geometry.Coordinates); i++ {
		a, b := s.Geometry.Coordinates[i-1], s.Geometry.Coordinates[i]
		length += geo.DistanceMeters(a[1], a[0], b[1], b[0])
	}
	if length < 50 || length > 5000 {
		return fmt.Errorf("signal reference length must be 50–5000 m")
	}
	for mode, ref := range s.SignalReferences {
		if !mode.Valid() || !slices.Contains(s.Modes, mode) || ref.Version != 1 || !ref.AmbiguityReviewed || !ref.Source.valid() || ref.CompetingPaths == nil || ref.StopAreas == nil || len(ref.CompetingPaths) > 8 || len(ref.StopAreas) > 32 {
			return fmt.Errorf("signal references require an allowed mode, version 1, explicit ambiguity review, source and bounded explicit path/stop arrays")
		}
		for _, line := range ref.CompetingPaths {
			if !signalLineValid(line, s.Geometry) {
				return fmt.Errorf("competing paths must be local bounded LineStrings")
			}
		}
		origin := s.Geometry.Coordinates[0]
		for _, area := range ref.StopAreas {
			if !signalPointValid(area.Longitude, area.Latitude, origin) || math.IsNaN(area.RadiusM) || math.IsInf(area.RadiusM, 0) || area.RadiusM < 0 || area.RadiusM > 500 {
				return fmt.Errorf("stop exclusions need local finite coordinates and a radius of 0–500 m")
			}
		}
	}
	return nil
}

func signalLineValid(line, segment ReferenceLine) bool {
	if len(segment.Coordinates) == 0 || len(segment.Coordinates[0]) != 2 || line.Type != "LineString" || len(line.Coordinates) < 2 || len(line.Coordinates) > 512 {
		return false
	}
	for _, p := range line.Coordinates {
		if len(p) != 2 || !signalPointValid(p[0], p[1], segment.Coordinates[0]) {
			return false
		}
	}
	return true
}

func signalPointValid(lng, lat float64, origin []float64) bool {
	return validReportPoint(lat, lng) && math.Abs(lat) <= 85 && math.Abs(lat-origin[1]) <= .25 && math.Abs(lng-origin[0]) <= .25
}
