package tv.subcult.museum.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Instant
import java.time.OffsetDateTime
import java.time.Duration
import kotlin.math.*

@Serializable data class ItineraryCapabilities(@SerialName("scheduled_transit_v1") val scheduledTransitV1: Boolean)
@Serializable data class TransitPlanRequest(val version: Int, @SerialName("departure_at") val departureAt: String)

@Serializable data class TransitPlatform(
    val id: String, val name: String, @SerialName("parent_id") val parentId: String,
    val latitude: Double, val longitude: Double
) {
    fun valid() = id.length in 1..20 && id.all { it in '0'..'9' } && name.isNotBlank() && latitude.isFinite() && longitude.isFinite() && latitude in -90.0..90.0 && longitude in -180.0..180.0
    fun matches(other: TransitPlatform): Boolean {
        if (!valid() || !other.valid() || id != other.id || name != other.name || parentId != other.parentId) return false
        val lat = Math.toRadians(latitude - other.latitude); val lng = Math.toRadians(longitude - other.longitude)
        val a = sin(lat / 2).pow(2) + cos(Math.toRadians(latitude)) * cos(Math.toRadians(other.latitude)) * sin(lng / 2).pow(2)
        return 6371000 * 2 * asin(sqrt(a.coerceIn(0.0, 1.0))) <= 25
    }
}
@Serializable data class TransitSource(
    val url: String, val publisher: String, val sha256: String,
    @SerialName("retrieved_at") val retrievedAt: String,
    @SerialName("license_url") val licenseUrl: String, val timezone: String
) {
    fun valid() = url == "https://www.transitchicago.com/downloads/sch_data/google_transit.zip" && publisher.isNotBlank() &&
        licenseUrl == "https://www.transitchicago.com/developers/terms/" && timezone == "America/Chicago" &&
        sha256.length == 64 && sha256.all { it in '0'..'9' || it.lowercaseChar() in 'a'..'f' } && transitInstant(retrievedAt) != null
}
@Serializable data class TransitIntent(
    val version: Int, val provider: String, val mode: String,
    @SerialName("from_stop_id") val fromStopId: String, @SerialName("to_stop_id") val toStopId: String,
    @SerialName("board_stop_id") val boardStopId: String, @SerialName("alight_stop_id") val alightStopId: String,
    @SerialName("walk_to_stop_seconds") val walkToStopSeconds: Int,
    @SerialName("walk_from_stop_seconds") val walkFromStopSeconds: Int,
    @SerialName("boarding_buffer_seconds") val boardingBufferSeconds: Int,
    @SerialName("walking_assumptions") val walkingAssumptions: String
)
@Serializable data class ApprovedTransit(
    val version: Int, val intent: TransitIntent, val board: TransitPlatform, val alight: TransitPlatform, val source: TransitSource,
    @SerialName("reviewed_at") val reviewedAt: String,
    @SerialName("walking_legs") val walkingLegs: List<RouteLeg> = emptyList()
) {
    fun valid(tour: TourDetail): Boolean {
        val i = intent; val stops = tour.stops.sortedBy { it.position }
        val from = stops.indexOfFirst { it.id == i.fromStopId }
        if (version < 1 || i.version != 1 || i.provider != "cta_gtfs_blue_v1" || i.mode != "walk" || i.mode !in tour.modes ||
            from < 0 || from + 1 >= stops.size || stops[from + 1].id != i.toStopId || stops[from + 1].position != stops[from].position + 1 ||
            !board.valid() || !alight.valid() || board.id != i.boardStopId || alight.id != i.alightStopId || board.id == alight.id ||
            i.walkToStopSeconds !in 60..3600 || i.walkFromStopSeconds !in 60..3600 || i.boardingBufferSeconds !in 60..1800 ||
            i.walkingAssumptions.isBlank() || i.walkingAssumptions.toByteArray().size > 2000 || !source.valid() || transitInstant(reviewedAt) == null ||
            walkingLegs.size != stops.size - 2) return false
        return (0 until stops.lastIndex).filter { it != from }.all { n ->
            val legs = walkingLegs.filter { it.fromPosition == stops[n].position && it.toPosition == stops[n + 1].position }
            val leg = legs.singleOrNull()
            leg != null && !leg.isReturn && (leg.fromStopId == null || leg.fromStopId == stops[n].id) && (leg.toStopId == null || leg.toStopId == stops[n + 1].id) &&
                leg.durationS >= 0 && leg.distanceM.isFinite() && leg.distanceM >= 0 && validTransitLine(leg.geometry)
        }
    }
}
@Serializable data class TransitService(
    val state: String, val scheduled: Boolean, val live: Boolean,
    val source: TransitSource? = null, val board: TransitPlatform? = null, val alight: TransitPlatform? = null,
    @SerialName("trip_id") val tripId: String? = null, @SerialName("route_id") val routeId: String? = null,
    val headsign: String? = null, @SerialName("departure_at") val departureAt: String? = null,
    @SerialName("arrival_at") val arrivalAt: String? = null, val issues: List<String> = emptyList()
)
@Serializable data class TransitSegment(
    val kind: String, @SerialName("from_stop_id") val fromStopId: String? = null,
    @SerialName("to_stop_id") val toStopId: String? = null,
    @SerialName("from_platform_id") val fromPlatformId: String? = null,
    @SerialName("to_platform_id") val toPlatformId: String? = null,
    val geometry: LineString? = null, @SerialName("duration_s") val durationS: Int,
    @SerialName("departure_at") val departureAt: String? = null, @SerialName("arrival_at") val arrivalAt: String? = null,
    val basis: String, val navigation: String
) {
    val label get() = when (kind) {
        "walk_to_stop" -> "Walk to boarding platform"
        "boarding_wait" -> "Boarding buffer and scheduled wait"
        "transit" -> "Scheduled Blue Line ride"
        "walk_from_stop" -> "Walk from alighting platform"
        else -> "Reviewed walking section"
    }
}
@Serializable data class ScheduledTransitPlan(
    val version: Int, @SerialName("tour_id") val tourId: String,
    @SerialName("revision_id") val revisionId: String? = null,
    @SerialName("connector_version") val connectorVersion: Int, val state: String,
    @SerialName("requested_departure_at") val requestedDepartureAt: String,
    @SerialName("departure_context_stop_id") val departureContextStopId: String,
    val service: TransitService, val segments: List<TransitSegment>, val assumptions: List<String>
) {
    fun valid(revision: TourRevision): Boolean {
        val tour = revision.snapshot?.tour ?: return false
        val choice = tour.approvedTransit ?: return false
        val requested = transitInstant(requestedDepartureAt) ?: return false
        if (!choice.valid(tour) || version != 1 || tourId != revision.tourId || revisionId != revision.id || connectorVersion != choice.version ||
            departureContextStopId != choice.intent.fromStopId || service.live || assumptions.size > 20 || assumptions.any { it.toByteArray().size > 4000 } ||
            state !in listOf("scheduled", "no_service", "stale", "provider_unavailable", "unavailable", "unsupported", "needs_review")) return false
        if (state != "scheduled") return segments.isEmpty() && service.state == state
        val departure = service.departureAt?.let(::transitInstant) ?: return false
        val arrival = service.arrivalAt?.let(::transitInstant) ?: return false
        val allowance = (choice.intent.walkToStopSeconds + choice.intent.boardingBufferSeconds).toLong()
        if (service.state != state || !service.scheduled || service.source?.valid() != true || service.board?.matches(choice.board) != true || service.alight?.matches(choice.alight) != true ||
            service.routeId != "Blue" || service.tripId.isNullOrBlank() || departure < requested.plusSeconds(allowance) || departure > requested.plusSeconds(allowance + 21600) ||
            arrival <= departure || arrival > departure.plusSeconds(10800)) return false
        val stops = tour.stops.sortedBy { it.position }
        var index = 0
        for (n in 0 until stops.lastIndex) {
            if (stops[n].id == choice.intent.fromStopId) {
                val kinds = listOf("walk_to_stop", "boarding_wait", "transit", "walk_from_stop")
                val bases = listOf("curator_walking_estimate", "schedule_and_curator_buffer", "cta_static_schedule", "curator_walking_estimate")
                val navigation = listOf("external_directions_required", "check_station_signs_and_alerts", "scheduled_rail", "external_directions_required")
                val times = listOf(requested, requested.plusSeconds(choice.intent.walkToStopSeconds.toLong()), departure, arrival, arrival.plusSeconds(choice.intent.walkFromStopSeconds.toLong()))
                for (k in 0..3) {
                    val s = segments.getOrNull(index++) ?: return false
                    val a = s.departureAt?.let(::transitInstant) ?: return false; val b = s.arrivalAt?.let(::transitInstant) ?: return false
                    if (s.kind != kinds[k] || s.basis != bases[k] || s.navigation != navigation[k] || s.geometry != null ||
                        s.fromStopId != (if (k == 0) choice.intent.fromStopId else null) || s.toStopId != (if (k == 3) choice.intent.toStopId else null) ||
                        s.fromPlatformId != (if (k == 0) null else if (k == 3) choice.alight.id else choice.board.id) ||
                        s.toPlatformId != (if (k == 3) null else if (k <= 1) choice.board.id else choice.alight.id) ||
                        abs(Duration.between(times[k], a).toMillis()) >= 10 || abs(Duration.between(times[k + 1], b).toMillis()) >= 10 ||
                        abs(Duration.between(a, b).toMillis() - s.durationS * 1000L) >= 1000) return false
                }
            } else {
                val leg = choice.walkingLegs.firstOrNull { it.fromPosition == stops[n].position && it.toPosition == stops[n + 1].position } ?: return false
                val s = segments.getOrNull(index++) ?: return false
                if (s.kind != "walk" || s.fromStopId != stops[n].id || s.toStopId != stops[n + 1].id || s.fromPlatformId != null || s.toPlatformId != null || s.departureAt != null || s.arrivalAt != null ||
                    s.geometry?.type != "LineString" || s.geometry.coordinates != leg.geometry || s.durationS != leg.durationS || s.basis != "retained_qualified_walking_route" || s.navigation != "in_app_walking") return false
            }
        }
        return index == segments.size
    }
    fun unavailableReason(now: Instant): String? {
        if (state != "scheduled") return "Timetable result: ${state.replace('_', ' ')}. Choose another time explicitly or wait for curator review."
        val source = service.source?.retrievedAt?.let(::transitInstant)
        if (source == null || source < now.minusSeconds(86400) || source > now.plusSeconds(300)) return "The retained timetable is stale. Review a fresh plan before using directions."
        val departure = service.departureAt?.let(::transitInstant)
        if (departure == null || departure <= now) return "The scheduled boarding time has passed. Byl does not know whether the train ran or you boarded."
        return null
    }
}
// Instant.parse only accepts a trailing Z before JDK 12, and Android 8–13 ship that parser. The API
// serializes CTA times with their Chicago offset, so parse any ISO-8601 offset explicitly.
fun parseInstant(text: String): Instant = OffsetDateTime.parse(text).toInstant()
fun transitInstant(text: String): Instant? = runCatching { parseInstant(text) }.getOrNull()
private fun validTransitLine(coordinates: List<List<Double>>) = coordinates.size >= 2 && coordinates.all { it.size == 2 && it[0].isFinite() && it[1].isFinite() && it[0] in -180.0..180.0 && it[1] in -90.0..90.0 }
