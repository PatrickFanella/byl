package models

import (
	"math"
	"strings"
	"testing"
)

func TestRouteReportMinimalAndBoundedInput(t *testing.T) {
	minimal := RouteReportInput{TourID: "tour", RevisionID: "revision", Mode: TravelModeWalk, Reason: "blocked", Affected: ReportLocation{Kind: "entrance", StopID: "stop"}}
	minimal.Normalize()
	if f := minimal.Validate(); len(f) != 0 || minimal.Direction != "unknown" || minimal.Duration != "unknown" {
		t.Fatalf("minimal report %+v %+v", minimal, f)
	}
	for name, change := range map[string]func(*RouteReportInput){
		"improvement needs meaning": func(r *RouteReportInput) { r.Reason = "improvement" },
		"long notes":                func(r *RouteReportInput) { r.Details = strings.Repeat("x", 4001) },
		"credentials in source":     func(r *RouteReportInput) { r.SourceURL = "https://user:password@example.com" },
		"unrelated fields":          func(r *RouteReportInput) { r.Affected.FromStopID = "other" },
		"unknown duration":          func(r *RouteReportInput) { r.Duration = "forever-ish" },
		"non-finite annotation": func(r *RouteReportInput) {
			lat, lng := math.NaN(), 0.0
			r.Annotation = &ReportPoint{Lat: &lat, Lng: &lng}
		},
		"missing coordinates": func(r *RouteReportInput) { r.Annotation = &ReportPoint{} },
		"three-dimensional point": func(r *RouteReportInput) {
			r.Alternative = &ReportAlternative{Type: "LineString", Coordinates: [][]float64{{1, 2, 3}, {4, 5}}}
		},
		"unbounded alternative": func(r *RouteReportInput) {
			r.Alternative = &ReportAlternative{Type: "LineString", Coordinates: make([][]float64, 65)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := minimal
			change(&r)
			if f := r.Validate(); len(f) == 0 {
				t.Fatal("invalid report accepted")
			}
		})
	}
}
