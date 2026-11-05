package tv.subcult.museum.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class EstimatesTest {
    private fun route(mode: String = "walk", status: String = "ready", durationS: Int = 1404, estimated: Int? = 24) =
        RouteSummary(mode = mode, status = status, distanceM = 1824.68, durationS = durationS, estimatedMinutes = estimated)

    @Test
    fun `reads the server estimate for a ready route`() {
        assertEquals("24 min walk", estimateText(route()))
        assertEquals("7 min ride", estimateText(route(mode = "bike", durationS = 405, estimated = 7)))
    }

    @Test
    fun `falls back to the duration when there is no estimate`() {
        assertEquals("11 min walk", estimateText(route(durationS = 650, estimated = null)))
    }

    @Test
    fun `is null unless the route is ready`() {
        assertNull(estimateText(route(status = "pending")))
        assertNull(estimateText(null))
    }

    @Test
    fun `formats distances`() {
        assertEquals("850 m", distanceText(850.0))
        assertEquals("1.8 km", distanceText(1824.68))
        assertEquals("ride", modeVerb("bike"))
        assertEquals("walk", modeVerb("walk"))
    }
}
