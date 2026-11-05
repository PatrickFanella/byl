package tv.subcult.museum.core

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class RouteBuilderTest {
    private lateinit var server: MockWebServer
    private val city: City = Fixtures.decode("city")
    private val places: List<PlaceSummary> = Fixtures.decode<ListPage<PlaceSummary>>("places_near").items

    @BeforeEach
    fun start() { server = MockWebServer(); server.start() }

    @AfterEach
    fun stop() = server.shutdown()

    private fun api() = Endpoints(ApiClient(server.url("/").toString().trimEnd('/')) { "tok" })

    @Test
    fun `create stays disabled under two stops and without a title or mode`() {
        val b = RouteBuilder(city)
        b.title = "Depot loop"
        assertFalse(b.state.value.canSubmit)
        b.add(places[0])
        assertFalse(b.state.value.canSubmit)
        assertEquals("Add at least 2 stops (1 so far)", b.state.value.stopsHint)
        b.add(places[1])
        assertTrue(b.state.value.canSubmit)
        b.title = "   "
        assertFalse(b.state.value.canSubmit)
        b.title = "Depot loop"
        b.walk = false
        b.bike = false
        assertFalse(b.state.value.canSubmit)
    }

    @Test
    fun `duplicates are ignored and thirty is the cap`() {
        val b = RouteBuilder(city)
        b.add(places[0]); b.add(places[0])
        assertEquals(1, b.state.value.stops.size)
        for (i in 0 until 40) b.add(places[1].copy(id = "p-$i"))
        assertEquals(RouteBuilder.MAX_STOPS, b.state.value.stops.size)
        assertEquals("30 stops is the maximum", b.state.value.stopsHint)
    }

    @Test
    fun `stops from another city block submit`() {
        val b = RouteBuilder(city)
        b.title = "Two towns"
        b.add(places[0])
        val foreign = places[1].copy(id = "foreign", cityId = "00000000-0000-4000-8000-0000000000c2")
        b.add(foreign)
        assertFalse(b.state.value.canSubmit)
        assertEquals("Every stop must be in ${city.name}", b.state.value.stopsHint)
        b.remove("foreign")
        b.add(places[1])
        assertTrue(b.state.value.canSubmit)
    }

    @Test
    fun `reorder and remove`() {
        val b = RouteBuilder(city)
        b.add(places[0]); b.add(places[1])
        b.move(1, 0)
        assertEquals(listOf(places[1].id, places[0].id), b.state.value.stops.map { it.id })
        b.remove(places[1].id)
        assertEquals(listOf(places[0].id), b.state.value.stops.map { it.id })
        assertTrue(b.contains(places[0]))
    }

    @Test
    fun `submit posts the route and returns it`() = runTest {
        server.enqueue(MockResponse().setResponseCode(201).setBody(Fixtures.text("user_tour")))
        val b = RouteBuilder(city)
        b.title = " Depot loop "
        b.summary = "Two stops"
        b.bike = false
        b.visibility = "private"
        b.add(places[0]); b.add(places[1])
        val created = b.submit(api())
        assertNotNull(created)
        assertTrue(created!!.isUserRoute)
        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/api/v1/me/tours", req.path)
        val body = Json.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals(city.slug, body["city"]!!.jsonPrimitive.content)
        assertEquals("Depot loop", body["title"]!!.jsonPrimitive.content)
        assertEquals(listOf("walk"), body["modes"]!!.jsonArray.map { it.jsonPrimitive.content })
        assertEquals("private", body["visibility"]!!.jsonPrimitive.content)
        assertEquals(listOf(places[0].id, places[1].id), body["stops"]!!.jsonArray.map { it.jsonObject["place_id"]!!.jsonPrimitive.content })
        assertNull(b.state.value.error)
    }

    @Test
    fun `validation fields surface on stops`() = runTest {
        server.enqueue(MockResponse().setResponseCode(422).setBody(Fixtures.text("error_validation")))
        val b = RouteBuilder(city)
        b.title = "x"
        b.add(places[0]); b.add(places[1])
        assertNull(b.submit(api()))
        assertEquals("must have between 2 and 30 stops", b.state.value.stopsError)
        assertEquals("request failed validation", b.state.value.error)
        assertFalse(b.state.value.busy)
    }

    @Test
    fun `editing prefills and patches`() = runTest {
        val mine: TourDetail = Fixtures.decode("user_tour")
        server.enqueue(MockResponse().setBody(Fixtures.text("user_tour")))
        val b = RouteBuilder(city, mine)
        assertTrue(b.isEditing)
        assertEquals(mine.title, b.title)
        assertEquals(mine.stops.sortedBy { it.position }.map { it.place.id }, b.state.value.stops.map { it.id })
        b.title = "Renamed"
        assertNotNull(b.submit(api()))
        val req = server.takeRequest()
        assertEquals("PATCH", req.method)
        assertEquals("/api/v1/me/tours/${mine.id}", req.path)
        val body = Json.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals("Renamed", body["title"]!!.jsonPrimitive.content)
        assertEquals(mine.stops.size, body["stops"]!!.jsonArray.size)
    }

    @Test
    fun `share URL is built from the base and slug`() {
        assertEquals("https://museum-api.subcult.tv/t/abc123XYZ0", shareUrl("https://museum-api.subcult.tv/", "abc123XYZ0"))
    }
}
