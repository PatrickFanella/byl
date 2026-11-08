package tv.subcult.museum.core

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import java.time.Instant
import kotlin.math.*

class DeviationDetectorTest {
    private val epoch = Instant.parse("2026-11-07T12:00:00Z").toEpochMilli()
    private fun point(x: Double, y: Double = 0.0) = Point(41.8 + y / 6_371_000 * 180 / PI, -88 + x / (6_371_000 * cos(41.8 * PI / 180)) * 180 / PI)
    private fun context(path: List<Point> = listOf(point(0.0), point(400.0))) = DeviationContext("reviewed-revision", "reviewed-segment", "walk", "forward", true, true, path, emptyList(), emptyList())
    private fun fix(seconds: Long, x: Double, time: Long = epoch): Fix { val p = point(x); return Fix(p.lat, p.lng, 3.0, time + seconds * 1000) }

    @Test fun `all shared fixtures match the reference exactly`() {
        val fixture = MuseumJson.parseToJsonElement(Fixtures.text("deviation-v1")).jsonObject
        val epoch = Instant.parse(fixture.getValue("epoch").jsonPrimitive.content).toEpochMilli()
        val cases = fixture.getValue("cases").jsonArray
        assertEquals(26, cases.size)
        fun points(value: JsonElement) = value.jsonArray.map { p -> val a = p.jsonArray; point(a[0].jsonPrimitive.double, a[1].jsonPrimitive.double) }
        for (row in cases.map { it.jsonObject }) {
            val name = row.getValue("name").jsonPrimitive.content
            val context = context().copy(
                mode = row.getValue("mode").jsonPrimitive.content, direction = row.getValue("direction").jsonPrimitive.content,
                reviewed = row.getValue("eligible").jsonPrimitive.boolean, ambiguityReviewed = row.getValue("ambiguity_reviewed").jsonPrimitive.boolean,
                path = points(row.getValue("path_m")), competingPaths = row.getValue("competing_paths_m").jsonArray.map(::points),
                stopAreas = row.getValue("stop_areas_m").jsonArray.map { val a = it.jsonArray; DeviationContext.StopArea(point(a[0].jsonPrimitive.double, a[1].jsonPrimitive.double), a[2].jsonPrimitive.double) })
            val detector = DeviationDetector(context)
            val reports = mutableListOf<DeviationSummary>()
            for (sample in row.getValue("samples").jsonArray) {
                if (sample is JsonArray) {
                    val p = point(sample[1].jsonPrimitive.double, sample[2].jsonPrimitive.double)
                    reports += detector.push(Fix(p.lat, p.lng, sample[3].jsonPrimitive.double, epoch + (sample[0].jsonPrimitive.double * 1000).toLong()))
                } else {
                    val control = when (sample.jsonObject.getValue("action").jsonPrimitive.content) {
                        "pause" -> DeviationDetector.Control.PAUSE
                        "resume" -> DeviationDetector.Control.RESUME
                        "end" -> DeviationDetector.Control.END
                        "permission_lost" -> DeviationDetector.Control.PERMISSION_LOST
                        "manual_arrival" -> DeviationDetector.Control.MANUAL_ARRIVAL
                        else -> error("Unknown control")
                    }
                    detector.control(control)
                }
            }
            val expected = row["expected_deviation"]?.jsonPrimitive?.booleanOrNull
            assertEquals(if (expected == null) 0 else 1, reports.size, name)
            if (expected != null) assertEquals(DeviationSummary(revisionId = context.revisionId, segmentId = context.segmentId, mode = context.mode, direction = context.direction, observationPeriod = "2026-11-07", sustainedDepartureRejoins = if (expected) 1 else 0), reports.single(), name)
        }
    }

    @Test fun `only minimal summary is serializable and repeated passages are latched`() {
        val detector = DeviationDetector(context())
        val summary = (0..20).flatMap { detector.push(fix(it * 10L, it * 20.0)) }.single()
        assertEquals(setOf("schema_version", "detector_version", "calibrated", "revision_id", "segment_id", "mode", "direction", "observation_period", "qualified_passages", "sustained_departure_rejoins"), MuseumJson.parseToJsonElement(MuseumJson.encodeToString(summary)).jsonObject.keys)
        assertTrue((0..20).flatMap { detector.push(fix(300 + it * 10L, it * 20.0)) }.isEmpty())
    }

    @Test fun `invalid geometry and timestamps fail closed`() {
        for (path in listOf(emptyList(), listOf(point(0.0)), listOf(point(0.0), point(1.0)), listOf(point(0.0), Point(Double.NaN, 0.0)))) assertFalse(DeviationDetector(context(path)).supported)
        val detector = DeviationDetector(context())
        for (time in listOf(-1L, Long.MAX_VALUE, 253402300800000L)) assertTrue(detector.push(fix(0, 0.0, time)).isEmpty())
    }

    @Test fun `UTC day and episode expiry cannot bridge evidence and end is terminal`() {
        val midnight = Instant.parse("2026-11-07T23:59:00Z").toEpochMilli()
        val detector = DeviationDetector(context())
        assertTrue((0..20).flatMap { detector.push(fix(it * 10L, it * 20.0, midnight)) }.isEmpty())
        val expired = DeviationDetector(context())
        for (i in 0..95) expired.push(fix(i * 10L, min(i * 20.0, 80.0)))
        assertTrue((5..20).flatMap { expired.push(fix(960 + (it - 5) * 10L, it * 20.0)) }.isEmpty())
        expired.control(DeviationDetector.Control.END); expired.control(DeviationDetector.Control.RESUME)
        assertTrue((0..20).flatMap { expired.push(fix(1500 + it * 10L, it * 20.0)) }.isEmpty())
    }
}
