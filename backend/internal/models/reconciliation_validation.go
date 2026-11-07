package models

import (
	"encoding/json"
	"math"
	"strings"
)

// Validate bounds private identity leads without granting their facts authority.
func (r ReconciliationRequest) Validate() Fields {
	f := Fields{}
	if r.Version < 1 {
		f["version"] = "must be a positive saved brief version"
	}
	if r.Item.Kind != CandidateKindPlace && r.Item.Kind != CandidateKindMoment {
		f["item.kind"] = "must be place or moment"
	}
	if strings.TrimSpace(r.Item.Title) == "" {
		f["item.title"] = "is required"
	}
	raw, err := json.Marshal(r.Item)
	if err != nil || len(raw) > 32768 || len(r.Item.Title) > 500 || len(r.Item.Origins) > 20 {
		f["item"] = "must be a bounded lead of at most 32768 bytes"
	}
	if (r.Item.Lat == nil) != (r.Item.Lng == nil) {
		f["item.lat"] = "provide both coordinates or neither"
	}
	if r.Item.Lat != nil && (math.IsNaN(*r.Item.Lat) || math.IsInf(*r.Item.Lat, 0) || *r.Item.Lat < -90 || *r.Item.Lat > 90) {
		f["item.lat"] = "must be a latitude"
	}
	if r.Item.Lng != nil && (math.IsNaN(*r.Item.Lng) || math.IsInf(*r.Item.Lng, 0) || *r.Item.Lng < -180 || *r.Item.Lng > 180) {
		f["item.lng"] = "must be a longitude"
	}
	if r.Item.Identity != nil {
		if len(r.Item.Identity.Aliases) > 20 || len(r.Item.Identity.HistoricalAddresses) > 20 {
			f["item.identity"] = "at most 20 aliases and historical addresses"
		}
	}
	return f
}

// Validate requires explicit, versioned curator decisions rather than automatic merges.
func (r IdentityDecisionRequest) Validate() Fields {
	f := Fields{}
	if raw, err := json.Marshal(r); err != nil || len(raw) > 32768 {
		f["decision"] = "must be at most 32768 encoded bytes"
	}
	if r.Version < 0 {
		f["version"] = "must be the last review version"
	}
	if strings.TrimSpace(r.Note) == "" || len(r.Note) > 4000 {
		f["note"] = "a review explanation of at most 4000 bytes is required"
	}
	if len(r.Aliases) > 20 || len(r.HistoricalAddresses) > 20 {
		f["aliases"] = "at most 20 aliases and historical addresses"
	}
	switch r.Outcome {
	case "existing_place", "existing_place_new_moment":
		if r.PlaceID == "" || r.MomentID != "" {
			f["place_id"] = "provide only an existing place"
		}
	case "existing_place_existing_moment":
		if r.PlaceID == "" || r.MomentID == "" {
			f["moment_id"] = "provide a moment belonging to the place"
		}
	case "needs_review", "rejected":
		if r.PlaceID != "" || r.MomentID != "" || len(r.Aliases) > 0 || len(r.HistoricalAddresses) > 0 {
			f["outcome"] = "unresolved or rejected leads cannot establish identity claims"
		}
	default:
		f["outcome"] = "must resolve an existing identity, request review, or reject"
	}
	return f
}
