package models

// EntityStatus is the publication state of content.
type EntityStatus string

const (
	EntityStatusDraft     EntityStatus = "draft"
	EntityStatusInReview  EntityStatus = "in_review"
	EntityStatusPublished EntityStatus = "published"
	EntityStatusArchived  EntityStatus = "archived"
)

func (s EntityStatus) Valid() bool {
	switch s {
	case EntityStatusDraft, EntityStatusInReview, EntityStatusPublished, EntityStatusArchived:
		return true
	}
	return false
}

// TourVisibility controls who can see a tour.
type TourVisibility string

const (
	TourVisibilityPublic   TourVisibility = "public"
	TourVisibilityUnlisted TourVisibility = "unlisted"
	TourVisibilityPrivate  TourVisibility = "private"
)

func (v TourVisibility) Valid() bool {
	switch v {
	case TourVisibilityPublic, TourVisibilityUnlisted, TourVisibilityPrivate:
		return true
	}
	return false
}

// TravelMode is how a tour is travelled.
type TravelMode string

const (
	TravelModeWalk TravelMode = "walk"
	TravelModeBike TravelMode = "bike"
)

// AllTravelModes lists every supported mode in canonical order.
var AllTravelModes = []TravelMode{TravelModeWalk, TravelModeBike}

func (m TravelMode) Valid() bool {
	return m == TravelModeWalk || m == TravelModeBike
}

// MediaKind is the media type of an asset.
type MediaKind string

const (
	MediaKindImage    MediaKind = "image"
	MediaKindVideo    MediaKind = "video"
	MediaKindAudio    MediaKind = "audio"
	MediaKindDocument MediaKind = "document"
)

func (k MediaKind) Valid() bool {
	switch k {
	case MediaKindImage, MediaKindVideo, MediaKindAudio, MediaKindDocument:
		return true
	}
	return false
}

// DatePrecision says how exact a moment's dates are.
type DatePrecision string

const (
	DatePrecisionDay   DatePrecision = "day"
	DatePrecisionMonth DatePrecision = "month"
	DatePrecisionYear  DatePrecision = "year"
	DatePrecisionCirca DatePrecision = "circa"
)

func (p DatePrecision) Valid() bool {
	switch p {
	case DatePrecisionDay, DatePrecisionMonth, DatePrecisionYear, DatePrecisionCirca:
		return true
	}
	return false
}

// AccountRole is an account's permission level.
type AccountRole string

const (
	AccountRoleVisitor AccountRole = "visitor"
	AccountRoleCurator AccountRole = "curator"
	AccountRoleAdmin   AccountRole = "admin"
)

func (r AccountRole) Valid() bool {
	switch r {
	case AccountRoleVisitor, AccountRoleCurator, AccountRoleAdmin:
		return true
	}
	return false
}

// AtLeast reports whether the role has at least the given level.
func (r AccountRole) AtLeast(min AccountRole) bool {
	rank := map[AccountRole]int{AccountRoleVisitor: 0, AccountRoleCurator: 1, AccountRoleAdmin: 2}
	return rank[r] >= rank[min]
}

// AuthProvider identifies who verified a sign-in.
type AuthProvider string

const (
	AuthProviderApple  AuthProvider = "apple"
	AuthProviderGoogle AuthProvider = "google"
	AuthProviderDev    AuthProvider = "dev"
)

func (p AuthProvider) Valid() bool {
	switch p {
	case AuthProviderApple, AuthProviderGoogle, AuthProviderDev:
		return true
	}
	return false
}

// CandidateKind is what a review-queue entry proposes to create.
type CandidateKind string

const (
	CandidateKindPlace  CandidateKind = "place"
	CandidateKindMoment CandidateKind = "moment"
	CandidateKindMedia  CandidateKind = "media"
)

func (k CandidateKind) Valid() bool {
	switch k {
	case CandidateKindPlace, CandidateKindMoment, CandidateKindMedia:
		return true
	}
	return false
}

// CandidateOrigin is where a review-queue entry came from.
type CandidateOrigin string

const (
	CandidateOriginImport CandidateOrigin = "import"
	CandidateOriginDraft  CandidateOrigin = "draft"
	CandidateOriginUser   CandidateOrigin = "user"
)

func (o CandidateOrigin) Valid() bool {
	switch o {
	case CandidateOriginImport, CandidateOriginDraft, CandidateOriginUser:
		return true
	}
	return false
}

// CandidateStatus is the review outcome.
type CandidateStatus string

const (
	CandidateStatusPending  CandidateStatus = "pending"
	CandidateStatusApproved CandidateStatus = "approved"
	CandidateStatusRejected CandidateStatus = "rejected"
)

func (s CandidateStatus) Valid() bool {
	switch s {
	case CandidateStatusPending, CandidateStatusApproved, CandidateStatusRejected:
		return true
	}
	return false
}

// RouteStatus is the computation state of a tour route.
type RouteStatus string

const (
	RouteStatusPending RouteStatus = "pending"
	RouteStatusReady   RouteStatus = "ready"
	RouteStatusFailed  RouteStatus = "failed"
)

// JobStatus is the lifecycle state of a background job.
type JobStatus string

const (
	JobStatusQueued  JobStatus = "queued"
	JobStatusRunning JobStatus = "running"
	JobStatusDone    JobStatus = "done"
	JobStatusFailed  JobStatus = "failed"
)

func (s JobStatus) Valid() bool {
	switch s {
	case JobStatusQueued, JobStatusRunning, JobStatusDone, JobStatusFailed:
		return true
	}
	return false
}
