package models

// ContributionReview is a live curator task, never a publication decision.
// Counts expose no consent, owner-scope, account or device identifier.
type ContributionReview struct {
	TourID            string                       `json:"tour_id"`
	TourTitle         string                       `json:"tour_title"`
	RevisionID        string                       `json:"revision_id"`
	RevisionNumber    int64                        `json:"revision_number"`
	SegmentID         string                       `json:"segment_id"`
	SegmentKind       string                       `json:"segment_kind"`
	Mode              TravelMode                   `json:"mode"`
	Direction         string                       `json:"direction"`
	WindowStart       Date                         `json:"window_start"`
	WindowEnd         Date                         `json:"window_end"`
	ObservedThrough   Date                         `json:"observed_through"`
	QualifiedPassages int                          `json:"qualified_passages"`
	DepartureRejoins  int                          `json:"departure_rejoins"`
	ContributorScopes int                          `json:"contributor_scopes"`
	DepartureScopes   int                          `json:"departure_scopes"`
	Flagged           bool                         `json:"flagged"`
	Thresholds        ContributionReviewThresholds `json:"thresholds"`
	Geometry          *ReferenceLine               `json:"geometry,omitempty"`
	Reports           []ContributionReviewReport   `json:"reports"`
	ReportsTruncated  bool                         `json:"reports_truncated"`
}
type ContributionReviewReport struct {
	ID       string         `json:"id"`
	Status   BriefStatus    `json:"status"`
	Reason   string         `json:"reason"`
	Affected ReportLocation `json:"affected"`
}
type ContributionReviewThresholds struct {
	Version                 string `json:"version"`
	MinimumScopes           int    `json:"minimum_scopes"`
	MinimumDepartureScopes  int    `json:"minimum_departure_scopes"`
	MinimumDeparturePercent int    `json:"minimum_departure_percent"`
}

func ReviewContributionThresholds() ContributionReviewThresholds {
	return ContributionReviewThresholds{Version: "uncalibrated-review-v1", MinimumScopes: 5, MinimumDepartureScopes: 3, MinimumDeparturePercent: 30}
}
func (r ContributionReview) MeetsThreshold() bool {
	t := ReviewContributionThresholds()
	return r.QualifiedPassages > 0 && r.ContributorScopes >= t.MinimumScopes && r.DepartureScopes >= t.MinimumDepartureScopes && r.DepartureRejoins*100 >= r.QualifiedPassages*t.MinimumDeparturePercent
}
