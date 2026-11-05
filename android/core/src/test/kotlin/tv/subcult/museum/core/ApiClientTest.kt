package tv.subcult.museum.core

import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class ApiClientTest {
    private lateinit var server: MockWebServer

    @BeforeEach
    fun start() {
        server = MockWebServer()
        server.start()
    }

    @AfterEach
    fun stop() = server.shutdown()

    private fun client(token: String? = "tok") = ApiClient(server.url("/").toString().trimEnd('/')) { token }

    @Test
    fun `sends the bearer token and decodes a page`() = runTest {
        server.enqueue(MockResponse().setBody(Fixtures.text("tours")).setHeader("Content-Type", "application/json"))
        val tours = Endpoints(client()).tours("downers-grove", mode = "walk")
        assertEquals(1, tours.size)
        val req = server.takeRequest()
        assertEquals("Bearer tok", req.getHeader("Authorization"))
        assertEquals("/api/v1/cities/downers-grove/tours?mode=walk", req.path)
    }

    @Test
    fun `a 401 calls the hook with the token used and throws unauthorized`() = runTest {
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error":{"code":"unauthorized","message":"sign in required"}}"""))
        val c = client("old-token")
        val rejected = mutableListOf<String?>()
        c.onUnauthorized = { rejected.add(it) }
        val e = assertThrows<ApiException> { Endpoints(c).me() }
        assertEquals(401, e.status)
        assertEquals("sign in required", e.message)
        assertTrue(e.isUnauthorized)
        assertEquals(listOf("old-token"), rejected)
    }

    @Test
    fun `a 422 carries field messages and does not call the hook`() = runTest {
        server.enqueue(MockResponse().setResponseCode(422).setBody(Fixtures.text("error_validation")))
        val c = client()
        var hookCalls = 0
        c.onUnauthorized = { hookCalls++ }
        val e = assertThrows<ApiException> {
            Endpoints(c).createTour(CreateUserTourRequest(city = "downers-grove", title = "x", stops = emptyList()))
        }
        assertEquals(422, e.status)
        assertEquals("must have between 2 and 30 stops", e.fields["stops"])
        assertEquals(0, hookCalls)
        assertEquals("POST", server.takeRequest().method)
    }

    @Test
    fun `a 204 completes a delete and a PUT body is JSON`() = runTest {
        server.enqueue(MockResponse().setResponseCode(204))
        server.enqueue(MockResponse().setBody(Fixtures.text("progress")))
        val api = Endpoints(client())
        api.unsave("t1")
        assertEquals("DELETE", server.takeRequest().method)
        val progress = api.putProgress("t1", ProgressRequest("walk", listOf(1), false))
        assertEquals(listOf(1, 2), progress.visitedPositions)
        val put = server.takeRequest()
        assertEquals("PUT", put.method)
        assertEquals("""{"mode":"walk","visited_positions":[1],"completed":false}""", put.body.readUtf8())
    }

    @Test
    fun `a non JSON error body still maps to an exception`() = runTest {
        server.enqueue(MockResponse().setResponseCode(502).setBody("bad gateway"))
        val e = assertThrows<ApiException> { Endpoints(client()).cities() }
        assertEquals(502, e.status)
        assertEquals("http_error", e.code)
        assertNull(e.fields["x"])
    }
}
