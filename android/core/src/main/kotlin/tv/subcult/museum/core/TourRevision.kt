package tv.subcult.museum.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ArrivalAccess(
    val mode: String, val status: String, val surface: String = "unknown",
    @SerialName("source_url") val sourceUrl: String = "",
    @SerialName("source_title") val sourceTitle: String = "",
    @SerialName("retrieved_at") val retrievedAt: String = "",
    val notes: String = ""
)

@Serializable
data class ArrivalPoint(
    val name: String, val kind: String, val latitude: Double?, val longitude: Double?,
    val access: List<ArrivalAccess> = emptyList(), val notes: String = ""
)

@Serializable
data class ApprovedArrival(
    val id: String, val version: Int, val point: ArrivalPoint,
    @SerialName("reviewed_at") val reviewedAt: String
) {
    fun navigationPoint(mode: String): Point? {
        val lat = point.latitude ?: return null
        val lng = point.longitude ?: return null
        if (!lat.isFinite() || !lng.isFinite() || lat !in -90.0..90.0 || lng !in -180.0..180.0) return null
        if (point.access.filter { it.mode == mode }.map { it.status } != listOf("allowed")) return null
        return Point(lat, lng)
    }
}

/** Only the public reference and mode-specific notices needed by the visitor client. */
@Serializable
data class ReviewedCorridor(val intent: Intent? = null) {
    @Serializable data class Reference(val note: String = "")
    @Serializable data class Access(val mode: String, val notes: String = "")
    @Serializable data class Segment(val id: String, val access: List<Access> = emptyList(),
        val geometry: kotlinx.serialization.json.JsonElement? = null,
        @SerialName("signal_references") val signalReferences: kotlinx.serialization.json.JsonElement? = null)
    @Serializable data class Traversal(@SerialName("segment_id") val segmentId: String, val reverse: Boolean = false)
    @Serializable data class Selection(val mode: String, val path: List<Traversal> = emptyList())
    @Serializable data class Intent(val source: Reference, val segments: List<Segment>, val routes: List<Selection>)

    fun notices(mode: String): List<String> {
        val selected = intent ?: return emptyList()
        val ids = selected.routes.filter { it.mode == mode }.flatMap { it.path.map { step -> step.segmentId } }.toSet()
        return (listOf(selected.source.note) + selected.segments.filter { it.id in ids }.flatMap { it.access.filter { access -> access.mode == mode }.map { access -> access.notes } })
            .filter { it.isNotBlank() }.distinct()
    }
}

@Serializable
data class TourSnapshot(val tour: TourDetail, val moments: Map<String, MomentDetail>, val routes: Map<String, TourRoute>)

@Serializable
data class TourRevision(
    val id: String,
    @SerialName("tour_id") val tourId: String,
    val number: Int,
    @SerialName("published_at") val publishedAt: String,
    val snapshot: TourSnapshot? = null
) {
    /** A computed line alone cannot authorize a reviewed trip. */
    fun unavailableReason(mode: String): String? {
        val retained = snapshot
        if (mode !in listOf("walk", "bike") || id.isBlank() || retained == null || retained.tour.id != tourId || retained.tour.revisionId != id || retained.tour.revisionNumber != number || retained.tour.status != "published" || mode !in retained.tour.modes)
            return "The reviewed itinerary is unavailable for this mode."
        val stops = retained.tour.stops
        if (stops.size !in 2..30 || stops.any { it.id.isBlank() } || stops.map { it.id }.toSet().size != stops.size || stops.map { it.position }.toSet() != (1..stops.size).toSet())
            return "The reviewed stop identities could not be read."
        val route = retained.routes[mode]
        val journey = route?.journey
        val geometry = route?.geometry
        if (route == null || route.mode != mode || route.tourId != tourId || route.status != "ready" || journey == null || journey.version != 1 || !journey.usable || journey.state != "usable" || journey.quality != "pass" || geometry?.type != "LineString" || geometry.coordinates.size < 2 || geometry.coordinates.any { it.size != 2 || !it[0].isFinite() || !it[1].isFinite() || it[0] !in -180.0..180.0 || it[1] !in -90.0..90.0 })
            return "The path is not ready for a reviewed trip in this mode."
        if (stops.any { it.place.arrivalPoint?.navigationPoint(mode) == null })
            return "A reviewed arrival point is missing or unavailable for this mode."
        if (stops.any { stop -> stop.featuredMoment?.let { retained.moments[it.id]?.id != it.id } == true })
            return "A retained story is missing from this revision."
        return null
    }
}

@Serializable
data class RevisionProgress(
    @SerialName("account_id") val accountId: String,
    @SerialName("tour_id") val tourId: String,
    @SerialName("revision_id") val revisionId: String,
    val mode: String,
    @SerialName("visited_stop_ids") val visitedStopIds: List<String> = emptyList(),
    @SerialName("completed_at") val completedAt: String? = null
)

@Serializable
data class RevisionProgressRequest(
    val mode: String,
    @SerialName("visited_stop_ids") val visitedStopIds: List<String>,
    val completed: Boolean,
    val reset: Boolean = false
)

fun TourDetail.canStart(mode: String): Boolean = revisionId != null && status == "published" &&
    mode in listOf("walk", "bike") && mode in modes && journeys?.get(mode)?.let { it.version == 1 && it.usable && it.state == "usable" && it.quality == "pass" } == true &&
    stops.isNotEmpty() && stops.all { it.place.arrivalPoint?.navigationPoint(mode) != null }
