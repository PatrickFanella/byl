package models

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// NetworkWay is an ordered source assertion, oriented with its reference segment.
// Reverse indicates travel against the OSM way's digitized direction.
type NetworkWay struct {
	ID      int64 `json:"id"`
	Reverse bool  `json:"reverse"`
}

// CorridorNetwork binds the reviewed reference to identities, not nearby lines.
// Its interpretation requires the private retained inspection cited at proposal.
type CorridorNetwork struct {
	Ways       []NetworkWay `json:"ways"`
	FromNodeID int64        `json:"from_node_id"`
	ToNodeID   int64        `json:"to_node_id"`
}

// CorridorAccess records a sourced decision independently for each travel mode.
type CorridorAccess struct {
	Mode   TravelMode `json:"mode"`
	Status string     `json:"status"`
	Notes  string     `json:"notes"`
}

// CorridorEvidenceRef is private. Public corridor intent exposes no inspection
// IDs or quoted excerpts. A citation proves traceability, not semantic truth.
type CorridorEvidenceRef struct {
	SegmentID  string     `json:"segment_id"`
	Purpose    string     `json:"purpose"` // network or access
	Mode       TravelMode `json:"mode,omitempty"`
	EvidenceID string     `json:"evidence_id"`
	Reference  string     `json:"reference"`
	Quote      string     `json:"quote"`
}

func (s CorridorSegment) ValidateQualityReference() error {
	if s.Network != nil {
		if len(s.Network.Ways) < 1 || len(s.Network.Ways) > 64 || s.Network.FromNodeID <= 0 || s.Network.ToNodeID <= 0 {
			return fmt.Errorf("network reference needs 1–64 ordered ways and positive endpoint node IDs")
		}
		for _, way := range s.Network.Ways {
			if way.ID <= 0 {
				return fmt.Errorf("network way IDs must be positive")
			}
		}
	}
	seen := map[TravelMode]bool{}
	for _, a := range s.Access {
		if !a.Mode.Valid() || seen[a.Mode] || utf8.RuneCountInString(a.Notes) > 1000 {
			return fmt.Errorf("access needs distinct modes and bounded notes")
		}
		seen[a.Mode] = true
		switch a.Status {
		case "allowed", "unknown", "restricted", "closed":
		default:
			return fmt.Errorf("access status must be allowed, unknown, restricted or closed")
		}
	}
	return nil
}

// PathEdge contains provider assertions, never independently inspected access.
type PathEdge struct {
	ID              int64  `json:"id"`
	WayID           int64  `json:"way_id"`
	BeginNodeID     int64  `json:"begin_node_id"`
	EndNodeID       int64  `json:"end_node_id"`
	Forward         bool   `json:"forward"`
	Use             string `json:"use"`
	Traversability  string `json:"traversability"`
	TravelMode      string `json:"travel_mode"`
	Bridge          bool   `json:"bridge"`
	Tunnel          bool   `json:"tunnel"`
	BeginShapeIndex int    `json:"begin_shape_index"`
	EndShapeIndex   int    `json:"end_shape_index"`
}

type AttributedLeg struct {
	Geometry     [][2]float64 `json:"geometry"`
	Edges        []PathEdge   `json:"edges"`
	OSMChangeset int64        `json:"osm_changeset"`
}

// PathAttribution identifies the strict inspection method and bounded outcome.
type PathAttribution struct {
	Provider string          `json:"provider"`
	Method   string          `json:"method"`
	Legs     []AttributedLeg `json:"legs"`
	Failure  string          `json:"failure,omitempty"`
}

type PathQualityIssue struct {
	Code      string `json:"code"`
	Severity  string `json:"severity"` // blocked, needs_research, information
	Origin    string `json:"origin"`   // inspected_source, provider_assertion, curator_assertion, unknown
	SegmentID string `json:"segment_id,omitempty"`
	Detail    string `json:"detail"`
}

// PathInspection records only the particular retained source passages inspected.
type PathInspection struct {
	SegmentID     string     `json:"segment_id"`
	Purpose       string     `json:"purpose"`
	Mode          TravelMode `json:"mode,omitempty"`
	EvidenceID    string     `json:"evidence_id"`
	URL           string     `json:"url"`
	ContentSHA256 string     `json:"content_sha256"`
	RetrievedAt   time.Time  `json:"retrieved_at"`
	Reference     string     `json:"reference"`
	Quote         string     `json:"quote"`
}

type PathQualitySummary struct {
	ReportID     string             `json:"report_id,omitempty"`
	InputCurrent bool               `json:"input_current"`
	Version      int                `json:"version"`
	Status       string             `json:"status"` // pass means eligible for curator review, not publication approval
	Issues       []PathQualityIssue `json:"issues"`
	CheckedAt    time.Time          `json:"checked_at"`
}

// PathQualityReport is a private checkpoint tied to exact working routing input.
type PathQualityReport struct {
	RouteSHA256 string `json:"route_sha256"`
	PathQualitySummary
	InputHash   string           `json:"input_hash"`
	Mode        TravelMode       `json:"mode"`
	Provider    string           `json:"provider"`
	Inspections []PathInspection `json:"inspections"`
	Attribution *PathAttribution `json:"attribution,omitempty"`
}

// Valid checks checkpoint coherence; the routing assessor supplies the findings.
func (r PathQualityReport) Valid() bool {
	if r.Version != 1 || !r.Mode.Valid() || r.InputHash == "" || r.Issues == nil || len(r.RouteSHA256) != 64 || r.CheckedAt.IsZero() || len(r.Issues) > 512 || len(r.Inspections) > 192 {
		return false
	}
	status := "pass"
	for _, issue := range r.Issues {
		if issue.Code == "" || len(issue.Code) > 100 || len(issue.Detail) > 2000 {
			return false
		}
		switch issue.Severity {
		case "blocked":
			status = "blocked"
		case "needs_research":
			if status == "pass" {
				status = "needs_research"
			}
		case "information":
		default:
			return false
		}
		switch issue.Origin {
		case "inspected_source", "provider_assertion", "curator_assertion", "unknown":
		default:
			return false
		}
	}
	if r.Status != status {
		return false
	}
	if status == "pass" && (r.Provider != "valhalla" || len(r.Inspections) == 0 || r.Attribution == nil || r.Attribution.Failure != "" || r.Attribution.Method != "trace_attributes/edge_walk" || len(r.Attribution.Legs) == 0) {
		return false
	}
	raw, err := json.Marshal(r)
	return err == nil && len(raw) <= 1<<20
}

// InspectPathEvidence accepts completed inspected pages and exact retained quotes.
// Catalogue metadata and URL labels alone cannot supply path/access inspection.
func InspectPathEvidence(ref CorridorEvidenceRef, e *SourceEvidence) (PathInspection, error) {
	if (ref.Purpose != "network" && ref.Purpose != "access") || ref.SegmentID == "" || (ref.Purpose == "access" && !ref.Mode.Valid()) || (ref.Purpose == "network" && ref.Mode != "") {
		return PathInspection{}, fmt.Errorf("path citation needs a segment and explicit network or mode-access purpose")
	}
	if e == nil || e.ID != ref.EvidenceID || e.Result == nil || e.Result.Outcome != "inspected_page" || e.Result.HTTPStatus < 200 || e.Result.HTTPStatus >= 300 || !webURL(e.Result.FinalURL) || e.Result.RetrievedAt.IsZero() || len(e.Result.ContentSHA256) != 64 || strings.TrimSpace(ref.Quote) == "" || utf8.RuneCountInString(ref.Quote) > 300 {
		return PathInspection{}, fmt.Errorf("path citation needs a completed inspected page and bounded quote")
	}
	if _, err := hex.DecodeString(e.Result.ContentSHA256); err != nil {
		return PathInspection{}, fmt.Errorf("path citation requires the retained content checksum")
	}
	for _, p := range e.Result.Passages {
		if p.Reference == ref.Reference && strings.Contains(p.Text, ref.Quote) {
			return PathInspection{SegmentID: ref.SegmentID, Purpose: ref.Purpose, Mode: ref.Mode, EvidenceID: e.ID, URL: e.Result.FinalURL, ContentSHA256: e.Result.ContentSHA256, RetrievedAt: e.Result.RetrievedAt, Reference: ref.Reference, Quote: ref.Quote}, nil
		}
	}
	return PathInspection{}, fmt.Errorf("path quote does not match its retained inspected passage")
}
