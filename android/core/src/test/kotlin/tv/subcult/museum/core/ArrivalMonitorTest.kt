package tv.subcult.museum.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import kotlin.math.PI
import kotlin.math.cos

class ArrivalMonitorTest {
    private val station = Point(41.796, -88.0093)

    /** A fix `meters` east of the station with the given accuracy. */
    private fun fix(meters: Double, accuracy: Double) =
        Fix(station.lat, station.lng + meters / (111_320 * cos(station.lat * PI / 180)), accuracy)

    @Test
    fun `inside the radius with a good fix arrives`() = assertTrue(hasArrived(fix(20.0, 10.0), station, 40.0))

    @Test
    fun `a poor fix never arrives`() = assertFalse(hasArrived(fix(5.0, 200.0), station, 40.0))

    @Test
    fun `outside the radius does not arrive`() = assertFalse(hasArrived(fix(120.0, 10.0), station, 40.0))

    @Test
    fun `the distance may use up the accuracy margin`() {
        assertTrue(hasArrived(fix(50.0, 15.0), station, 40.0))
        assertFalse(hasArrived(fix(60.0, 15.0), station, 40.0))
    }

    @Test
    fun `invalid accuracy never arrives and 65 m is the ceiling`() {
        assertFalse(hasArrived(fix(0.0, -1.0), station, 40.0))
        assertTrue(hasArrived(fix(0.0, 65.0), station, 40.0))
        assertFalse(hasArrived(fix(0.0, 66.0), station, 40.0))
    }

    @Test
    fun `fires once per stop until retargeted`() {
        val m = ArrivalMonitor(station, 40.0)
        assertTrue(m.update(fix(10.0, 10.0)))
        assertFalse(m.update(fix(12.0, 10.0)))
        assertFalse(m.update(fix(300.0, 10.0)))
        assertFalse(m.update(fix(8.0, 10.0)))
        assertTrue(m.hasArrived)
        val tivoli = Point(41.7943, -88.0107)
        m.retarget(tivoli, 30.0)
        assertFalse(m.hasArrived)
        assertTrue(m.update(Fix(tivoli.lat, tivoli.lng, 8.0)))
    }

    @Test
    fun `exposes the distance to the stop`() {
        val m = ArrivalMonitor(station, 40.0)
        assertEquals(100.0, m.distance(fix(100.0, 10.0)), 2.0)
        assertEquals(0.0, distanceM(station, station), 0.0001)
    }
}
