package models

import (
	"math"
	"testing"
)

func TestArrivalEvidenceValidation(t *testing.T) {
	lat, lng := 41.8, -88.01
	base := ArrivalProposalRequest{Point: ArrivalPoint{Name: "Fixture entrance", Kind: "entrance", Latitude: &lat, Longitude: &lng, Access: []ArrivalAccess{{Mode: TravelModeWalk, Status: "unknown", Surface: "unknown"}, {Mode: TravelModeBike, Status: "unknown", Surface: "unknown"}}}}
	if f := base.Validate(); len(f) > 0 {
		t.Fatalf("explicit unknown is valid %+v", f)
	}
	for _, tc := range []struct {
		name string
		edit func(*ArrivalProposalRequest)
	}{
		{"missing coordinate", func(r *ArrivalProposalRequest) { r.Point.Latitude = nil }},
		{"nonfinite", func(r *ArrivalProposalRequest) { v := math.NaN(); r.Point.Longitude = &v }},
		{"missing bike", func(r *ArrivalProposalRequest) { r.Point.Access = r.Point.Access[:1] }},
		{"duplicate mode", func(r *ArrivalProposalRequest) { r.Point.Access[1].Mode = TravelModeWalk }},
		{"unsourced allowed", func(r *ArrivalProposalRequest) { r.Point.Access[0].Status = "allowed" }},
		{"unsourced surface", func(r *ArrivalProposalRequest) { r.Point.Access[0].Surface = "paved" }},
		{"invented eligibility", func(r *ArrivalProposalRequest) { r.Point.Access[0].Status = "accessible" }},
		{"credentials", func(r *ArrivalProposalRequest) {
			a := &r.Point.Access[0]
			a.SourceURL = "https://user:secret@example.org/"
			a.SourceTitle = "Fixture"
			a.RetrievedAt = "2026-01-01"
		}},
		{"future date", func(r *ArrivalProposalRequest) {
			a := &r.Point.Access[0]
			a.SourceURL = "https://example.org/"
			a.SourceTitle = "Fixture"
			a.RetrievedAt = "2999-01-01"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.Point.Access = append([]ArrivalAccess{}, base.Point.Access...)
			tc.edit(&r)
			if len(r.Validate()) == 0 {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}
