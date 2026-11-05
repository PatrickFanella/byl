package tv.subcult.museum.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class DirectionsTest {
    @Test
    fun `builds the navigation URI per mode`() {
        assertEquals("google.navigation:q=41.796,-88.0093&mode=b", navigationUri(41.796, -88.0093, "bike"))
        assertEquals("google.navigation:q=41.796,-88.0093&mode=w", navigationUri(41.796, -88.0093, "walk"))
    }

    @Test
    fun `builds a geo URI with the place name`() {
        assertEquals("geo:41.796,-88.0093?q=41.796,-88.0093(Main%20Street%20Station)", geoUri(41.796, -88.0093, "Main Street Station"))
    }
}
