package tv.subcult.museum.core

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import java.time.Instant
import java.util.concurrent.TimeUnit

class HappeningsTest {
    @Serializable private data class Fixture(val now: String, val cases: List<Example>)
    @Serializable private data class Example(val name: String, val response: ListPage<Happening>, val expected_statuses: List<String>)

    @Test fun sharedResponseStatesAndClockFallbacks() {
        val fixture = Fixtures.decode<Fixture>("happenings-v1")
        val now = Instant.parse(fixture.now)
        assertEquals(17, fixture.cases.size)
        fixture.cases.forEach { example ->
            assertEquals(example.expected_statuses, example.response.items.map { it.presentationStatus(now).wire }, example.name)
        }
        val notice = fixture.cases.first().response.items.first()
        assertEquals("Nov 8, 2026 · America/Chicago · dates only", notice.occurrenceText)
        assertEquals("Last source check: 2026-11-08 15:00 UTC", notice.checkedText)
        assertEquals(HappeningStatus.EXPIRED, notice.presentationStatus(Instant.parse("2026-11-10T06:00:00Z")))
        assertNull(Happening.sourceURL("javascript:alert(1)"))
        assertNull(Happening.sourceURL("https://user:secret@example.com/"))
        assertNotNull(Happening.sourceURL("https://organizer.example/announcement"))
    }

    @Test fun publicOptionalEndpointIsBoundedAndAnonymous() = runTest {
        val server = MockWebServer(); server.start()
        try {
            val response = MuseumJson.parseToJsonElement(Fixtures.text("happenings-v1")).jsonObject.getValue("cases").jsonArray.first().jsonObject.getValue("response")
            server.enqueue(MockResponse().setBody(response.toString()))
            val api = Endpoints(ApiClient(server.url("/").toString()) { null })
            assertEquals(1, api.happenings("place-id", "20").items.size)
            val request = requireNotNull(server.takeRequest(5, TimeUnit.SECONDS))
            assertEquals("GET", request.method)
            assertEquals("/api/v1/places/place-id/happenings", request.requestUrl!!.encodedPath)
            assertEquals("20", request.requestUrl!!.queryParameter("limit"))
            assertEquals("20", request.requestUrl!!.queryParameter("cursor"))
            assertNull(request.getHeader("Authorization"))
            server.enqueue(MockResponse().setResponseCode(404).setBody(Fixtures.text("error_not_found")))
            try { api.happenings("place-id"); fail<Unit>("Expected optional endpoint failure") }
            catch (error: ApiException) { assertEquals(404, error.status) }
        } finally { server.shutdown() }
    }
}
