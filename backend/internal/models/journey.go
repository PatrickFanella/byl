package models

import "strings"

// RouteJourney explains a selected mode without exposing retained private proof.
// Version 1 is a response projection; it is not part of the routing fingerprint.
type RouteJourney struct {
	Version            int              `json:"version"`
	State              string           `json:"state"`
	Computation        string           `json:"computation"`
	Quality            string           `json:"quality"`
	Usable             bool             `json:"usable"`
	IssueCodes         []string         `json:"issue_codes"`
	TravelDistanceM    *float64         `json:"travel_distance_m"`
	TravelDurationS    *int             `json:"travel_duration_s"`
	DwellDurationS     int              `json:"dwell_duration_s"`
	NarrationDurationS int              `json:"narration_duration_s"`
	ItineraryDurationS *int             `json:"itinerary_duration_s"`
	RoundTripDurationS *int             `json:"round_trip_duration_s"`
	Return             RouteReturn      `json:"return_plan"`
	Segments           []JourneySegment `json:"segments"`
	Assumptions        []string         `json:"assumptions"`
}

type RouteReturn struct {
	Status    string   `json:"status"`
	DistanceM *float64 `json:"distance_m"`
	DurationS *int     `json:"duration_s"`
}

// JourneySource is an allowlist of approved public reference metadata.
type JourneySource struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Publisher   string `json:"publisher"`
	Version     string `json:"version"`
	RetrievedOn string `json:"retrieved_on"`
	License     string `json:"license"`
	LicenseURL  string `json:"license_url"`
}

// JourneySegment describes an ordered reference traversal, not inferred provider
// edge attribution. Its length/time are never substituted for computed metrics.
type JourneySegment struct {
	ID      string        `json:"id"`
	Kind    string        `json:"kind"`
	Reverse bool          `json:"reverse"`
	Basis   string        `json:"basis"`
	Surface *string       `json:"surface"`
	Modes   []TravelMode  `json:"modes"`
	Access  string        `json:"access"`
	Source  JourneySource `json:"source"`
}

// DescribeJourney uses only this itinerary's selected route and approved public
// intent. A retained revision must pass its own frozen tour, never live joins.
func DescribeJourney(tour *TourDetail, route *TourRoute) *RouteJourney {
	j := &RouteJourney{Version: 1, State: "missing", Computation: "missing", Quality: "unreviewed", IssueCodes: []string{}, Segments: []JourneySegment{}, Return: RouteReturn{Status: "unplanned"}, Assumptions: []string{
		"Travel metrics describe the selected computed geometry, not field timing or proof of legal access.",
		"Plan 180 seconds of dwell per story stop, plus 120 seconds of narration per featured story; these are assumptions, not measured audio lengths.",
		"An unplanned return has no distance or time estimate. Itinerary totals exclude an unplanned return.",
	}}
	j.DwellDurationS = 180 * len(tour.Stops)
	for _, s := range tour.Stops {
		if s.FeaturedMoment != nil {
			j.NarrationDurationS += 120
		}
	}
	if route == nil {
		return j
	}
	if tour.Transit != nil && route.Mode == TravelModeWalk {
		j.State, j.Computation, j.Quality = "requires_transit_plan", "scheduled", "requires_schedule"
		return j
	}
	j.Computation = string(route.Status)
	j.State = j.Computation
	if q := route.Quality; q != nil && q.Version == 1 {
		j.Quality = q.Status
		seen := map[string]bool{}
		for _, issue := range q.Issues {
			if !seen[issue.Code] {
				j.IssueCodes = append(j.IssueCodes, issue.Code)
				seen[issue.Code] = true
			}
		}
	}
	if route.Status == RouteStatusReady {
		j.State = "unreviewed"
		if q := route.Quality; q != nil && q.Version == 1 {
			j.State = q.Status
			if !q.InputCurrent {
				j.State = "stale"
			} else if q.Status == "pass" {
				j.State = "usable"
				j.Usable = true
			}
		}
		if route.Geometry != nil && route.Geometry.Type == "LineString" && len(route.Geometry.Coordinates) >= 2 && route.DistanceM >= 0 && route.DurationS >= 0 {
			distance, duration := route.DistanceM, route.DurationS
			j.TravelDistanceM, j.TravelDurationS = &distance, &duration
			total := duration + j.DwellDurationS + j.NarrationDurationS
			j.ItineraryDurationS = &total
			if len(route.Legs) > 0 {
				leg := route.Legs[len(route.Legs)-1]
				if leg.Return && leg.FromPosition == len(tour.Stops) && leg.ToPosition == 1 && leg.DistanceM >= 0 && leg.DurationS >= 0 && leg.DistanceM <= route.DistanceM && leg.DurationS <= route.DurationS {
					j.Return = RouteReturn{Status: "included", DistanceM: &leg.DistanceM, DurationS: &leg.DurationS}
					j.RoundTripDurationS = &total
				}
			}
		} else {
			j.State = "unavailable"
			j.Usable = false
		}
	}
	if tour.Corridor != nil && tour.Corridor.Intent != nil {
		intent := tour.Corridor.Intent
		byID := map[string]CorridorSegment{}
		for _, segment := range intent.Segments {
			byID[segment.ID] = segment
		}
		for _, selection := range intent.Routes {
			if selection.Mode == route.Mode {
				for _, traversal := range selection.Path {
					segment, ok := byID[traversal.SegmentID]
					if !ok {
						continue
					}
					source := intent.Source
					if segment.Source != nil {
						source = *segment.Source
					}
					item := JourneySegment{ID: segment.ID, Kind: segment.Kind, Reverse: traversal.Reverse, Basis: "approved_reference", Modes: segment.Modes, Access: "unknown", Source: JourneySource{URL: source.URL, Title: source.Title, Publisher: source.Publisher, Version: source.Version, RetrievedOn: source.RetrievedOn, License: source.License, LicenseURL: source.LicenseURL}}
					if surface := strings.TrimSpace(segment.Surface); surface != "" {
						item.Surface = &surface
					}
					for _, access := range segment.Access {
						if access.Mode == route.Mode {
							item.Access = access.Status
						}
					}
					j.Segments = append(j.Segments, item)
				}
			}
		}
	}
	return j
}
