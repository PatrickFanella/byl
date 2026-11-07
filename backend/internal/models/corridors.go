package models

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"invisible-museum/backend/internal/geo"
)

// CorridorSource identifies the submitted reference and its copying/access context.
// Retaining this metadata is not a legal-access or field-inspection certificate.
type CorridorSource struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Publisher   string `json:"publisher"`
	Version     string `json:"version"`
	RetrievedOn string `json:"retrieved_on"`
	License     string `json:"license"`
	LicenseURL  string `json:"license_url"`
	Note        string `json:"note"`
}

// ReferenceLine retains dimensions so malformed imports cannot be truncated.
type ReferenceLine struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

// CorridorSegment has explicit endpoint identities. Geometry crossings create
// no connection; road connectors must be selected and reviewed explicitly.
type CorridorSegment struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	FromNode string          `json:"from_node"`
	ToNode   string          `json:"to_node"`
	Branches []string        `json:"branches"`
	Modes    []TravelMode    `json:"modes"`
	Geometry ReferenceLine   `json:"geometry"`
	Source   *CorridorSource `json:"source"`
}

// CorridorTraversal selects one directed reference segment, including repeats.
type CorridorTraversal struct {
	SegmentID string `json:"segment_id"`
	Reverse   bool   `json:"reverse"`
}

// ShapingPoint is routing intent between story stops, never an exhibit/arrival.
type ShapingPoint struct {
	ID             string   `json:"id"`
	SegmentID      string   `json:"segment_id"`
	TraversalIndex int      `json:"traversal_index"`
	AfterStopID    string   `json:"after_stop_id"`
	BeforeStopID   string   `json:"before_stop_id"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
	Note           string   `json:"note"`
}

// ModeCorridor records independent walking/biking choices, with no mode fallback.
type ModeCorridor struct {
	Mode              TravelMode          `json:"mode"`
	Branch            string              `json:"branch"`
	Direction         string              `json:"direction"`
	Path              []CorridorTraversal `json:"path"`
	AllowedConnectors []string            `json:"allowed_connectors"`
	ShapingPoints     []ShapingPoint      `json:"shaping_points"`
}

// CorridorIntent is a bounded imported reference plus the reviewed selection.
type CorridorIntent struct {
	Key      string            `json:"key"`
	Name     string            `json:"name"`
	Source   CorridorSource    `json:"source"`
	Segments []CorridorSegment `json:"segments"`
	Routes   []ModeCorridor    `json:"routes"`
}

// CorridorProposalRequest compares the approved pointer and captures the draft
// version server-side. Removal must be explicit and separately reviewed.
type CorridorProposalRequest struct {
	Version int64           `json:"version"`
	Intent  *CorridorIntent `json:"intent"`
	Remove  bool            `json:"remove"`
}

// ApprovedCorridor excludes private authors and review rationales from snapshots.
type ApprovedCorridor struct {
	ID            string          `json:"id"`
	Version       int64           `json:"version"`
	Intent        *CorridorIntent `json:"intent"`
	ContentSHA256 string          `json:"content_sha256"`
	ReviewedAt    time.Time       `json:"reviewed_at"`
}

// CorridorProposal is private curator review history.
type CorridorProposal struct {
	ID            string          `json:"id"`
	TourID        string          `json:"tour_id"`
	BaseVersion   int64           `json:"base_version"`
	DraftVersion  int64           `json:"draft_version"`
	Intent        *CorridorIntent `json:"intent"`
	Remove        bool            `json:"remove"`
	ContentSHA256 string          `json:"content_sha256"`
	Status        string          `json:"status"`
	CreatedBy     string          `json:"created_by"`
	ReviewedBy    *string         `json:"reviewed_by"`
	ReviewNote    string          `json:"review_note"`
	CreatedAt     time.Time       `json:"created_at"`
	ReviewedAt    *time.Time      `json:"reviewed_at"`
}

func boundedIdentity(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 100
}

func (s CorridorSource) valid() bool {
	date, err := ParseDate(s.RetrievedOn)
	return webURL(s.URL) && len(s.URL) <= 2000 && webURL(s.LicenseURL) && len(s.LicenseURL) <= 2000 &&
		strings.TrimSpace(s.Title) != "" && utf8.RuneCountInString(s.Title) <= 300 &&
		strings.TrimSpace(s.Publisher) != "" && utf8.RuneCountInString(s.Publisher) <= 200 &&
		boundedIdentity(s.Version) && boundedIdentity(s.License) && err == nil && date != nil && !date.IsZero() && utf8.RuneCountInString(s.Note) <= 1000
}

// Validate checks explicit topology and bounded input, not real-world access.
func (c CorridorIntent) Validate() Fields {
	f := Fields{}
	if !boundedIdentity(c.Key) || strings.TrimSpace(c.Name) == "" || utf8.RuneCountInString(c.Name) > 300 || !c.Source.valid() {
		f["source"] = "provide corridor identity and bounded source, version, retrieval and license metadata"
	}
	if len(c.Segments) < 1 || len(c.Segments) > 64 || len(c.Routes) < 1 || len(c.Routes) > 2 {
		f["segments"] = "reference allows 1–64 segments and 1–2 independent mode selections"
		return f
	}
	segments := map[string]CorridorSegment{}
	nodes := map[string][2]float64{}
	vertices := 0
	for _, s := range c.Segments {
		if !boundedIdentity(s.ID) || !boundedIdentity(s.FromNode) || !boundedIdentity(s.ToNode) {
			f["segments"] = "segment and endpoint node identities are required"
		}
		if _, exists := segments[s.ID]; exists {
			f["segments"] = "segment identities must be unique"
		}
		segments[s.ID] = s
		if (s.Kind != "trail" && s.Kind != "road_connector") || (s.Kind == "road_connector" && s.Source == nil) || (s.Source != nil && !s.Source.valid()) {
			f["segments"] = "kind must be trail or road_connector; road connectors need their own source context"
		}
		if len(s.Branches) < 1 || len(s.Branches) > 8 || len(s.Modes) > 2 {
			f["segments"] = "record 1–8 branch identities and at most two allowed modes"
		}
		for _, b := range s.Branches {
			if !boundedIdentity(b) {
				f["segments"] = "branch identity is required"
			}
		}
		if len(s.Modes) == 2 && s.Modes[0] == s.Modes[1] {
			f["segments"] = "allowed modes must be distinct"
		}
		for _, mode := range s.Modes {
			if !mode.Valid() {
				f["segments"] = "allowed modes must be walk or bike; empty means unresolved"
			}
		}
		if s.Geometry.Type != "LineString" || len(s.Geometry.Coordinates) < 2 {
			f["geometry"] = "each segment needs a LineString with at least two points"
			continue
		}
		vertices += len(s.Geometry.Coordinates)
		valid := true
		for _, p := range s.Geometry.Coordinates {
			if len(p) != 2 || !validReportPoint(p[1], p[0]) {
				f["geometry"] = "reference needs finite WGS84 longitude/latitude pairs"
				valid = false
			}
		}
		if !valid {
			continue
		}
		for _, endpoint := range []struct {
			id string
			p  []float64
		}{{s.FromNode, s.Geometry.Coordinates[0]}, {s.ToNode, s.Geometry.Coordinates[len(s.Geometry.Coordinates)-1]}} {
			point := [2]float64{endpoint.p[0], endpoint.p[1]}
			if prior, exists := nodes[endpoint.id]; exists {
				if geo.DistanceMeters(prior[1], prior[0], point[1], point[0]) > 20 {
					f["nodes"] = "one declared junction's endpoints must agree within 20 m"
				}
			} else {
				nodes[endpoint.id] = point
			}
		}
	}
	if vertices > 5000 {
		f["geometry"] = "reference is limited to 5000 coordinate pairs in total"
	}
	// Avoid indexing malformed source geometry while checking selected paths.
	if len(f) > 0 {
		return f
	}
	seenModes := map[TravelMode]bool{}
	for _, route := range c.Routes {
		if !route.Mode.Valid() || seenModes[route.Mode] || !boundedIdentity(route.Branch) || !boundedIdentity(route.Direction) {
			f["routes"] = "select each mode once with an explicit branch and direction"
		}
		seenModes[route.Mode] = true
		if len(route.Path) < 1 || len(route.Path) > 64 || len(route.AllowedConnectors) > 16 || len(route.ShapingPoints) > 60 {
			f["routes"] = "each mode allows 1–64 traversals, 16 connectors and 60 shaping points"
			continue
		}
		allowed := map[string]bool{}
		for _, id := range route.AllowedConnectors {
			s, exists := segments[id]
			if !exists || s.Kind != "road_connector" || allowed[id] || !slices.Contains(s.Modes, route.Mode) || !slices.Contains(s.Branches, route.Branch) {
				f["connectors"] = "select distinct sourced road connectors for this branch and mode"
			}
			allowed[id] = true
		}
		lastNode := ""
		for _, traversal := range route.Path {
			s, exists := segments[traversal.SegmentID]
			if !exists || !slices.Contains(s.Branches, route.Branch) || !slices.Contains(s.Modes, route.Mode) || (s.Kind == "road_connector" && !allowed[s.ID]) {
				f["path"] = "path must select this branch's allowed mode segments and explicit road connectors"
				continue
			}
			from, to := s.FromNode, s.ToNode
			if traversal.Reverse {
				from, to = to, from
			}
			if lastNode != "" && from != lastNode {
				f["path"] = "segments must share an explicit endpoint identity; a crossing or nearby coordinate is not a junction"
			}
			lastNode = to
		}
		seenShapes := map[string]bool{}
		lastTraversal := -1
		for _, p := range route.ShapingPoints {
			if !boundedIdentity(p.ID) || seenShapes[p.ID] || p.AfterStopID == "" || p.BeforeStopID == "" || p.AfterStopID == p.BeforeStopID || utf8.RuneCountInString(p.Note) > 500 {
				f["shaping_points"] = "give each shaping point an identity and distinct adjacent story-stop references"
			}
			seenShapes[p.ID] = true
			if p.TraversalIndex < lastTraversal || p.TraversalIndex < 0 || p.TraversalIndex >= len(route.Path) || route.Path[p.TraversalIndex].SegmentID != p.SegmentID {
				f["shaping_points"] = "points must follow their explicit traversal occurrence in path order"
				continue
			}
			lastTraversal = p.TraversalIndex
			s, exists := segments[p.SegmentID]
			if !exists || p.Latitude == nil || p.Longitude == nil || !validReportPoint(*p.Latitude, *p.Longitude) {
				f["shaping_points"] = "each point needs explicit finite coordinates on a selected segment"
				continue
			}
			line := make([][2]float64, 0, len(s.Geometry.Coordinates))
			for _, q := range s.Geometry.Coordinates {
				line = append(line, [2]float64{q[0], q[1]})
			}
			if !geo.NearPolyline([2]float64{*p.Longitude, *p.Latitude}, line, 50) {
				f["shaping_points"] = "points must lie within 50 m of the selected reference segment"
			}
		}
	}
	return f
}

// ValidateStops checks current working itinerary references without changing them.
func (c CorridorIntent) ValidateStops(tour *TourDetail) error {
	positions := map[string]int{}
	for _, stop := range tour.Stops {
		positions[stop.ID] = stop.Position
	}
	for _, route := range c.Routes {
		if !slices.Contains(tour.Modes, route.Mode) {
			return fmt.Errorf("corridor mode no longer belongs to this tour")
		}
		last := 0
		for _, point := range route.ShapingPoints {
			after, before := positions[point.AfterStopID], positions[point.BeforeStopID]
			if after == 0 || before != after+1 || after < last {
				return fmt.Errorf("shaping points no longer match adjacent ordered story stops")
			}
			last = after
		}
	}
	return nil
}
