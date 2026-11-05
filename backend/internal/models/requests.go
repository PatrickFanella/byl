package models

import (
	"encoding/json"
	"strings"
)

// Fields maps a request field to a validation message. Empty means valid.
type Fields map[string]string

func (f Fields) set(key, msg string) { f[key] = msg }

func required(f Fields, key, value string) {
	if strings.TrimSpace(value) == "" {
		f.set(key, "is required")
	}
}

func validLatLng(f Fields, lat, lng float64) {
	if lat < -90 || lat > 90 {
		f.set("latitude", "must be between -90 and 90")
	}
	if lng < -180 || lng > 180 {
		f.set("longitude", "must be between -180 and 180")
	}
}

func validModes(f Fields, modes []TravelMode) {
	for _, m := range modes {
		if !m.Valid() {
			f.set("modes", "contains an unknown mode")
			return
		}
	}
}

func validStops(f Fields, stops []StopInput, min, max int) {
	if len(stops) < min || len(stops) > max {
		f.set("stops", "must have between 2 and 30 stops")
		return
	}
	for _, s := range stops {
		if strings.TrimSpace(s.PlaceID) == "" {
			f.set("stops", "every stop needs a place_id")
			return
		}
	}
}

// CreateCityRequest creates a city.
type CreateCityRequest struct {
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Region      string  `json:"region"`
	CountryCode string  `json:"country_code"`
	CenterLat   float64 `json:"center_lat"`
	CenterLng   float64 `json:"center_lng"`
	BBoxMinLat  float64 `json:"bbox_min_lat"`
	BBoxMinLng  float64 `json:"bbox_min_lng"`
	BBoxMaxLat  float64 `json:"bbox_max_lat"`
	BBoxMaxLng  float64 `json:"bbox_max_lng"`
	Timezone    string  `json:"timezone"`
}

func (r CreateCityRequest) Validate() Fields {
	f := Fields{}
	required(f, "name", r.Name)
	validLatLng(f, r.CenterLat, r.CenterLng)
	if r.BBoxMinLat >= r.BBoxMaxLat || r.BBoxMinLng >= r.BBoxMaxLng {
		f.set("bbox", "min must be less than max")
	}
	return f
}

// UpdateCityRequest patches a city.
type UpdateCityRequest struct {
	Name        *string  `json:"name"`
	Region      *string  `json:"region"`
	CountryCode *string  `json:"country_code"`
	CenterLat   *float64 `json:"center_lat"`
	CenterLng   *float64 `json:"center_lng"`
	BBoxMinLat  *float64 `json:"bbox_min_lat"`
	BBoxMinLng  *float64 `json:"bbox_min_lng"`
	BBoxMaxLat  *float64 `json:"bbox_max_lat"`
	BBoxMaxLng  *float64 `json:"bbox_max_lng"`
	Timezone    *string  `json:"timezone"`
}

// CreateTopicRequest creates a topic.
type CreateTopicRequest struct {
	CityID      string `json:"city_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r CreateTopicRequest) Validate() Fields {
	f := Fields{}
	required(f, "city_id", r.CityID)
	required(f, "name", r.Name)
	return f
}

// UpdateTopicRequest patches a topic.
type UpdateTopicRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// CreatePlaceRequest creates a place.
type CreatePlaceRequest struct {
	CityID         string  `json:"city_id"`
	Slug           string  `json:"slug"`
	Name           string  `json:"name"`
	Summary        string  `json:"summary"`
	Kind           string  `json:"kind"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	ArrivalRadiusM float64 `json:"arrival_radius_m"`
	Address        string  `json:"address"`
}

func (r CreatePlaceRequest) Validate() Fields {
	f := Fields{}
	required(f, "city_id", r.CityID)
	required(f, "name", r.Name)
	validLatLng(f, r.Latitude, r.Longitude)
	if r.ArrivalRadiusM < 0 || r.ArrivalRadiusM > 500 {
		f.set("arrival_radius_m", "must be between 0 and 500")
	}
	return f
}

// UpdatePlaceRequest patches a place.
type UpdatePlaceRequest struct {
	Name            *string  `json:"name"`
	Summary         *string  `json:"summary"`
	Kind            *string  `json:"kind"`
	Latitude        *float64 `json:"latitude"`
	Longitude       *float64 `json:"longitude"`
	ArrivalRadiusM  *float64 `json:"arrival_radius_m"`
	Address         *string  `json:"address"`
	PrimaryMomentID *string  `json:"primary_moment_id"`
}

func (r UpdatePlaceRequest) Validate() Fields {
	f := Fields{}
	if r.Latitude != nil && (*r.Latitude < -90 || *r.Latitude > 90) {
		f.set("latitude", "must be between -90 and 90")
	}
	if r.Longitude != nil && (*r.Longitude < -180 || *r.Longitude > 180) {
		f.set("longitude", "must be between -180 and 180")
	}
	if r.ArrivalRadiusM != nil && (*r.ArrivalRadiusM < 0 || *r.ArrivalRadiusM > 500) {
		f.set("arrival_radius_m", "must be between 0 and 500")
	}
	return f
}

// CreateMomentRequest creates a moment.
type CreateMomentRequest struct {
	PlaceID       string        `json:"place_id"`
	Slug          string        `json:"slug"`
	Title         string        `json:"title"`
	Body          string        `json:"body"`
	DateStart     string        `json:"date_start"`
	DateEnd       string        `json:"date_end"`
	DatePrecision DatePrecision `json:"date_precision"`
	DateDisplay   string        `json:"date_display"`
	HeroMediaID   *string       `json:"hero_media_id"`
	TopicIDs      []string      `json:"topic_ids"`
	Sources       []Source      `json:"sources"`
}

func (r CreateMomentRequest) Validate() Fields {
	f := Fields{}
	required(f, "place_id", r.PlaceID)
	required(f, "title", r.Title)
	if r.DatePrecision != "" && !r.DatePrecision.Valid() {
		f.set("date_precision", "must be day, month, year, or circa")
	}
	if _, err := ParseDate(r.DateStart); err != nil {
		f.set("date_start", "must be YYYY-MM-DD")
	}
	if _, err := ParseDate(r.DateEnd); err != nil {
		f.set("date_end", "must be YYYY-MM-DD")
	}
	return f
}

// UpdateMomentRequest patches a moment.
type UpdateMomentRequest struct {
	Title         *string        `json:"title"`
	Body          *string        `json:"body"`
	DateStart     *string        `json:"date_start"`
	DateEnd       *string        `json:"date_end"`
	DatePrecision *DatePrecision `json:"date_precision"`
	DateDisplay   *string        `json:"date_display"`
	HeroMediaID   *string        `json:"hero_media_id"`
}

func (r UpdateMomentRequest) Validate() Fields {
	f := Fields{}
	if r.DatePrecision != nil && !r.DatePrecision.Valid() {
		f.set("date_precision", "must be day, month, year, or circa")
	}
	if r.DateStart != nil {
		if _, err := ParseDate(*r.DateStart); err != nil {
			f.set("date_start", "must be YYYY-MM-DD")
		}
	}
	if r.DateEnd != nil {
		if _, err := ParseDate(*r.DateEnd); err != nil {
			f.set("date_end", "must be YYYY-MM-DD")
		}
	}
	return f
}

// StopInput is one stop in a stops list.
type StopInput struct {
	PlaceID          string  `json:"place_id"`
	FeaturedMomentID *string `json:"featured_moment_id"`
	Note             string  `json:"note"`
}

// StopsRequest replaces a tour's stops.
type StopsRequest struct {
	Stops []StopInput `json:"stops"`
}

func (r StopsRequest) Validate() Fields {
	f := Fields{}
	validStops(f, r.Stops, 1, 30)
	return f
}

// CreateTourRequest creates a curated tour.
type CreateTourRequest struct {
	CityID      string         `json:"city_id"`
	Slug        string         `json:"slug"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary"`
	TopicID     *string        `json:"topic_id"`
	Modes       []TravelMode   `json:"modes"`
	HeroMediaID *string        `json:"hero_media_id"`
	Visibility  TourVisibility `json:"visibility"`
	Stops       []StopInput    `json:"stops"`
}

func (r CreateTourRequest) Validate() Fields {
	f := Fields{}
	required(f, "city_id", r.CityID)
	required(f, "title", r.Title)
	validModes(f, r.Modes)
	if r.Visibility != "" && !r.Visibility.Valid() {
		f.set("visibility", "must be public, unlisted, or private")
	}
	if len(r.Stops) > 0 {
		validStops(f, r.Stops, 1, 30)
	}
	return f
}

// UpdateTourRequest patches a tour.
type UpdateTourRequest struct {
	Title       *string         `json:"title"`
	Summary     *string         `json:"summary"`
	TopicID     *string         `json:"topic_id"`
	Modes       []TravelMode    `json:"modes"`
	HeroMediaID *string         `json:"hero_media_id"`
	Visibility  *TourVisibility `json:"visibility"`
}

func (r UpdateTourRequest) Validate() Fields {
	f := Fields{}
	validModes(f, r.Modes)
	if r.Visibility != nil && !r.Visibility.Valid() {
		f.set("visibility", "must be public, unlisted, or private")
	}
	return f
}

// CreateUserTourRequest creates a user route.
type CreateUserTourRequest struct {
	City       string         `json:"city"`
	Title      string         `json:"title"`
	Summary    string         `json:"summary"`
	Modes      []TravelMode   `json:"modes"`
	Visibility TourVisibility `json:"visibility"`
	Stops      []StopInput    `json:"stops"`
}

func (r CreateUserTourRequest) Validate() Fields {
	f := Fields{}
	required(f, "city", r.City)
	required(f, "title", r.Title)
	validModes(f, r.Modes)
	if r.Visibility == TourVisibilityPublic || (r.Visibility != "" && !r.Visibility.Valid()) {
		f.set("visibility", "must be unlisted or private")
	}
	validStops(f, r.Stops, 2, 30)
	return f
}

// UpdateUserTourRequest patches a user route.
type UpdateUserTourRequest struct {
	Title      *string         `json:"title"`
	Summary    *string         `json:"summary"`
	Modes      []TravelMode    `json:"modes"`
	Visibility *TourVisibility `json:"visibility"`
	Stops      []StopInput     `json:"stops"`
}

func (r UpdateUserTourRequest) Validate() Fields {
	f := Fields{}
	if r.Title != nil && strings.TrimSpace(*r.Title) == "" {
		f.set("title", "is required")
	}
	validModes(f, r.Modes)
	if r.Visibility != nil && (*r.Visibility == TourVisibilityPublic || !r.Visibility.Valid()) {
		f.set("visibility", "must be unlisted or private")
	}
	if r.Stops != nil {
		validStops(f, r.Stops, 2, 30)
	}
	return f
}

// ProgressRequest records progress on a tour.
type ProgressRequest struct {
	Mode             TravelMode `json:"mode"`
	VisitedPositions []int      `json:"visited_positions"`
	Completed        bool       `json:"completed"`
}

func (r ProgressRequest) Validate() Fields {
	f := Fields{}
	if !r.Mode.Valid() {
		f.set("mode", "must be walk or bike")
	}
	for _, p := range r.VisitedPositions {
		if p < 1 {
			f.set("visited_positions", "positions start at 1")
			break
		}
	}
	return f
}

// CreateCandidateRequest adds an entry to the review queue by hand.
type CreateCandidateRequest struct {
	Kind           CandidateKind   `json:"kind"`
	Origin         CandidateOrigin `json:"origin"`
	SourceName     *string         `json:"source_name"`
	ExternalID     *string         `json:"external_id"`
	TargetPlaceID  *string         `json:"target_place_id"`
	TargetMomentID *string         `json:"target_moment_id"`
	Payload        json.RawMessage `json:"payload"`
}

func (r CreateCandidateRequest) Validate() Fields {
	f := Fields{}
	if !r.Kind.Valid() {
		f.set("kind", "must be place, moment, or media")
	}
	if r.Origin != "" && !r.Origin.Valid() {
		f.set("origin", "must be import, draft, or user")
	}
	if len(r.Payload) == 0 || !json.Valid(r.Payload) {
		f.set("payload", "must be a JSON object")
	}
	return f
}

// ReviewRequest approves or rejects a candidate.
type ReviewRequest struct {
	Note    string          `json:"note"`
	Payload json.RawMessage `json:"payload"`
}

// MediaMetadataRequest patches asset metadata.
type MediaMetadataRequest struct {
	Caption    *string `json:"caption"`
	Credit     *string `json:"credit"`
	License    *string `json:"license"`
	LicenseURL *string `json:"license_url"`
	SourceURL  *string `json:"source_url"`
}

// SourcesRequest replaces a moment's sources.
type SourcesRequest struct {
	Sources []Source `json:"sources"`
}

func (r SourcesRequest) Validate() Fields {
	f := Fields{}
	for _, s := range r.Sources {
		if strings.TrimSpace(s.Title) == "" {
			f.set("sources", "every source needs a title")
			break
		}
	}
	return f
}

// TopicsRequest replaces a moment's topics.
type TopicsRequest struct {
	TopicIDs []string `json:"topic_ids"`
}

// MediaListRequest replaces a moment's media.
type MediaListRequest struct {
	MediaIDs []string `json:"media_ids"`
}

// RoleRequest changes an account's role.
type RoleRequest struct {
	Role AccountRole `json:"role"`
}

func (r RoleRequest) Validate() Fields {
	f := Fields{}
	if !r.Role.Valid() {
		f.set("role", "must be visitor, curator, or admin")
	}
	return f
}

// AppleSignInRequest exchanges an Apple identity token.
type AppleSignInRequest struct {
	IdentityToken string `json:"identity_token"`
	DisplayName   string `json:"display_name"`
}

// GoogleSignInRequest exchanges a Google ID token.
type GoogleSignInRequest struct {
	IDToken string `json:"id_token"`
}

// DevSignInRequest signs in by email when DEV_AUTH_ENABLED is set.
type DevSignInRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func (r DevSignInRequest) Validate() Fields {
	f := Fields{}
	if !strings.Contains(r.Email, "@") {
		f.set("email", "must be an email address")
	}
	return f
}
