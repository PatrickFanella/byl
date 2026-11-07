package models

import (
	"encoding/json"
	"time"
)

// TourSnapshot pins reviewed content and route inputs/results without live joins.
type TourSnapshot struct {
	Tour          TourDetail               `json:"tour"`
	Moments       map[string]MomentDetail  `json:"moments"`
	Routes        map[TravelMode]TourRoute `json:"routes"`
	RoutingIntent json.RawMessage          `json:"routing_intent"`
}

// TourRevision is an immutable published release of a tour.
type TourRevision struct {
	ID           string        `json:"id"`
	TourID       string        `json:"tour_id"`
	Number       int64         `json:"number"`
	DraftVersion int64         `json:"draft_version"`
	PublishedAt  time.Time     `json:"published_at"`
	Snapshot     *TourSnapshot `json:"snapshot,omitempty"`
}

// PublishRevisionRequest compares both the editable draft and the published pointer.
type PublishRevisionRequest struct {
	DraftVersion      int64   `json:"draft_version"`
	CurrentRevisionID *string `json:"current_revision_id"`
}

// RevisionProgress stores visits by stable stop ID within exactly one revision.
type RevisionProgress struct {
	AccountID      string     `json:"account_id"`
	TourID         string     `json:"tour_id"`
	RevisionID     string     `json:"revision_id"`
	Mode           TravelMode `json:"mode"`
	VisitedStopIDs []string   `json:"visited_stop_ids"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// RevisionProgressRequest never infers identity from a stop's position.
type RevisionProgressRequest struct {
	Mode           TravelMode `json:"mode"`
	VisitedStopIDs []string   `json:"visited_stop_ids"`
	Completed      bool       `json:"completed"`
	Reset          bool       `json:"reset"`
}

// Validate bounds revision progress and preserves the explicit reset convention.
func (r RevisionProgressRequest) Validate() Fields {
	f := Fields{}
	if !r.Mode.Valid() {
		f["mode"] = "must be walk or bike"
	}
	if len(r.VisitedStopIDs) > 30 {
		f["visited_stop_ids"] = "must contain at most 30 stop IDs"
	}
	if r.Reset && (len(r.VisitedStopIDs) > 0 || r.Completed) {
		f["reset"] = "reset must not include visits or completion"
	}
	return f
}
