package tv.subcult.museum.core

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.jsonObject
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import java.io.IOException
import java.util.UUID
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

class RouteReportsTest {
    private lateinit var server: MockWebServer
    private val fixture: RouteReport = Fixtures.decode("route_report")
    private val revision: TourRevision = Fixtures.decode("tour_revision_1")
    private val context get() = RouteReportContext(revision, "walk")
    private val owner get() = fixture.ownerAccountId
    private val http = OkHttpClient.Builder().retryOnConnectionFailure(false).callTimeout(3, TimeUnit.SECONDS).build()
    @BeforeEach fun start() { server = MockWebServer(); server.start() }
    @AfterEach fun stop() = server.shutdown()
    // The fixture listens on IPv4. After intentional connection loss, a cached
    // localhost route can fall through to unserved ::1 when retries are disabled.
    private fun api(base: String = server.url("/").newBuilder().host("127.0.0.1").build().toString()) = Endpoints(ApiClient(base, http) { "opening-token" })
    private fun flow(store: KeyValueStore = InMemoryKeyValueStore(), account: String = owner, context: RouteReportContext? = this.context, current: () -> Boolean = { true }) = RouteReports(api(), store, account, context, isCurrent = current)
    private fun response(input: RouteReportInput = fixture.submitted, code: Int = 201) = MockResponse().setResponseCode(code).setBody(MuseumJson.encodeToString(fixture.copy(submitted = input)))
    private fun request() = requireNotNull(server.takeRequest(5, TimeUnit.SECONDS))

    @Test fun canonicalFixtureAndReportOnlyContextKeepRetainedReferences() {
        assertEquals("entrance", fixture.submitted.affected.kind)
        assertEquals("Please confirm the operator notice and closure dates.", fixture.reason)
        assertNull(fixture.submitted.annotation); assertNull(fixture.submitted.alternative)
        assertNull(context.unavailableReason)
        assertEquals(2, context.targets.count { it.affected.kind == "entrance" })
        assertEquals(2, context.targets.count { it.affected.kind == "stop" })
        val empty = revision.snapshot!!.let { snapshot -> revision.copy(snapshot = snapshot.copy(routes = snapshot.routes + ("walk" to snapshot.routes.getValue("walk").copy(legs = emptyList())))) }
        assertTrue(RouteReportContext(empty, "walk").targets.none { it.affected.kind == "segment" })
    }
    @Test fun closingReturnUsesStableStopIDsAndExplicitMismatchesAreRejected() {
        val snapshot = revision.snapshot!!; val stops = snapshot.tour.stops
        val route = snapshot.routes.getValue("walk")
        val malformed = route.legs.first().copy(fromStopId = stops[1].id)
        val returning = route.legs.first().copy(fromPosition = 2, toPosition = 1, fromStopId = stops[1].id, toStopId = stops[0].id, isReturn = true)
        val retained = revision.copy(snapshot = snapshot.copy(routes = snapshot.routes + ("walk" to route.copy(legs = listOf(malformed, returning)))))
        val segments = RouteReportContext(retained, "walk").targets.filter { it.affected.kind == "segment" }
        assertEquals(1, segments.size); assertTrue(segments[0].label.endsWith("(return)")); assertEquals(stops[1].id, segments[0].affected.fromStopId)
    }
    @Test fun unavailableModeAndMalformedNestedRevisionCannotSend() = runTest {
        val unsupported = flow(context = RouteReportContext(revision, "drive"))
        assertFalse(unsupported.state.value.canSubmit); assertNull(unsupported.submit())
        val malformed = revision.copy(snapshot = revision.snapshot!!.copy(tour = revision.snapshot!!.tour.copy(revisionId = "different")))
        val mismatch = flow(context = RouteReportContext(malformed, "walk"))
        assertFalse(mismatch.state.value.canSubmit); assertNull(mismatch.submit()); assertEquals(0, server.requestCount)
    }
    @Test fun minimalReportPinsModeSegmentAndRevisionWithoutJourneyOrCoordinates() = runTest {
        val model = flow()
        val segment = context.targets.first { it.affected.kind == "segment" }
        val fields = model.state.value.fields.copy(target = segment.id, details = " Report only ")
        model.edit(fields)
        val input = fields.input(context); server.enqueue(response(input))
        assertNotNull(model.submit())
        val posted = request()
        assertEquals("/api/v1/me/route-reports", posted.path)
        assertEquals("Bearer opening-token", posted.getHeader("Authorization"))
        assertNotNull(UUID.fromString(posted.getHeader("Idempotency-Key")))
        val body = posted.body.readUtf8()
        val decoded = MuseumJson.decodeFromString<RouteReportInput>(body)
        assertEquals(revision.id, decoded.revisionId); assertEquals("walk", decoded.mode); assertEquals(segment.affected, decoded.affected)
        assertEquals("Report only", decoded.details); assertNull(decoded.annotation); assertNull(decoded.alternative)
        val keys = MuseumJson.parseToJsonElement(body).jsonObject.keys
        assertTrue(keys.none { it in listOf("trace", "visited_stop_ids", "started_at", "locations") })
        assertNull(model.state.value.pending); assertNotNull(model.state.value.selected)
    }
    @Test fun optionalCoordinatesAreExplicitFiniteAndBounded() {
        val fields = ReportFields(target = context.targets[1].id, latitude = "41.8", longitude = "-88", alternativePoints = "-88,41.8\n-88.0001,41.8001")
        val input = fields.input(context)
        assertEquals(ReportPoint(41.8, -88.0), input.annotation); assertEquals(2, input.alternative!!.coordinates.size)
        assertThrows(IllegalArgumentException::class.java) { fields.copy(latitude = "NaN").input(context) }
        assertThrows(IllegalArgumentException::class.java) { fields.copy(longitude = "").input(context) }
        assertThrows(IllegalArgumentException::class.java) { fields.copy(alternativePoints = "-88,41").input(context) }
        assertThrows(IllegalArgumentException::class.java) { fields.copy(alternativePoints = List(65) { "-88,41" }.joinToString("\n")).input(context) }
        assertThrows(IllegalArgumentException::class.java) { fields.copy(alternativePoints = "-88,41,9\n-88,41").input(context) }
    }
    @Test fun lostResponseRecreatesExactRequestBodyAndKey() = runTest {
        val store = InMemoryKeyValueStore(); val first = flow(store)
        val fields = first.state.value.fields.copy(details = "Keep this private observation")
        first.edit(fields)
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AFTER_REQUEST))
        assertNull(first.submit()); val lost = request(); val pending = first.state.value.pending!!
        first.edit(fields.copy(details = "Do not change this uncertain request"))
        assertEquals(fields, first.state.value.fields); first.close()
        val restored = flow(store); assertEquals(pending, restored.state.value.pending); assertEquals(fields, restored.state.value.fields)
        server.enqueue(response(pending.input, 200)); assertNotNull(restored.submit(), restored.state.value.toString()); val retry = request()
        assertEquals(lost.body.readUtf8(), retry.body.readUtf8()); assertEquals(lost.getHeader("Idempotency-Key"), retry.getHeader("Idempotency-Key"))
        assertNull(flow(store).state.value.pending)
    }
    private class FailingStore : KeyValueStore {
        val values = mutableMapOf<String, String>()
        @Volatile var failPut = false
        @Volatile var failRemove = false
        var cancelRemove = false
        override fun get(key: String) = values[key]
        override fun put(key: String, value: String) { if (failPut) throw IOException("Fixture full disk"); values[key] = value }
        override fun remove(key: String) {
            if (cancelRemove) throw CancellationException("Fixture cancelled cleanup")
            if (failRemove) throw IOException("Fixture read-only disk")
            values.remove(key)
        }
    }
    @Test fun failedDiskWriteSendsNoPostAndRetainsTheInMemoryKey() = runTest {
        val store = FailingStore(); val model = flow(store)
        store.failPut = true
        assertNull(model.submit()); assertEquals(0, server.requestCount)
        assertTrue(model.state.value.message!!.contains("No report was sent"))
        val pending = model.state.value.pending!!
        store.failPut = false; server.enqueue(response(pending.input)); assertNotNull(model.submit())
        assertEquals(pending.key, request().getHeader("Idempotency-Key"))
    }
    @Test fun cancelledCleanupPropagatesAndRetainsThePersistedRetryWithoutPublishingSuccess() = runTest {
        val store = FailingStore(); val model = flow(store)
        val input = model.state.value.fields.input(context)
        store.cancelRemove = true; server.enqueue(response(input))
        try { model.submit(); fail<Unit>("Expected cleanup cancellation") }
        catch (_: CancellationException) { /* Expected: the acknowledged request can be replayed. */ }
        val posted = request()
        assertNull(model.state.value.selected); assertFalse(model.state.value.busy)
        val pending = model.state.value.pending!!
        assertEquals(posted.getHeader("Idempotency-Key"), pending.key)
        assertEquals(pending, flow(store).state.value.pending)
    }
    @Test fun rejectedInputIsUnlockedOnlyAfterDurablyClearingThePendingRequest() = runTest {
        val store = FailingStore(); val model = flow(store)
        val fields = model.state.value.fields.copy(sourceUrl = "invalid"); model.edit(fields)
        server.enqueue(MockResponse().setResponseCode(422).setBody("""{"error":{"code":"validation","message":"Invalid","fields":{"source_url":"Use HTTP"}}}"""))
        assertNull(model.submit()); request(); assertTrue(model.state.value.canEdit)
        assertEquals(fields, model.state.value.fields); assertNull(model.state.value.pending)
        assertEquals("Use HTTP", model.state.value.fieldErrors["source_url"])
    }
    @Test fun failedDiscardAndFailedValidationCleanupKeepPendingInput() = runTest {
        val store = FailingStore(); val model = flow(store)
        val fields = model.state.value.fields.copy(details = "Keep private input"); model.edit(fields)
        server.enqueue(MockResponse().setResponseCode(503)); assertNull(model.submit()); request()
        val pending = model.state.value.pending!!
        store.failRemove = true; model.discardLocalDraft(); assertEquals(pending, model.state.value.pending); assertEquals(fields, model.state.value.fields)
        store.failRemove = false
        server.dispatcher = object : okhttp3.mockwebserver.Dispatcher() {
            override fun dispatch(request: okhttp3.mockwebserver.RecordedRequest): MockResponse {
                store.failPut = true
                return MockResponse().setResponseCode(422).setBody("""{"error":{"code":"validation","message":"Invalid"}}""")
            }
        }
        assertNull(model.submit()); request(); assertEquals(pending, model.state.value.pending); assertFalse(model.state.value.canEdit)
        assertEquals(fields, model.state.value.fields)
    }
    @Test fun draftsAreScopedToAccountServerRevisionAndModeAndCorruptDraftsFailClosed() {
        val store = FailingStore(); val model = flow(store)
        model.edit(model.state.value.fields.copy(details = "Private draft"))
        assertEquals("Private draft", flow(store).state.value.fields.details)
        assertEquals("", flow(store, account = "another-account").state.value.fields.details)
        assertEquals("", flow(store, context = RouteReportContext(revision, "bike")).state.value.fields.details)
        assertEquals("", flow(store, context = context.copy(revision = revision.copy(id = UUID.randomUUID().toString()))).state.value.fields.details)
        val other = RouteReports(api("https://another.test/"), store, owner, context) { true }
        assertEquals("", other.state.value.fields.details)
        store.values[store.values.keys.first()] = "corrupt pending identity"
        val corrupt = flow(store); assertFalse(corrupt.state.value.canEdit); assertFalse(corrupt.state.value.canSubmit)
        corrupt.discardLocalDraft(); assertTrue(corrupt.state.value.canEdit)
    }
    @Test fun unauthorizedAndForeignSubmissionsNeverPopulateAnotherOwner() = runTest {
        val model = flow()
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error":{"code":"unauthorized","message":"Expired"}}"""))
        assertNull(model.submit()); request(); assertTrue(model.state.value.needsSignIn); assertNotNull(model.state.value.pending)
        val foreign = flow(); val input = foreign.state.value.fields.input(context)
        server.enqueue(MockResponse().setResponseCode(201).setBody(MuseumJson.encodeToString(fixture.copy(ownerAccountId = "foreign", submitted = input))))
        assertNull(foreign.submit()); request(); assertNull(foreign.state.value.selected); assertNotNull(foreign.state.value.pending)
    }
    @Test fun staleAccountResponseAndDoubleTapCannotChangeTheOpeningRequest() = runTest {
        val current = AtomicBoolean(true); val model = flow(current = { current.get() })
        val input = model.state.value.fields.input(context)
        server.enqueue(response(input).setBodyDelay(150, TimeUnit.MILLISECONDS))
        val first = async(Dispatchers.Default) { model.submit() }
        request()
        assertNull(model.submit()); current.set(false)
        assertNull(first.await()); assertNull(model.state.value.selected)
        assertEquals(1, server.requestCount)
        model.close(); assertEquals(ReportFields(), model.state.value.fields)
    }
    @Test fun ownerDecisionAndCorrectionOutcomeAreReadWithoutSwitchingATrip() = runTest {
        val model = flow(context = null)
        server.enqueue(MockResponse().setBody(Fixtures.text("route_report")))
        val outcome = RouteCorrectionOutcome(UUID.randomUUID().toString(), fixture.submitted.revisionId, true, "2026-12-01", true)
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(outcome)))
        model.loadDetail(fixture.id)
        assertEquals(fixture.reason, model.state.value.selected!!.reason); assertEquals(outcome, model.state.value.outcome)
        assertEquals("GET", request().method); assertEquals("GET", request().method); assertEquals(2, server.requestCount)
    }
    @Test fun noCorrectionYetIsNormalAndForeignDetailIsRefused() = runTest {
        val model = flow(context = null)
        server.enqueue(MockResponse().setBody(Fixtures.text("route_report"))); server.enqueue(MockResponse().setResponseCode(404))
        model.loadDetail(fixture.id); request(); request()
        assertNotNull(model.state.value.selected); assertNull(model.state.value.outcome); assertNull(model.state.value.outcomeError)
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(fixture.copy(ownerAccountId = "foreign"))))
        model.loadDetail(fixture.id); request()
        assertNull(model.state.value.selected); assertNotNull(model.state.value.detailError); assertNull(model.state.value.outcome)
    }
    @Test fun pagesAreBoundedAndWrongCorrectionSourceNeverOffersAnUpdate() = runTest {
        val model = flow(context = null)
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(ListPage((0..20).map { fixture.copy(id = "report-$it") }))))
        model.loadPage(40); val page = request()
        assertEquals("21", page.requestUrl!!.queryParameter("limit")); assertEquals("40", page.requestUrl!!.queryParameter("offset"))
        assertEquals(20, model.state.value.items.size); assertTrue(model.state.value.hasMore)
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(ListPage(listOf(fixture.copy(ownerAccountId = "foreign"))))))
        model.loadPage(); request(); assertTrue(model.state.value.items.isEmpty()); assertNotNull(model.state.value.listError)
        server.enqueue(MockResponse().setBody(Fixtures.text("route_report")))
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(RouteCorrectionOutcome("new", "wrong-source", false))))
        model.loadDetail(fixture.id); request(); request()
        assertNotNull(model.state.value.selected); assertNull(model.state.value.outcome); assertNotNull(model.state.value.outcomeError)
    }
}
