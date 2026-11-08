package models

import "time"

// RouteInvestigationRequest selects observations, never approval or publication.
// Research is an optional new-place/history lead for the existing sourcing queue.
type RouteInvestigationRequest struct {
	Version          int              `json:"version"`
	RelatedReportIDs []string         `json:"related_report_ids"`
	Research         *RouteBriefInput `json:"research,omitempty"`
}

func (r RouteInvestigationRequest) Validate() Fields {
	f := Fields{}
	if r.Version < 1 {
		f["version"] = "must be the latest report version"
	}
	if len(r.RelatedReportIDs) > 19 {
		f["related_report_ids"] = "select at most 19 related reports"
	}
	if r.Research != nil {
		for k, v := range r.Research.Validate() {
			f["research."+k] = v
		}
	}
	return f
}

// InvestigationObservation retains each unverified submission. DuplicateOf is a
// conservative grouping hint, never a count of independent witnesses.
type InvestigationObservation struct {
	Report      RouteReport `json:"report"`
	DuplicateOf string      `json:"duplicate_of,omitempty"`
}

// RouteInvestigationInput is immutable, including the selected report versions.
type RouteInvestigationInput struct {
	Observations []InvestigationObservation `json:"observations"`
	BriefID      string                     `json:"brief_id"`
	EvidenceIDs  []string                   `json:"evidence_ids"`
	Research     *RouteBriefInput           `json:"research,omitempty"`
}

// RouteInvestigationResult compares the published direction of exactly one
// retained mode. It cannot update a route, source fact, story or narration.
type RouteInvestigationResult struct {
	Status                   string             `json:"status"`
	ComparisonDirection      string             `json:"comparison_direction"`
	Before                   TourRoute          `json:"before"`
	After                    *TourRoute         `json:"after"`
	DistanceChangeM          *float64           `json:"distance_change_m"`
	DurationChangeS          *int               `json:"duration_change_s"`
	AffectedStopIDs          []string           `json:"affected_stop_ids"`
	Arrivals                 []ArrivalSelection `json:"arrivals"`
	ReviewedCorridor         *ApprovedCorridor  `json:"reviewed_corridor,omitempty"`
	Evidence                 []SourceEvidence   `json:"evidence"`
	Quality                  *PathQualityReport `json:"quality,omitempty"`
	Issues                   []PathQualityIssue `json:"issues"`
	StoriesPreserved         bool               `json:"stories_preserved"`
	IndependentCorroboration bool               `json:"independent_corroboration"`
	CheckedAt                time.Time          `json:"checked_at"`
}

type RouteInvestigation struct {
	ID        string                    `json:"id"`
	ReportID  string                    `json:"report_id"`
	JobID     string                    `json:"job_id"`
	CreatedBy string                    `json:"created_by"`
	Input     RouteInvestigationInput   `json:"input"`
	Result    *RouteInvestigationResult `json:"result"`
	CreatedAt time.Time                 `json:"created_at"`
}

const JobKindInvestigateRouteReport = "investigate_route_report"

type InvestigateRouteReportPayload struct {
	InvestigationID string `json:"investigation_id"`
}
