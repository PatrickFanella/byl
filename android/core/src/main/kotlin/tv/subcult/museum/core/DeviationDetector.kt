package tv.subcult.museum.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Instant
import java.time.ZoneOffset
import kotlin.math.*

/** Raw fixes and points have no serialization contract. Only DeviationSummary may leave memory. */
data class DeviationContext(
    val revisionId: String, val segmentId: String, val mode: String, val direction: String,
    val reviewed: Boolean, val ambiguityReviewed: Boolean, val path: List<Point>,
    val competingPaths: List<List<Point>>, val stopAreas: List<StopArea>
) {
    data class StopArea(val center: Point, val radiusM: Double)
}

@Serializable
data class DeviationSummary(
    @SerialName("schema_version") val schemaVersion: Int = 1,
    @SerialName("detector_version") val detectorVersion: String = DeviationDetector.VERSION,
    val calibrated: Boolean = false,
    @SerialName("revision_id") val revisionId: String,
    @SerialName("segment_id") val segmentId: String,
    val mode: String, val direction: String,
    @SerialName("observation_period") val observationPeriod: String,
    @SerialName("qualified_passages") val qualifiedPassages: Int = 1,
    @SerialName("sustained_departure_rejoins") val sustainedDepartureRejoins: Int
)

/** Direct port of the uncalibrated reference: no IO, subscriptions, or retained trace. */
class DeviationDetector(private val context: DeviationContext) {
    enum class Control { PAUSE, RESUME, END, PERMISSION_LOST, MANUAL_ARRIVAL }
    private data class XY(val x: Double, val y: Double)
    private data class Projection(val distance: Double, val along: Double)
    private data class Outside(val first: Fix, val departureAlong: Double, var count: Int = 0, var qualified: Boolean = false)
    private val origin = context.path.firstOrNull() ?: Point(0.0, 0.0)
    private val path: List<XY>
    private val competing: List<List<XY>>
    private val length: Double
    val supported: Boolean
    private var paused = false
    private var ended = false
    private var reported = false
    private var armed = false
    private var deviated = false
    private var lastSeen = -1L
    private var previous: Fix? = null
    private var start: Fix? = null
    private var trusted: Projection? = null
    private var nearSamples = 0
    private var samples = 0
    private var uncertainSince: Long? = null
    private var outside: Outside? = null

    init {
        fun validPath(points: List<Point>) = points.size in 2..512 && points.all { valid(it) && abs(it.lat - origin.lat) <= .25 && abs(it.lng - origin.lng) <= .25 }
        val valid = context.reviewed && context.ambiguityReviewed && context.revisionId.isNotBlank() && context.segmentId.isNotBlank() &&
            context.mode in listOf("walk", "bike") && context.direction in listOf("forward", "reverse") && validPath(context.path) &&
            context.competingPaths.size <= 8 && context.competingPaths.all(::validPath) && context.stopAreas.size <= 32 &&
            context.stopAreas.all { valid(it.center) && it.radiusM.isFinite() && it.radiusM in 0.0..500.0 }
        path = if (valid) (if (context.direction == "reverse") context.path.reversed() else context.path).map(::xy) else emptyList()
        competing = if (valid) context.competingPaths.map { it.map(::xy) } else emptyList()
        length = path.zipWithNext().sumOf { (a, b) -> hypot(b.x - a.x, b.y - a.y) }
        supported = valid && length in 50.0..5000.0
    }

    fun control(control: Control) {
        clear(); previous = null
        if (control == Control.END) ended = true
        paused = control != Control.RESUME
    }

    fun push(fix: Fix): List<DeviationSummary> {
        if (!supported || paused || ended || reported) return emptyList()
        val time = fix.timestampMillis
        if (time < 0 || time > 253402300799999L || time <= lastSeen) return emptyList()
        lastSeen = time
        if (!valid(fix.point) || !fix.accuracyM.isFinite() || fix.accuracyM !in 0.0..20.0) {
            clear(); previous = null; return emptyList()
        }
        val accuracy = max(3.0, fix.accuracyM)
        val prior = previous; previous = fix
        if (prior != null) {
            val elapsed = time - prior.timestampMillis
            val movement = max(0.0, distanceM(prior.point, fix.point) - max(3.0, prior.accuracyM) - accuracy)
            if (elapsed > 20_000 || movement / (elapsed / 1000.0) > (if (context.mode == "walk") 4 else 15)) { clear(); return emptyList() }
        }
        start?.let { if (time - it.timestampMillis > 900_000 || day(fix) != day(it)) clear() }
        val projections = project(fix.point, path).sortedBy { it.distance }
        val closest = projections.firstOrNull() ?: run { clear(); return emptyList() }
        val ambiguous = projections.any { it.distance <= accuracy + 10 && abs(it.along - closest.along) > max(20.0, accuracy * 2) }
        val competitor = competing.any { line -> project(fix.point, line).any { it.distance <= accuracy + 10 } }
        val stopAccess = context.stopAreas.any { distanceM(it.center, fix.point) <= it.radiusM + accuracy }
        if (ambiguous || competitor || stopAccess || trusted?.let { closest.along < it.along - 30 } == true) { clear(); return emptyList() }
        val near = closest.distance + accuracy <= 20
        val far = closest.distance - accuracy >= 35
        if (!near && !far) {
            nearSamples = 0; uncertainSince = uncertainSince ?: time
            if (outside?.qualified == false) outside = null
            if (trusted == null || time - uncertainSince!! > 20_000) clear()
            return emptyList()
        }
        uncertainSince = null; samples++
        if (near) {
            nearSamples++
            if (nearSamples >= 2) armed = true
            if (start == null && closest.along <= length * .1) start = fix
            outside?.let { candidate ->
                if (nearSamples >= 2) {
                    if (candidate.qualified && closest.along - candidate.departureAlong >= 30) deviated = true
                    outside = null
                }
            }
            trusted = closest
            val first = start
            if (first != null && outside == null && nearSamples >= 2 && closest.along >= length * .9 && samples >= 5 &&
                time - first.timestampMillis >= 30_000 && distanceM(first.point, fix.point) - accuracy - max(3.0, first.accuracyM) >= 30) {
                reported = true
                val summary = DeviationSummary(revisionId = context.revisionId, segmentId = context.segmentId, mode = context.mode, direction = context.direction,
                    observationPeriod = day(fix), sustainedDepartureRejoins = if (deviated) 1 else 0)
                clear(); previous = null; lastSeen = -1L
                return listOf(summary)
            }
        } else {
            val trusted = trusted
            if (start == null || trusted == null || (outside == null && !armed)) { clear(); return emptyList() }
            val candidate = outside ?: Outside(fix, trusted.along).also { outside = it }
            nearSamples = 0; candidate.count++
            if (candidate.count >= 3 && time - candidate.first.timestampMillis >= 20_000 &&
                distanceM(candidate.first.point, fix.point) - accuracy - max(3.0, candidate.first.accuracyM) >= 30) candidate.qualified = true
        }
        return emptyList()
    }

    private fun clear() {
        start = null; trusted = null; nearSamples = 0; armed = false; uncertainSince = null; samples = 0; deviated = false; outside = null
    }
    private fun xy(point: Point) = XY(Math.toRadians(point.lng - origin.lng) * 6_371_000 * cos(Math.toRadians(origin.lat)), Math.toRadians(point.lat - origin.lat) * 6_371_000)
    private fun project(point: Point, line: List<XY>): List<Projection> {
        val p = xy(point); var along = 0.0
        return line.zipWithNext().mapNotNull { (a, b) ->
            val dx = b.x - a.x; val dy = b.y - a.y; val length = hypot(dx, dy)
            if (length == 0.0) return@mapNotNull null
            val t = (((p.x - a.x) * dx + (p.y - a.y) * dy) / (length * length)).coerceIn(0.0, 1.0)
            Projection(hypot(p.x - a.x - t * dx, p.y - a.y - t * dy), along + t * length).also { along += length }
        }
    }
    companion object {
        const val VERSION = "uncalibrated-v1"
        private fun valid(p: Point) = p.lat.isFinite() && p.lng.isFinite() && abs(p.lat) <= 85 && abs(p.lng) <= 180
        private fun day(fix: Fix) = Instant.ofEpochMilli(fix.timestampMillis).atZone(ZoneOffset.UTC).toLocalDate().toString()
    }
}
