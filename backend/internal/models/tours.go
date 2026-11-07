package models

import "time"

// RouteLeg joins story stops, including an optional closing return to the first.
type RouteLeg struct {
	FromStopID       string       `json:"from_stop_id,omitempty"`
	ToStopID         string       `json:"to_stop_id,omitempty"`
	NavigationSource string       `json:"navigation_source,omitempty"`
	Return           bool         `json:"return,omitempty"`
	FromPosition     int          `json:"from_position"`
	ToPosition       int          `json:"to_position"`
	DistanceM        float64      `json:"distance_m"`
	DurationS        int          `json:"duration_s"`
	Geometry         [][2]float64 `json:"geometry"`
}

// LineString is a GeoJSON LineString with [lng, lat] coordinates.
type LineString struct {
	Type        string       `json:"type"`
	Coordinates [][2]float64 `json:"coordinates"`
}

// RouteSummary describes a route without its geometry.
type RouteSummary struct {
	Mode             TravelMode  `json:"mode"`
	Status           RouteStatus `json:"status"`
	DistanceM        float64     `json:"distance_m"`
	DurationS        int         `json:"duration_s"`
	EstimatedMinutes *int        `json:"estimated_minutes"`
	Error            string      `json:"error,omitempty"`
	ComputedAt       *time.Time  `json:"computed_at"`
}

// TourRoute is the full route for one mode.
type TourRoute struct {
	ID         string      `json:"id"`
	TourID     string      `json:"tour_id"`
	Mode       TravelMode  `json:"mode"`
	Status     RouteStatus `json:"status"`
	Provider   string      `json:"provider"`
	DistanceM  float64     `json:"distance_m"`
	DurationS  int         `json:"duration_s"`
	Geometry   *LineString `json:"geometry"`
	Legs       []RouteLeg  `json:"legs"`
	Error      string      `json:"error,omitempty"`
	ComputedAt *time.Time  `json:"computed_at"`
}

// Summary projects a route to its summary.
func (r TourRoute) Summary() RouteSummary {
	s := RouteSummary{Mode: r.Mode, Status: r.Status, DistanceM: r.DistanceM, DurationS: r.DurationS, Error: r.Error, ComputedAt: r.ComputedAt}
	if r.Status == RouteStatusReady {
		m := (r.DurationS + 59) / 60
		s.EstimatedMinutes = &m
	}
	return s
}

// TourSummary is a tour without stops.
type TourSummary struct {
	DraftVersion   int64                       `json:"draft_version"`
	RouteVersion   int64                       `json:"route_version"`
	RevisionID     *string                     `json:"revision_id"`
	RevisionNumber int64                       `json:"revision_number"`
	ID             string                      `json:"id"`
	CityID         string                      `json:"city_id"`
	CityIDs        []string                    `json:"city_ids"`
	Slug           string                      `json:"slug"`
	Title          string                      `json:"title"`
	Summary        string                      `json:"summary"`
	TopicID        *string                     `json:"topic_id"`
	Topic          *Topic                      `json:"topic"`
	Modes          []TravelMode                `json:"modes"`
	HeroMedia      *MediaAsset                 `json:"hero_media"`
	Status         EntityStatus                `json:"status"`
	Visibility     TourVisibility              `json:"visibility"`
	OwnerAccountID *string                     `json:"owner_account_id"`
	StopCount      int                         `json:"stop_count"`
	Routes         map[TravelMode]RouteSummary `json:"routes"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
}

// TourStop is one ordered stop on a tour.
type TourStop struct {
	ID               string         `json:"id"`
	Position         int            `json:"position"`
	Place            PlaceSummary   `json:"place"`
	FeaturedMomentID *string        `json:"featured_moment_id"`
	FeaturedMoment   *MomentSummary `json:"featured_moment"`
	Note             string         `json:"note"`
}

// TourDetail is a tour with its stops.
type TourDetail struct {
	WorkingCopy bool `json:"-"`
	TourSummary
	Stops    []TourStop        `json:"stops"`
	Corridor *ApprovedCorridor `json:"corridor,omitempty"`
}

// TourProgress records a visitor's progress through a tour.
type TourProgress struct {
	RevisionID       string     `json:"revision_id,omitempty"`
	AccountID        string     `json:"account_id"`
	TourID           string     `json:"tour_id"`
	Mode             TravelMode `json:"mode"`
	VisitedPositions []int      `json:"visited_positions"`
	StartedAt        time.Time  `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
