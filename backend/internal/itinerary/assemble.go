// Package itinerary assembles private, source-linked historical proposals.
package itinerary

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

type scored struct {
	material models.ItineraryMaterial
	score    int
	terms    []string
}

// Assemble uses retained text and a deliberately optimistic geography screen.
// Passing that screen is never an estimate of actual route distance or travel time.
func Assemble(in models.RouteBriefInput, material models.ItineraryMaterials, now time.Time) models.ItineraryProposal {
	in.Normalize()
	p := models.ItineraryProposal{Version: 1, Status: "draft", CreatedAt: now, Title: in.Topic + " — " + in.Area, RequestedShape: in.TripShape, TripShape: in.TripShape, Modes: in.Modes, TargetDurationMinutes: in.DurationMinutes, TravelState: "not_computed", Stops: []models.ItineraryStop{}, TravelSequence: []int{}, Excluded: []models.ItineraryExclusion{}, ResearchGaps: slices.Clone(material.Gaps), Assumptions: []string{"Historical paragraphs are reused from retained material; their inclusion is not a new factual review.", "What to notice is a curator research question, not verified current viewing guidance.", "Plan three minutes of dwell per stop, plus narration at 130 words per minute; actual travel and access still require computation and review."}}
	if p.ResearchGaps == nil {
		p.ResearchGaps = []models.ItineraryGap{}
	}
	gap := func(code, detail string) {
		p.Status = "needs_research"
		p.ResearchGaps = append(p.ResearchGaps, models.ItineraryGap{Code: code, Detail: detail})
	}
	if p.TripShape == "flexible" {
		p.TripShape = "point_to_point"
		p.Assumptions = append(p.Assumptions, "No trip shape was chosen; this proposal uses point-to-point travel for review.")
	}
	budget := 60
	if in.DurationMinutes != nil {
		budget = *in.DurationMinutes
	} else {
		p.Assumptions = append(p.Assumptions, "No duration was supplied; the geography screen uses a provisional 60-minute planning budget.")
	}
	terms := topicTerms(in.Topic)
	byPlace := map[string]scored{}
	for _, m := range material.Items {
		score, matched := relevance(m, in, terms)
		reason := ""
		if (m.EvidenceKind != "curated_record" && m.EvidenceKind != "evidence_linked_draft") || m.PlaceID == "" || m.Narrative == "" || len(m.SourceURLs) == 0 || len(m.ContentSHA256) != 64 || !finite(m.Latitude, m.Longitude) {
			reason = "missing_supported_location"
		} else if score == 0 {
			reason = "not_thematic"
		}
		if reason != "" {
			p.Excluded = append(p.Excluded, models.ItineraryExclusion{Material: m, Reason: reason})
			continue
		}
		s := scored{m, score, matched}
		old, exists := byPlace[m.PlaceID]
		if !exists || score > old.score || score == old.score && identity(m) < identity(old.material) {
			if exists {
				p.Excluded = append(p.Excluded, models.ItineraryExclusion{Material: old.material, Reason: "another_moment_selected_at_place"})
			}
			byPlace[m.PlaceID] = s
		} else {
			p.Excluded = append(p.Excluded, models.ItineraryExclusion{Material: m, Reason: "another_moment_selected_at_place"})
		}
	}
	choices := []scored{}
	for _, s := range byPlace {
		choices = append(choices, s)
	}
	sort.Slice(choices, func(i, j int) bool { return identity(choices[i].material) < identity(choices[j].material) })
	start, end := -1, -1
	for i, s := range choices {
		if sameName(in.StartPreference, s.material.PlaceName) {
			if start >= 0 {
				gap("ambiguous_start", "The requested start matches several researched places.")
			}
			start = i
		}
		if sameName(in.EndPreference, s.material.PlaceName) {
			if end >= 0 {
				gap("ambiguous_end", "The requested end matches several researched places.")
			}
			end = i
		}
	}
	if strings.TrimSpace(in.StartPreference) != "" && start < 0 {
		gap("start_unresolved", "The requested start does not resolve to a supported place; no endpoint was invented.")
	}
	if strings.TrimSpace(in.EndPreference) != "" && end < 0 {
		gap("end_unresolved", "The requested end does not resolve to a supported place; no endpoint was invented.")
	}
	if p.TripShape != "point_to_point" && end >= 0 && start >= 0 && end != start {
		gap("endpoint_shape_conflict", "A return trip must end at its start; review the requested endpoints.")
	}
	speed := 500.0
	if slices.Contains(in.Modes, models.TravelModeWalk) {
		speed = 100
	}
	// Compare possible compact groups; an isolated high-scoring hit must not displace
	// a supported downtown group. Stable identities make identical inputs repeatable.
	best := []int{}
	bestScore := -1
	for anchor := range choices {
		if start >= 0 && anchor != start {
			continue
		}
		if p.TripShape == "point_to_point" && end >= 0 && anchor == end && start != end && len(choices) > 1 {
			continue
		}
		if p.TripShape != "point_to_point" && end >= 0 && start < 0 && anchor != end {
			continue
		}
		route := []int{anchor}
		used := map[int]bool{anchor: true}
		limit := 8
		if p.TripShape == "point_to_point" && end >= 0 && end != anchor {
			limit = 7
		}
		for len(route) < limit {
			next := -1
			nearest := math.Inf(1)
			for i, s := range choices {
				if used[i] || i == end && p.TripShape == "point_to_point" {
					continue
				}
				proposed := append(slices.Clone(route), i)
				screen := proposed
				if p.TripShape == "point_to_point" && end >= 0 && !used[end] {
					screen = append(slices.Clone(proposed), end)
				}
				if !fits(screen, choices, p.TripShape, budget, speed) {
					continue
				}
				d := distance(choices[route[len(route)-1]].material, s.material)
				if d < nearest {
					next, nearest = i, d
				}
			}
			if next < 0 {
				break
			}
			route = append(route, next)
			used[next] = true
		}
		if end >= 0 && p.TripShape == "point_to_point" && !used[end] && fits(append(slices.Clone(route), end), choices, p.TripShape, budget, speed) {
			route = append(route, end)
		}
		score := 0
		for _, i := range route {
			score += choices[i].score
		}
		if !fits(route, choices, p.TripShape, budget, speed) {
			continue
		}
		if len(route) > len(best) || len(route) == len(best) && score > bestScore {
			best, bestScore = route, score
		}
	}
	selected := map[int]bool{}
	for _, i := range best {
		selected[i] = true
		m := choices[i].material
		narration := narrationMinutes(m.Narrative)
		stop := models.ItineraryStop{Material: m, ThemeReason: "Matched requested theme/event terms: " + strings.Join(choices[i].terms, ", "), WhatToNotice: fmt.Sprintf("Curator research question: what traces of “%s” can be observed at %s today? Confirm visible features before writing viewing guidance.", m.MomentTitle, m.PlaceName), PlannedDwellMinutes: 3, PlannedNarrationMinutes: narration}
		p.Stops = append(p.Stops, stop)
		p.PlannedDwellMinutes += 3
		p.PlannedNarrationMinutes += narration
	}
	for i, s := range choices {
		if !selected[i] {
			reason := "geography_or_duration_requires_review"
			if len(best) >= 8 {
				reason = "proposal_stop_limit"
			}
			p.Excluded = append(p.Excluded, models.ItineraryExclusion{Material: s.material, Reason: reason})
		}
	}
	for i := range p.Stops {
		p.TravelSequence = append(p.TravelSequence, i)
		if i+1 < len(p.Stops) {
			next := p.Stops[i+1].Material
			p.Stops[i].Transition = fmt.Sprintf("Continue the theme with “%s” at %s. The connecting path still needs review.", next.MomentTitle, next.PlaceName)
		}
	}
	if len(p.Stops) > 1 {
		if p.TripShape == "loop" {
			p.TravelSequence = append(p.TravelSequence, 0)
		}
		if p.TripShape == "out_and_back" {
			for i := len(p.Stops) - 2; i >= 0; i-- {
				p.TravelSequence = append(p.TravelSequence, i)
			}
		}
	}
	if len(p.Stops) < 2 {
		gap("insufficient_supported_locations", "At least two distinct source-supported locations are needed for an itinerary. Keep the supported single-site history and research additional locations; do not pad the tour.")
	}
	if end >= 0 && len(best) > 0 {
		actual := best[len(best)-1]
		if p.TripShape != "point_to_point" {
			actual = best[0]
		}
		if actual != end {
			gap("requested_end_impractical", "The requested end does not fit the selected trip shape and provisional time screen.")
		}
	}
	p.Introduction = fmt.Sprintf("Explore %s in %s through %d researched locations and their selected historical accounts. This is a private proposal; current viewing guidance, connecting paths and publication still require review.", in.Topic, in.Area, len(p.Stops))
	return p
}

func identity(m models.ItineraryMaterial) string {
	return m.PlaceID + ":" + m.MomentID + ":" + m.CandidateID
}
func finite(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lng) && !math.IsInf(lat, 0) && !math.IsInf(lng, 0) && math.Abs(lat) <= 90 && math.Abs(lng) <= 180
}
func distance(a, b models.ItineraryMaterial) float64 {
	return geo.DistanceMeters(a.Latitude, a.Longitude, b.Latitude, b.Longitude)
}
func narrationMinutes(text string) int {
	return max(1, int(math.Ceil(float64(len(strings.Fields(text)))/130)))
}
func fits(route []int, items []scored, shape string, budget int, speed float64) bool {
	planned, meters := 0, 0.0
	for i, n := range route {
		planned += 3 + narrationMinutes(items[n].material.Narrative)
		if i > 0 {
			meters += distance(items[route[i-1]].material, items[n].material)
		}
	}
	if len(route) > 1 {
		if shape == "loop" {
			meters += distance(items[route[len(route)-1]].material, items[route[0]].material)
		}
		if shape == "out_and_back" {
			meters *= 2
		}
	}
	return float64(planned)+meters/speed <= float64(budget)
}
func normalized(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}
func sameName(want, actual string) bool {
	return strings.TrimSpace(want) != "" && normalized(want) == normalized(actual)
}
func topicTerms(topic string) []string {
	terms := []string{}
	for _, t := range strings.Fields(normalized(topic)) {
		if len(t) >= 3 && !strings.Contains(" the and for from with history historical historic tour walking biking local ", " "+t+" ") && !slices.Contains(terms, t) {
			terms = append(terms, t)
		}
	}
	return terms
}
func relevance(m models.ItineraryMaterial, in models.RouteBriefInput, terms []string) (int, []string) {
	title, body := normalized(m.MomentTitle), normalized(m.Narrative)
	score := 0
	matched := []string{}
	for _, t := range terms {
		if strings.Contains(title, t) {
			score += 3
			matched = append(matched, t)
		} else if strings.Contains(body, t) {
			score++
			matched = append(matched, t)
		}
	}
	for _, lead := range in.EventLeads {
		term := normalized(lead)
		if len(term) >= 3 && (strings.Contains(title, term) || strings.Contains(body, term)) {
			score += 5
			matched = append(matched, "event: "+lead)
		}
	}
	return score, matched
}
