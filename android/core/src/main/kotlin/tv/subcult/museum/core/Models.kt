// Data transfer objects for the Invisible Museum API. Field names follow backend/api/openapi.yaml
// through @SerialName. Detail types that the API flattens (place + moments, moment + body,
// tour + stops) are flat data classes here.
package tv.subcult.museum.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ListPage<T>(
    val items: List<T> = emptyList(),
    @SerialName("next_cursor") val nextCursor: String = ""
)

@Serializable
data class ApiErrorBody(val error: Detail) {
    @Serializable
    data class Detail(val code: String, val message: String, val fields: Map<String, String>? = null)
}

@Serializable
data class BoundingBox(
    @SerialName("min_lat") val minLat: Double,
    @SerialName("min_lng") val minLng: Double,
    @SerialName("max_lat") val maxLat: Double,
    @SerialName("max_lng") val maxLng: Double
)

@Serializable
data class City(
    val id: String,
    val slug: String,
    val name: String,
    val region: String = "",
    @SerialName("country_code") val countryCode: String = "",
    @SerialName("center_lat") val centerLat: Double,
    @SerialName("center_lng") val centerLng: Double,
    val bbox: BoundingBox,
    val timezone: String = "",
    val topics: List<Topic>? = null
)

@Serializable
data class Topic(
    val id: String,
    @SerialName("city_id") val cityId: String,
    val slug: String,
    val name: String,
    val description: String = "",
    @SerialName("moment_count") val momentCount: Int = 0
)

@Serializable
data class MediaAsset(
    val id: String,
    val kind: String,
    @SerialName("public_url") val publicUrl: String,
    @SerialName("mime_type") val mimeType: String = "",
    val width: Int? = null,
    val height: Int? = null,
    @SerialName("duration_seconds") val durationSeconds: Double? = null,
    val caption: String = "",
    val credit: String = "",
    val license: String = "",
    @SerialName("license_url") val licenseUrl: String = "",
    @SerialName("source_url") val sourceUrl: String = ""
)

@Serializable
data class PlaceSummary(
    val id: String,
    @SerialName("city_id") val cityId: String,
    val slug: String,
    val name: String,
    val summary: String = "",
    val kind: String = "",
    val latitude: Double,
    val longitude: Double,
    @SerialName("arrival_radius_m") val arrivalRadiusM: Double = 40.0,
    val address: String = "",
    val status: String = "",
    @SerialName("primary_moment_id") val primaryMomentId: String? = null,
    @SerialName("moment_count") val momentCount: Int = 0,
    @SerialName("distance_m") val distanceM: Double? = null,
    @SerialName("arrival_point") val arrivalPoint: ApprovedArrival? = null
)

@Serializable
data class PlaceDetail(
    val id: String,
    @SerialName("city_id") val cityId: String,
    val slug: String,
    val name: String,
    val summary: String = "",
    val kind: String = "",
    val latitude: Double,
    val longitude: Double,
    @SerialName("arrival_radius_m") val arrivalRadiusM: Double = 40.0,
    val address: String = "",
    val status: String = "",
    @SerialName("primary_moment_id") val primaryMomentId: String? = null,
    @SerialName("moment_count") val momentCount: Int = 0,
    @SerialName("primary_moment") val primaryMoment: MomentSummary? = null,
    val moments: List<MomentSummary> = emptyList()
) {
    val summaryOnly: PlaceSummary
        get() = PlaceSummary(id, cityId, slug, name, summary, kind, latitude, longitude, arrivalRadiusM, address, status, primaryMomentId, momentCount)
}

@Serializable
data class Source(
    val title: String = "",
    val url: String = "",
    val publisher: String = "",
    @SerialName("retrieved_at") val retrievedAt: String? = null,
    val license: String = "",
    val note: String = ""
)

@Serializable
data class MomentSummary(
    val id: String,
    @SerialName("place_id") val placeId: String,
    val slug: String,
    val title: String,
    val excerpt: String = "",
    @SerialName("date_start") val dateStart: String? = null,
    @SerialName("date_end") val dateEnd: String? = null,
    @SerialName("date_precision") val datePrecision: String = "",
    @SerialName("date_display") val dateDisplay: String = "",
    @SerialName("hero_media") val heroMedia: MediaAsset? = null,
    @SerialName("has_narration") val hasNarration: Boolean = false,
    val status: String = "",
    @SerialName("topic_ids") val topicIds: List<String> = emptyList()
)

@Serializable
data class MomentDetail(
    val id: String,
    @SerialName("place_id") val placeId: String,
    val slug: String,
    val title: String,
    val excerpt: String = "",
    @SerialName("date_start") val dateStart: String? = null,
    @SerialName("date_end") val dateEnd: String? = null,
    @SerialName("date_precision") val datePrecision: String = "",
    @SerialName("date_display") val dateDisplay: String = "",
    @SerialName("hero_media") val heroMedia: MediaAsset? = null,
    @SerialName("has_narration") val hasNarration: Boolean = false,
    val status: String = "",
    @SerialName("topic_ids") val topicIds: List<String> = emptyList(),
    val body: String = "",
    val media: List<MediaAsset> = emptyList(),
    val sources: List<Source> = emptyList(),
    val topics: List<Topic> = emptyList(),
    val narration: MediaAsset? = null
)

@Serializable
data class RouteSummary(
    val mode: String,
    val status: String,
    @SerialName("distance_m") val distanceM: Double = 0.0,
    @SerialName("duration_s") val durationS: Int = 0,
    @SerialName("estimated_minutes") val estimatedMinutes: Int? = null,
    val error: String? = null
)

@Serializable
data class RouteLeg(
    @SerialName("from_position") val fromPosition: Int,
    @SerialName("to_position") val toPosition: Int,
    @SerialName("distance_m") val distanceM: Double = 0.0,
    @SerialName("duration_s") val durationS: Int = 0,
    val geometry: List<List<Double>> = emptyList()
)

@Serializable
data class LineString(val type: String = "LineString", val coordinates: List<List<Double>> = emptyList())

@Serializable
data class TourRoute(
    val id: String,
    @SerialName("tour_id") val tourId: String,
    val mode: String,
    val status: String,
    val provider: String = "",
    @SerialName("distance_m") val distanceM: Double = 0.0,
    @SerialName("duration_s") val durationS: Int = 0,
    val geometry: LineString? = null,
    val legs: List<RouteLeg> = emptyList(),
    val error: String? = null,
    val journey: RouteJourney? = null
)

@Serializable
data class TourSummary(
    val id: String,
    @SerialName("city_id") val cityId: String,
    val slug: String,
    val title: String,
    val summary: String = "",
    @SerialName("topic_id") val topicId: String? = null,
    val topic: Topic? = null,
    val modes: List<String> = emptyList(),
    @SerialName("hero_media") val heroMedia: MediaAsset? = null,
    val status: String = "",
    val visibility: String = "",
    @SerialName("owner_account_id") val ownerAccountId: String? = null,
    @SerialName("stop_count") val stopCount: Int = 0,
    val routes: Map<String, RouteSummary> = emptyMap(),
    @SerialName("revision_id") val revisionId: String? = null,
    @SerialName("revision_number") val revisionNumber: Int? = null
) {
    val isUserRoute: Boolean get() = ownerAccountId != null
}

@Serializable
data class TourStop(
    val id: String,
    val position: Int,
    val place: PlaceSummary,
    @SerialName("featured_moment_id") val featuredMomentId: String? = null,
    @SerialName("featured_moment") val featuredMoment: MomentSummary? = null,
    val note: String = ""
)

@Serializable
data class TourDetail(
    val id: String,
    @SerialName("city_id") val cityId: String,
    val slug: String,
    val title: String,
    val summary: String = "",
    @SerialName("topic_id") val topicId: String? = null,
    val topic: Topic? = null,
    val modes: List<String> = emptyList(),
    @SerialName("hero_media") val heroMedia: MediaAsset? = null,
    val status: String = "",
    val visibility: String = "",
    @SerialName("owner_account_id") val ownerAccountId: String? = null,
    @SerialName("stop_count") val stopCount: Int = 0,
    val routes: Map<String, RouteSummary> = emptyMap(),
    val stops: List<TourStop> = emptyList(),
    val journeys: Map<String, RouteJourney>? = null,
    @SerialName("revision_id") val revisionId: String? = null,
    @SerialName("revision_number") val revisionNumber: Int? = null,
    val corridor: ReviewedCorridor? = null
) {
    val isUserRoute: Boolean get() = ownerAccountId != null
    val summaryOnly: TourSummary
        get() = TourSummary(id, cityId, slug, title, summary, topicId, topic, modes, heroMedia, status, visibility, ownerAccountId, stopCount, routes, revisionId, revisionNumber)
}

@Serializable
data class TourProgress(
    @SerialName("account_id") val accountId: String = "",
    @SerialName("tour_id") val tourId: String,
    val mode: String,
    @SerialName("visited_positions") val visitedPositions: List<Int> = emptyList(),
    @SerialName("started_at") val startedAt: String = "",
    @SerialName("completed_at") val completedAt: String? = null
)

@Serializable
data class Account(
    val id: String,
    val provider: String = "",
    val email: String = "",
    @SerialName("display_name") val displayName: String = "",
    val role: String = "visitor"
)

@Serializable
data class Session(
    val token: String,
    @SerialName("expires_at") val expiresAt: String = "",
    val account: Account
)

// Request bodies

@Serializable
data class StopInput(@SerialName("place_id") val placeId: String, val note: String = "")

@Serializable
data class CreateUserTourRequest(
    val city: String,
    val title: String,
    val summary: String = "",
    val modes: List<String> = listOf("walk", "bike"),
    val visibility: String = "unlisted",
    val stops: List<StopInput>
)

@Serializable
data class UpdateUserTourRequest(
    val title: String? = null,
    val summary: String? = null,
    val modes: List<String>? = null,
    val visibility: String? = null,
    val stops: List<StopInput>? = null
)

@Serializable
data class ProgressRequest(
    val mode: String,
    @SerialName("visited_positions") val visitedPositions: List<Int>,
    val completed: Boolean,
    val reset: Boolean = false
)

@Serializable
data class GoogleSignInRequest(@SerialName("id_token") val idToken: String, @SerialName("display_name") val displayName: String = "")

@Serializable
data class DevSignInRequest(val email: String, @SerialName("display_name") val displayName: String = "")

// Optional on older APIs; no private evidence or provider edge payloads.
@Serializable
data class RouteJourney(
    val version: Int,
    val state: String,
    val computation: String,
    val quality: String,
    val usable: Boolean,
    @SerialName("issue_codes") val issueCodes: List<String>,
    @SerialName("travel_distance_m") val travelDistanceM: Double?,
    @SerialName("travel_duration_s") val travelDurationS: Int?,
    @SerialName("dwell_duration_s") val dwellDurationS: Int,
    @SerialName("narration_duration_s") val narrationDurationS: Int,
    @SerialName("itinerary_duration_s") val itineraryDurationS: Int?,
    @SerialName("round_trip_duration_s") val roundTripDurationS: Int?,
    @SerialName("return_plan") val returnPlan: RouteReturn,
    val segments: List<JourneySegment>,
    val assumptions: List<String>
)

@Serializable
data class RouteReturn(
    val status: String,
    @SerialName("distance_m") val distanceM: Double?,
    @SerialName("duration_s") val durationS: Int?
)

@Serializable
data class JourneySegment(
    val id: String,
    val kind: String,
    val reverse: Boolean,
    val basis: String,
    val surface: String?,
    val modes: List<String>?,
    val access: String,
    val source: JourneySource
)

@Serializable
data class JourneySource(
    val url: String,
    val title: String,
    val publisher: String,
    val version: String,
    @SerialName("retrieved_on") val retrievedOn: String,
    val license: String,
    @SerialName("license_url") val licenseURL: String
)
