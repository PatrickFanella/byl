package tv.subcult.museum.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.*

@Serializable
internal data class ReviewedSignalReference(
    val version: Int, @SerialName("ambiguity_reviewed") val ambiguityReviewed: Boolean,
    @SerialName("competing_paths") val competingPaths: List<LineString>,
    @SerialName("stop_areas") val stopAreas: List<StopArea>, val source: JourneySource
) {
    @Serializable data class StopArea(val latitude: Double, val longitude: Double, @SerialName("radius_m") val radiusM: Double)
}

/** Optional observation metadata may fail without invalidating the navigation snapshot. */
fun contributionContexts(revision: TourRevision?, mode: String): List<DeviationContext> {
    val snapshot = revision?.snapshot ?: return emptyList()
    val tour = snapshot.tour
    val journey = snapshot.routes[mode]?.journey ?: return emptyList()
    if (tour.transit != null && tour.transit != JsonNull || mode !in listOf("walk", "bike") || mode !in tour.modes ||
        tour.id != revision.tourId || (tour.revisionId != null && tour.revisionId != revision.id) ||
        journey.version != 1 || !journey.usable || journey.state != "usable" || journey.quality != "pass" || journey.segments.any { it.kind == "transit" }) return emptyList()
    val intent = tour.corridor?.intent ?: return emptyList()
    val selected = intent.routes.firstOrNull { it.mode == mode } ?: return emptyList()
    if (selected.path.size > 64) return emptyList()
    val storyAreas = tour.stops.map { stop ->
        DeviationContext.StopArea(stop.place.arrivalPoint?.navigationPoint(mode) ?: Point(stop.place.latitude, stop.place.longitude), stop.place.arrivalRadiusM)
    }
    fun points(line: LineString): List<Point>? = if (line.type == "LineString" && line.coordinates.size in 2..512 && line.coordinates.all { it.size == 2 }) line.coordinates.map { Point(it[1], it[0]) } else null
    return selected.path.mapNotNull { traversal ->
        runCatching {
            val segment = intent.segments.firstOrNull { it.id == traversal.segmentId } ?: return@mapNotNull null
            val ref = segment.signalReferences?.jsonObject?.get(mode)?.let { MuseumJson.decodeFromJsonElement<ReviewedSignalReference>(it) } ?: return@mapNotNull null
            val geometry = segment.geometry?.let { MuseumJson.decodeFromJsonElement<LineString>(it) } ?: return@mapNotNull null
            if (ref.version != 1 || !ref.ambiguityReviewed || ref.source.version.isBlank() || ref.competingPaths.size > 8 || ref.stopAreas.size + storyAreas.size > 32 ||
                journey.segments.none { it.id == segment.id && it.reverse == traversal.reverse && it.access == "allowed" }) return@mapNotNull null
            val path = points(geometry) ?: return@mapNotNull null
            val competing = ref.competingPaths.map { points(it) ?: return@mapNotNull null }
            val context = DeviationContext(revision.id, segment.id, mode, if (traversal.reverse) "reverse" else "forward", true, true, path, competing,
                storyAreas + ref.stopAreas.map { DeviationContext.StopArea(Point(it.latitude, it.longitude), it.radiusM) })
            context.takeIf { DeviationDetector(it).supported }
        }.getOrNull()
    }.distinctBy { it.segmentId to it.direction }
}
