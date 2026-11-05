package tv.subcult.museum.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.int
import kotlinx.serialization.json.boolean
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class ActiveTourTest {
    private lateinit var server: MockWebServer
    private val tourId = "00000000-0000-4000-8000-00000000e0e1"
    private val tour: TourDetail = Fixtures.decode("tour")
    private val store = InMemoryKeyValueStore()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val puts = java.util.concurrent.CopyOnWriteArrayList<String>()
    @Volatile private var routeStatus = 200
    @Volatile private var putStatus = 200
    @Volatile private var remoteProgress = false

    @BeforeEach
    fun start() {
        server = MockWebServer()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.path ?: ""
                return when {
                    request.method == "GET" && path.startsWith("/api/v1/tours/$tourId/route") ->
                        if (routeStatus == 200) MockResponse().setBody(Fixtures.text("route"))
                        else MockResponse().setResponseCode(routeStatus).setBody("""{"error":{"code":"internal","message":"down"}}""")
                    request.method == "GET" && path == "/api/v1/me/progress" ->
                        MockResponse().setBody(if (remoteProgress) """{"items":[${Fixtures.text("progress")}],"next_cursor":""}""" else """{"items":[],"next_cursor":""}""")
                    request.method == "PUT" && path == "/api/v1/me/progress/$tourId" -> {
                        if (putStatus == 200) {
                            puts.add(request.body.readUtf8())
                            MockResponse().setBody(Fixtures.text("progress"))
                        } else MockResponse().setResponseCode(putStatus).setBody("""{"error":{"code":"internal","message":"down"}}""")
                    }
                    else -> MockResponse().setResponseCode(404).setBody(Fixtures.text("error_not_found"))
                }
            }
        }
        server.start()
    }

    @AfterEach
    fun stop() = server.shutdown()

    private fun api(token: String? = "tok") = Endpoints(ApiClient(server.url("/").toString().trimEnd('/')) { token })
    private fun model(signedIn: Boolean) = ActiveTour(api(if (signedIn) "tok" else null), tour, "walk", store, { signedIn }, scope)
    private fun fix(stop: TourStop, accuracy: Double = 10.0) = Fix(stop.place.latitude, stop.place.longitude, accuracy)
    private fun visitedOf(body: String) = Json.parseToJsonElement(body).jsonObject["visited_positions"]!!.jsonArray.map { it.jsonPrimitive.int }
    private fun completedOf(body: String) = Json.parseToJsonElement(body).jsonObject["completed"]!!.jsonPrimitive.boolean

    @Test
    fun `next stop advances on arrival`() = runTest {
        val m = model(false)
        assertEquals(1, m.state.value.nextStop?.position)
        assertEquals("0 of 4 stops", m.progressText)
        m.handleFix(fix(tour.stops[0]))
        m.flush()
        assertEquals(setOf(1), m.state.value.visited)
        assertEquals(2, m.state.value.nextStop?.position)
        assertEquals(1, m.state.value.arrivedStop?.position)
        assertEquals("1 of 4 stops", m.progressText)
        assertFalse(m.state.value.isComplete)
    }

    @Test
    fun `a poor fix does not advance but updates the distance`() = runTest {
        val m = model(false)
        m.handleFix(fix(tour.stops[0], accuracy = 200.0))
        assertEquals(1, m.state.value.nextStop?.position)
        assertTrue(m.state.value.visited.isEmpty())
        m.handleFix(fix(tour.stops[1]))
        assertTrue((m.state.value.distanceToNextM ?: 0.0) > 50)
        assertTrue(m.state.value.visited.isEmpty())
    }

    @Test
    fun `progress persists locally and posts when signed in`() = runTest {
        val m = model(true)
        m.handleFix(fix(tour.stops[0]))
        m.flush()
        assertEquals(1, puts.size)
        assertEquals(listOf(1), visitedOf(puts[0]))
        assertFalse(completedOf(puts[0]))
        val again = model(true)
        assertEquals(setOf(1), again.state.value.visited)
        assertEquals(2, again.state.value.nextStop?.position)
    }

    @Test
    fun `progress stays local when signed out`() = runTest {
        val m = model(false)
        m.handleFix(fix(tour.stops[0]))
        m.flush()
        assertTrue(puts.isEmpty())
        assertNotNull(store.get(LocalProgress.key(tourId, "walk")))
    }

    @Test
    fun `completion posts completed true`() = runTest {
        val m = model(true)
        for (s in tour.stops) { m.handleFix(fix(s)); m.flush() }
        assertTrue(m.state.value.isComplete)
        assertNull(m.state.value.nextStop)
        assertEquals(listOf(1, 2, 3, 4), visitedOf(puts.last()))
        assertTrue(completedOf(puts.last()))
    }

    @Test
    fun `manual arrival works without a fix`() = runTest {
        val m = model(false)
        m.markArrived()
        assertEquals(setOf(1), m.state.value.visited)
        assertEquals(2, m.state.value.nextStop?.position)
    }

    @Test
    fun `legs split into past active and future`() = runTest {
        val m = model(false)
        m.loadRoute()
        assertEquals(LoadStatus.READY, m.state.value.routeStatus)
        assertNull(m.state.value.activeLeg)
        assertEquals(listOf(2, 3, 4), m.state.value.futureLegs.map { it.toPosition })
        m.handleFix(fix(tour.stops[0]))
        assertEquals(1, m.state.value.activeLeg?.fromPosition)
        assertEquals(2, m.state.value.activeLeg?.toPosition)
        assertEquals(listOf(3, 4), m.state.value.futureLegs.map { it.toPosition })
        assertTrue(m.state.value.pastLegs.isEmpty())
        m.handleFix(fix(tour.stops[1]))
        assertEquals(listOf(2), m.state.value.pastLegs.map { it.toPosition })
        assertEquals(3, m.state.value.activeLeg?.toPosition)
    }

    @Test
    fun `the route falls back to the cache when the API is down`() = runTest {
        model(false).loadRoute()
        routeStatus = 500
        val offline = model(false)
        offline.loadRoute()
        assertEquals(LoadStatus.READY, offline.state.value.routeStatus)
        assertTrue(offline.state.value.routeFromCache)
        assertEquals(3, offline.state.value.route?.legs?.size)
    }

    @Test
    fun `a route failure without cache is reported`() = runTest {
        routeStatus = 500
        val m = model(false)
        m.loadRoute()
        assertEquals(LoadStatus.ERROR, m.state.value.routeStatus)
        assertEquals("down", m.state.value.routeMessage)
        assertNull(m.state.value.route)
    }

    @Test
    fun `server progress is merged when the tour starts`() = runTest {
        remoteProgress = true
        val m = model(true)
        m.loadRemoteProgress()
        m.flush()
        assertEquals(setOf(1, 2), m.state.value.visited)
        assertEquals(3, m.state.value.nextStop?.position)
        assertFalse(m.needsSync)
        assertTrue(puts.isEmpty())
    }

    @Test
    fun `local stops ahead of the server are pushed after the merge`() = runTest {
        remoteProgress = true
        val m = model(true)
        repeat(3) { m.markArrived(); m.flush() }
        m.loadRemoteProgress()
        m.flush()
        assertEquals(listOf(1, 2, 3), visitedOf(puts.last()))
    }

    @Test
    fun `stale progress re-syncs when the server is back`() = runTest {
        putStatus = 500
        val m = model(true)
        m.markArrived()
        m.flush()
        assertEquals("down", m.state.value.syncError)
        assertTrue(m.needsSync)
        putStatus = 200
        m.syncIfNeeded()
        assertNull(m.state.value.syncError)
        assertFalse(m.needsSync)
        assertEquals(1, puts.size)
        m.syncIfNeeded()
        assertEquals(1, puts.size)
    }

    @Test
    fun `progress PUTs are sent in order and carry every stop known`() = runTest {
        val m = model(true)
        m.markArrived()
        m.markArrived()
        m.flush()
        assertEquals(2, puts.size)
        assertEquals(listOf(1, 2), visitedOf(puts.last()))
        assertTrue(visitedOf(puts.first()).all { it in visitedOf(puts.last()) })
    }

    @Test
    fun `the active tour is restored from the store after a relaunch`() = runTest {
        val first = model(false)
        first.markArrived()
        val restored = ActiveTour.restore(api(null), store, { false }, scope)
        assertEquals(tourId, restored?.tour?.id)
        assertEquals("walk", restored?.mode)
        assertEquals(setOf(1), restored?.state?.value?.visited)
        assertEquals(2, restored?.state?.value?.nextStop?.position)
        first.end()
        assertNull(ActiveTour.restore(api(null), store, { false }, scope))
    }

    @Test
    fun `resetting a signed-in walk clears the server record`() = runTest {
        val m = model(true)
        m.markArrived()
        m.flush()
        m.resetProgress()
        m.flush()
        assertEquals(emptyList<Int>(), visitedOf(puts.last()))
        assertTrue(m.state.value.visited.isEmpty())
        assertEquals(1, m.state.value.nextStop?.position)
    }

    @Test
    fun `navigation URI points at the next stop`() = runTest {
        val m = model(false)
        assertEquals("google.navigation:q=${tour.stops[0].place.latitude},${tour.stops[0].place.longitude}&mode=w", m.navigationUri())
    }

    @Test
    fun `an arrival is announced once and acknowledged by the UI`() = runTest {
        val m = model(false)
        m.markArrived()
        assertEquals(1, m.state.value.arrivedStop?.position)
        m.acknowledgeArrival()
        assertNull(m.state.value.arrivedStop)
        assertEquals(2, m.state.value.nextStop?.position, "acknowledging does not move the walk")
        m.markArrived()
        assertEquals(2, m.state.value.arrivedStop?.position)
    }
}
