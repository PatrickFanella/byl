package tv.subcult.museum.core

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okhttp3.mockwebserver.SocketPolicy
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import java.io.File
import java.io.IOException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

class RouteSuggestionsTest {
    private lateinit var server: MockWebServer
    private val fixture: RouteSuggestion = Fixtures.decode("route_suggestion")
    private val owner get() = fixture.ownerAccountId
    private val http = OkHttpClient.Builder().retryOnConnectionFailure(false).callTimeout(3, TimeUnit.SECONDS).build()

    @BeforeEach
    fun start() { server = MockWebServer(); server.start() }

    @AfterEach
    fun stop() = server.shutdown()

    private fun api(token: String = "fixture-owner-token", base: String = server.url("/").toString()) =
        Endpoints(ApiClient(base, http) { token })

    private fun flow(store: KeyValueStore = InMemoryKeyValueStore(), account: String = owner, current: () -> Boolean = { true }) =
        RouteSuggestions(api(), store, account, current)

    private suspend fun open(flow: RouteSuggestions) {
        server.enqueue(MockResponse().setBody("""{"enabled":true}"""))
        flow.loadIntake()
        assertEquals("/api/v1/route-brief-intake", request().path)
    }

    private fun request(): RecordedRequest = server.takeRequest(3, TimeUnit.SECONDS) ?: error("Expected a synthetic API request")
    private fun response(record: RouteSuggestion = fixture, code: Int = 200) =
        MockResponse().setResponseCode(code).setBody(MuseumJson.encodeToString(record))

    @Test
    fun sharedDtoFixtureMatchesPrivateContract() {
        assertEquals(File("../../backend/api/fixtures/route-briefs-v1/brief-response.json").readText(), Fixtures.text("route_suggestion"))
        assertEquals("Rail workers", fixture.submitted.topic)
        assertEquals(listOf("Old depot, address unknown"), fixture.submitted.placeLeads)
        assertEquals("More research needed", suggestionStatus(fixture.status).label)
        assertEquals("An operator source is still needed.", fixture.reason)
        val minimal = MuseumJson.decodeFromString<RouteSuggestionInput>("""{"topic":"A question","area":"A region"}""")
        assertEquals(listOf("walk", "bike"), minimal.modes)
        assertTrue(minimal.sourceLinks.isEmpty())
        assertEquals("flexible", minimal.tripShape)
        assertFalse(MuseumJson.decodeFromString<SuggestionIntake>("{}").enabled)
        assertEquals("Status unavailable", suggestionStatus("future_server_state").label)
        assertEquals(listOf("Received", "Research in progress", "More research needed", "Curator review", "Accepted", "Declined"),
            listOf("draft", "processing", "needs_research", "review", "accepted", "declined").map { suggestionStatus(it).label })
    }

    @Test
    fun unknownClosedAndUnavailableGatesNeverSendASuggestion() = runTest {
        val flow = flow()
        flow.edit(SuggestionFields("Question", "Region"))
        assertNull(flow.submit())
        assertEquals(0, server.requestCount)
        server.enqueue(MockResponse().setBody("""{"enabled":false}"""))
        flow.loadIntake()
        request()
        assertFalse(flow.state.value.canSubmit)
        assertNull(flow.submit())
        server.enqueue(MockResponse().setResponseCode(404))
        flow.loadIntake()
        request()
        assertNull(flow.state.value.intake)
        assertNotNull(flow.state.value.intakeError)
        assertNull(flow.submit())
        assertEquals(2, server.requestCount)
        flow.close()
        assertNull(flow.submit())
    }

    @Test
    fun lostResponseRestoresExactBodyAndKeyAfterRecreation() = runTest {
        val store = InMemoryKeyValueStore()
        val first = flow(store)
        val fields = SuggestionFields("  A question  ", "Region", places = "Unknown depot\n\nAnother lead", sources = " https://example.org/lead ")
        first.edit(fields)
        open(first)
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AFTER_REQUEST))
        assertNull(first.submit())
        val lost = request()
        val pending = first.state.value.pending!!
        assertNotNull(lost.getHeader("Idempotency-Key"))
        assertEquals("Bearer fixture-owner-token", lost.getHeader("Authorization"))
        assertEquals("/api/v1/me/route-briefs", lost.path)
        assertFalse(first.state.value.canEdit)
        first.edit(SuggestionFields("Must not replace the uncertain request", "Elsewhere"))
        assertEquals(fields, first.state.value.fields)
        first.close()

        val restored = flow(store)
        assertEquals(fields, restored.state.value.fields)
        assertEquals(pending, restored.state.value.pending)
        open(restored)
        server.enqueue(response(code = 200))
        assertEquals(fixture, restored.submit())
        val retry = request()
        assertEquals(lost.getHeader("Idempotency-Key"), retry.getHeader("Idempotency-Key"))
        assertEquals(lost.body.readUtf8(), retry.body.readUtf8())
        assertEquals(SuggestionFields(), restored.state.value.fields)
        assertNull(flow(store).state.value.pending)
        assertEquals(fixture, restored.state.value.selected)
    }

    @Test
    fun validationRetainsEditableLeadsAndPermitsCorrection() = runTest {
        val flow = flow()
        val fields = SuggestionFields("Question", "Region", sources = "not a link")
        flow.edit(fields)
        open(flow)
        server.enqueue(MockResponse().setResponseCode(422).setBody("""{"error":{"code":"validation_failed","message":"Invalid links","fields":{"source_links":"Use HTTP or HTTPS without credentials"}}}"""))
        assertNull(flow.submit())
        val rejected = request()
        assertTrue(flow.state.value.canEdit)
        assertEquals(fields, flow.state.value.fields)
        assertEquals("Use HTTP or HTTPS without credentials", flow.state.value.fieldErrors["source_links"])
        flow.edit(fields.copy(sources = "https://example.org/lead"))
        server.enqueue(response(code = 201))
        assertNotNull(flow.submit())
        assertNotEquals(rejected.getHeader("Idempotency-Key"), request().getHeader("Idempotency-Key"))
    }

    @Test
    fun closureAndExpiredAuthenticationRetainRetryIdentity() = runTest {
        for ((code, name) in listOf(403 to "intake_disabled", 401 to "unauthorized")) {
            val store = InMemoryKeyValueStore()
            val flow = flow(store)
            flow.edit(SuggestionFields("Question", "Region"))
            open(flow)
            server.enqueue(MockResponse().setResponseCode(code).setBody("""{"error":{"code":"$name","message":"Unavailable"}}"""))
            assertNull(flow.submit())
            val rejected = request()
            assertNotNull(flow.state.value.pending)
            assertFalse(flow.state.value.canSubmit)
            val restored = flow(store)
            open(restored)
            server.enqueue(response())
            assertNotNull(restored.submit())
            assertEquals(rejected.getHeader("Idempotency-Key"), request().getHeader("Idempotency-Key"))
        }
    }

    @Test
    fun quotaErrorsKeepInputAndUsefulMessage() = runTest {
        val flow = flow()
        val fields = SuggestionFields("Question", "Region")
        flow.edit(fields)
        open(flow)
        server.enqueue(MockResponse().setResponseCode(429).setBody("""{"error":{"code":"submission_limit","message":"At most 10 new suggestions per day"}}"""))
        assertNull(flow.submit())
        request()
        assertEquals(fields, flow.state.value.fields)
        assertTrue(flow.state.value.canEdit)
        assertEquals("At most 10 new suggestions per day", flow.state.value.message)
    }

    @Test
    fun draftsArePartitionedByAccountAndOriginWithoutStoredTokens() {
        val values = mutableMapOf<String, String>()
        val store = object : KeyValueStore {
            override fun get(key: String) = values[key]
            override fun put(key: String, value: String) { values[key] = value }
            override fun remove(key: String) { values.remove(key) }
        }
        val first = flow(store)
        first.edit(SuggestionFields("Private question", "Region"))
        assertEquals(SuggestionFields(), flow(store, "another-account").state.value.fields)
        val otherOrigin = RouteSuggestions(api(base = "https://other.example"), store, owner) { true }
        assertEquals(SuggestionFields(), otherOrigin.state.value.fields)
        assertEquals("Private question", flow(store).state.value.fields.topic)
        assertTrue(values.keys.none { it.contains("fixture-owner-token") })
        assertTrue(values.values.none { it.contains("fixture-owner-token") })
        first.close()
        assertEquals(SuggestionFields(), first.state.value.fields)
        assertEquals("Private question", flow(store).state.value.fields.topic)
    }

    @Test
    fun foreignDetailAndOwnerMismatchNeverAppearAsPrivateStatus() = runTest {
        val flow = flow()
        server.enqueue(MockResponse().setResponseCode(404).setBody("""{"error":{"code":"not_found","message":"not found"}}"""))
        flow.loadDetail(fixture.id)
        request()
        assertNull(flow.state.value.selected)
        assertEquals("This suggestion is not available for this account.", flow.state.value.detailError)
        server.enqueue(response(fixture.copy(ownerAccountId = "another-account")))
        flow.loadDetail(fixture.id)
        request()
        assertNull(flow.state.value.selected)
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(ListPage(items = listOf(fixture.copy(ownerAccountId = "another-account"))))))
        flow.loadPage()
        request()
        assertTrue(flow.state.value.items.isEmpty())
        assertNotNull(flow.state.value.listError)
    }

    @Test
    fun privateListUsesBoundedLookaheadAndOffset() = runTest {
        val flow = flow()
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(ListPage(items = (1..21).map { fixture.copy(id = it.toString()) }))))
        flow.loadPage()
        assertEquals("/api/v1/me/route-briefs?limit=21&offset=0", request().path)
        assertEquals(20, flow.state.value.items.size)
        assertTrue(flow.state.value.hasMore)
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(ListPage(items = listOf(fixture.copy(id = "21"))))))
        flow.loadPage(20)
        assertEquals("/api/v1/me/route-briefs?limit=21&offset=20", request().path)
        assertEquals("21", flow.state.value.items.single().id)
        assertFalse(flow.state.value.hasMore)
    }

    @Test
    fun accountChangeDuringSubmissionIgnoresResponseAndKeepsRecovery() = runTest {
        val store = InMemoryKeyValueStore()
        val current = AtomicReference(owner)
        val flow = flow(store, current = { current.get() == owner })
        flow.edit(SuggestionFields("Original account input", "Region"))
        open(flow)
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.method == "POST") { entered.countDown(); release.await(3, TimeUnit.SECONDS); return response() }
                return MockResponse().setBody("""{"enabled":true}""")
            }
        }
        val old = async(Dispatchers.Default) { flow.submit() }
        try {
            assertTrue(entered.await(3, TimeUnit.SECONDS))
            current.set("another-account")
            assertEquals(SuggestionFields(), flow(store, "another-account").state.value.fields)
            release.countDown()
            assertNull(old.await())
            val original = request()
            flow.close()
            current.set(owner)
            val recovered = flow(store)
            assertNotNull(recovered.state.value.pending)
            recovered.loadIntake()
            request()
            assertNotNull(recovered.submit())
            assertEquals(original.getHeader("Idempotency-Key"), request().getHeader("Idempotency-Key"))
        } finally { release.countDown() }
    }

    @Test
    fun secondTapCannotSendSimultaneousSubmission() = runTest {
        val flow = flow()
        flow.edit(SuggestionFields("Question", "Region"))
        open(flow)
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                entered.countDown()
                release.await(3, TimeUnit.SECONDS)
                return response()
            }
        }
        val first = async(Dispatchers.Default) { flow.submit() }
        try {
            assertTrue(entered.await(3, TimeUnit.SECONDS))
            assertNull(flow.submit())
            assertEquals(2, server.requestCount)
            release.countDown()
            assertNotNull(first.await())
            request()
        } finally { release.countDown() }
    }


    @Test
    fun oldPageResponseCannotReplaceNewerSelection() = runTest {
        val flow = flow()
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val offset = request.requestUrl!!.queryParameter("offset")
                if (offset == "0") { entered.countDown(); release.await(3, TimeUnit.SECONDS) }
                return MockResponse().setBody(MuseumJson.encodeToString(ListPage(items = listOf(fixture.copy(id = if (offset == "0") "old" else "new")))))
            }
        }
        val old = async(Dispatchers.Default) { flow.loadPage(0) }
        try {
            assertTrue(entered.await(3, TimeUnit.SECONDS))
            flow.loadPage(20)
            assertEquals("new", flow.state.value.items.single().id)
            release.countDown()
            old.await()
            assertEquals(20, flow.state.value.offset)
            assertEquals("new", flow.state.value.items.single().id)
        } finally { release.countDown() }
    }

    @Test
    fun closedScreenCannotReceiveLatePrivateDetail() = runTest {
        val flow = flow()
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                entered.countDown()
                release.await(3, TimeUnit.SECONDS)
                return response()
            }
        }
        val loading = async(Dispatchers.Default) { flow.loadDetail(fixture.id) }
        try {
            assertTrue(entered.await(3, TimeUnit.SECONDS))
            flow.close()
            release.countDown()
            loading.await()
            assertNull(flow.state.value.selected)
            assertNull(flow.state.value.selectedId)
            assertEquals(SuggestionFields(), flow.state.value.fields)
        } finally { release.countDown() }
    }

    @Test
    fun failedPersistenceNeverSendsUnrecoverableRequest() = runTest {
        val store = object : KeyValueStore {
            override fun get(key: String): String? = null
            override fun put(key: String, value: String) { throw IOException("Synthetic disk error") }
            override fun remove(key: String) {}
        }
        val flow = flow(store)
        flow.edit(SuggestionFields("Question", "Region"))
        open(flow)
        assertNull(flow.submit())
        assertEquals(1, server.requestCount)
        assertEquals("Could not save the draft on this device. No submission was sent.", flow.state.value.message)
    }

    @Test
    fun confirmedSubmissionSurvivesDraftCleanupFailureWithItsRetryIdentity() = runTest {
        val values = mutableMapOf<String, String>()
        val store = object : KeyValueStore {
            override fun get(key: String) = values[key]
            override fun put(key: String, value: String) { values[key] = value }
            override fun remove(key: String) { throw IOException("Synthetic cleanup error") }
        }
        val flow = flow(store)
        flow.edit(SuggestionFields("Question", "Region"))
        open(flow)
        server.enqueue(response(code = 201))

        assertEquals(fixture, flow.submit())
        val original = request()
        assertEquals(fixture, flow.state.value.selected)
        assertNotNull(flow.state.value.pending)
        assertTrue(flow.state.value.message?.contains("Saved on the server") == true)

        val recovered = flow(store)
        open(recovered)
        server.enqueue(response(code = 200))
        assertEquals(fixture, recovered.submit())
        val retry = request()
        assertEquals(original.getHeader("Idempotency-Key"), retry.getHeader("Idempotency-Key"))
        assertEquals(original.body.readUtf8(), retry.body.readUtf8())
    }
}
