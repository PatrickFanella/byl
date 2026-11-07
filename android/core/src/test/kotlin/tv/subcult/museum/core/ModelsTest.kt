package tv.subcult.museum.core

import kotlinx.serialization.encodeToString
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ModelsTest {
    @Test
    fun `shared journey fixtures retain totals partial modes and legacy compatibility`() {
        val route: TourRoute = Fixtures.decode("journey-route-return")
        val tour: TourDetail = Fixtures.decode("journey-tour-partial")
        val journey = requireNotNull(route.journey)
        assertTrue(journey.usable)
        assertEquals(2904, journey.itineraryDurationS)
        assertEquals(2904, journey.roundTripDurationS)
        assertEquals(300, journey.returnPlan.durationS)
        assertEquals("asphalt", journey.segments.first().surface)
        assertNull(journey.segments.last().surface)
        assertEquals("failed", tour.journeys?.get("bike")?.state)
        assertNull(tour.journeys?.get("bike")?.travelDurationS)
        assertEquals(29, tour.routes["walk"]?.estimatedMinutes)
        assertEquals(route, MuseumJson.decodeFromString<TourRoute>(MuseumJson.encodeToString(route)))
        assertEquals(tour, MuseumJson.decodeFromString<TourDetail>(MuseumJson.encodeToString(tour)))
        for (name in listOf("journey-route-pending", "journey-route-failed")) {
            val unavailable: TourRoute = Fixtures.decode(name)
            assertNull(unavailable.journey?.itineraryDurationS)
            assertFalse(unavailable.journey?.usable ?: true)
        }
        val unreviewed: TourRoute = Fixtures.decode("journey-route-unreviewed")
        assertEquals("unreviewed", unreviewed.journey?.state)
        assertFalse(unreviewed.journey?.usable ?: true)
        assertNull(Fixtures.decode<TourRoute>("route").journey)
        assertNull(Fixtures.decode<TourDetail>("tour").journeys)
    }

    @Test
    fun `decodes the city with topics`() {
        val city: City = Fixtures.decode("city")
        assertEquals("downers-grove", city.slug)
        assertEquals(4, city.topics?.size)
        assertEquals(41.76, city.bbox.minLat, 0.001)
    }

    @Test
    fun `decodes the tour with stops and routes`() {
        val tour: TourDetail = Fixtures.decode("tour")
        assertEquals("downtown-memory-loop", tour.slug)
        assertEquals(4, tour.stops.size)
        assertEquals(listOf("walk", "bike"), tour.modes)
        assertEquals("ready", tour.routes["walk"]?.status)
        assertEquals(24, tour.routes["walk"]?.estimatedMinutes)
        val fishel = tour.stops[2]
        assertNull(fishel.featuredMomentId)
        assertEquals("Summer Concert Lawn", fishel.featuredMoment?.title)
        assertFalse(tour.isUserRoute)
    }

    @Test
    fun `decodes the tour list page`() {
        val page: ListPage<TourSummary> = Fixtures.decode("tours")
        assertEquals(1, page.items.size)
        assertEquals("", page.nextCursor)
    }

    @Test
    fun `decodes the route with geometry and legs`() {
        val route: TourRoute = Fixtures.decode("route")
        assertEquals("ready", route.status)
        assertEquals(4, route.geometry?.coordinates?.size)
        assertEquals(3, route.legs.size)
        assertEquals(1, route.legs[0].fromPosition)
        assertEquals(2, route.legs[0].toPosition)
    }

    @Test
    fun `decodes the place with its moments`() {
        val place: PlaceDetail = Fixtures.decode("place")
        assertEquals("downers-grove-museum", place.slug)
        assertEquals(1, place.moments.size)
        assertEquals(place.primaryMomentId, place.primaryMoment?.id)
        val near: ListPage<PlaceSummary> = Fixtures.decode("places_near")
        assertNotNull(near.items[0].distanceM)
    }

    @Test
    fun `decodes the moment with sources and topics`() {
        val moment: MomentDetail = Fixtures.decode("moment")
        assertEquals("Built for the Talkies", moment.title)
        assertEquals(2, moment.sources.size)
        assertEquals(2, moment.topics.size)
        assertTrue(moment.body.isNotEmpty())
        assertNull(moment.narration)
    }

    @Test
    fun `decodes the session and a user tour`() {
        val session: Session = Fixtures.decode("session")
        assertEquals("visitor", session.account.role)
        val mine: TourDetail = Fixtures.decode("user_tour")
        assertTrue(mine.isUserRoute)
        assertEquals("unlisted", mine.visibility)
        assertEquals(2, mine.stops.size)
        assertEquals(10, mine.slug.length)
        val progress: TourProgress = Fixtures.decode("progress")
        assertEquals(listOf(1, 2), progress.visitedPositions)
    }

    @Test
    fun `decodes the error envelope`() {
        val body: ApiErrorBody = Fixtures.decode("error_validation")
        assertEquals("validation_failed", body.error.code)
        assertEquals("must have between 2 and 30 stops", body.error.fields?.get("stops"))
    }

    @Test
    fun `round-trips the tour and route through JSON`() {
        val tour: TourDetail = Fixtures.decode("tour")
        assertEquals(tour, MuseumJson.decodeFromString<TourDetail>(MuseumJson.encodeToString(tour)))
        val route: TourRoute = Fixtures.decode("route")
        assertEquals(route, MuseumJson.decodeFromString<TourRoute>(MuseumJson.encodeToString(route)))
        val moment: MomentDetail = Fixtures.decode("moment")
        assertEquals(moment, MuseumJson.decodeFromString<MomentDetail>(MuseumJson.encodeToString(moment)))
    }
}
