package tv.subcult.museum.core

import kotlinx.coroutines.*
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.JsonPrimitive
import okhttp3.mockwebserver.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*
import java.time.Instant
import java.util.concurrent.CopyOnWriteArrayList

class ScheduledTransitTest {
    private lateinit var server: MockWebServer
    private lateinit var api: Endpoints
    private lateinit var store: TestStore
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val requests = CopyOnWriteArrayList<RecordedRequest>()
    private var now = Instant.parse("2026-11-08T20:00:00Z")
    private var enabled = true
    private var refuse = false
    private var reply: ScheduledTransitPlan = Fixtures.decode("backend-transit-plan")
    private var candidate: TourRevision? = null
    private var late: (() -> Unit)? = null
    private class TestStore: KeyValueStore {
        val map = mutableMapOf<String, String>()
        var fail = false
        var silent = false
        override fun get(key: String) = map[key]
        override fun put(key: String, value: String) { if (fail) error("Synthetic storage failure"); if (!silent) map[key] = value }
        override fun remove(key: String) { if (fail) error("Synthetic storage failure"); map.remove(key) }
    }
    @BeforeEach fun setup() {
        server = MockWebServer(); server.start(); api = Endpoints(ApiClient(server.url("/").toString()) { "alice-token" }); store = TestStore()
        server.dispatcher = object: Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                requests += request
                val path = request.requestUrl!!.encodedPath
                if (path.endsWith("itinerary-capabilities")) return MockResponse().setBody("{\"scheduled_transit_v1\":$enabled}")
                if (path.endsWith("transit-plan")) { late?.invoke(); return MockResponse().setBody(MuseumJson.encodeToString(reply)) }
                if (refuse) return MockResponse().setResponseCode(403).setBody("{\"error\":{\"code\":\"unsupported_itinerary\",\"message\":\"Disabled\"}}")
                candidate?.let { revision ->
                    if (path.endsWith("/revisions")) return MockResponse().setBody(MuseumJson.encodeToString(ListPage(items = listOf(revision.copy(snapshot = null)))))
                    return MockResponse().setBody(MuseumJson.encodeToString(revision))
                }
                return MockResponse().setResponseCode(503)
            }
        }
    }
    @AfterEach fun stop() { scope.cancel(); server.shutdown() }
    private fun raw(): TourRevision = Fixtures.decode("backend-transit-revision")
    /** Explicit synthetic arrival assertions; the actual handler wire corpus stays unchanged. */
    private fun revision(): TourRevision {
        val r = raw(); val snapshot = r.snapshot!!
        val stops = snapshot.tour.stops.mapIndexed { n, s ->
            s.copy(featuredMoment = null, featuredMomentId = null, place = s.place.copy(arrivalPoint = ApprovedArrival("synthetic-$n", 1,
                ArrivalPoint("Synthetic public arrival", "entrance", s.place.latitude, s.place.longitude,
                    listOf(ArrivalAccess("walk", "allowed", "paved", "https://example.org/synthetic", "Synthetic test", "2026-11-08", "Not field proof"))), "2026-11-08T00:00:00Z")))
        }
        return r.copy(snapshot = snapshot.copy(tour = snapshot.tour.copy(stops = stops)))
    }
    private fun controller(owner: String? = "alice", rev: TourRevision = revision(), current: () -> Boolean = { true }) = TransitTrip(api, rev, store, owner, current, { now })
    private suspend fun retained(): TransitTrip = controller().also { it.request(Instant.parse(reply.requestedDepartureAt)); it.accept() }
    private fun active(r: TourRevision = revision(), admitted: Boolean? = true) = ActiveTour.startPinned(api, r, "walk", store, { false }, scope, "alice", { true }, admitted)
    private fun unavailable(state: String) = reply.copy(state = state, segments = emptyList(), service = TransitService(state, true, false))

    @Test fun actualHandlerWireCorpusMatchesRevisionAndSeparateWalkingLegs() {
        assertTrue(reply.valid(raw())); assertTrue(raw().snapshot!!.tour.approvedTransit!!.valid(raw().snapshot!!.tour))
        assertNull(raw().snapshot!!.routes["walk"])
        assertEquals(listOf("walk_to_stop","boarding_wait","transit","walk_from_stop","walk","walk"), reply.segments.map { it.kind })
    }
    @Test fun alteredRevisionPlatformsLiveRailGeometryAndOrderAreRejected() {
        listOf(reply.copy(revisionId = "other"), reply.copy(connectorVersion = 999), reply.copy(service = reply.service.copy(live = true)),
            reply.copy(service = reply.service.copy(board = reply.service.board!!.copy(id = "999"))),
            reply.copy(segments = reply.segments.map { if (it.kind == "transit") it.copy(geometry = LineString(coordinates = listOf(listOf(0.0,0.0),listOf(1.0,1.0)))) else it }),
            reply.copy(segments = reply.segments.toMutableList().also { java.util.Collections.swap(it,0,1) }))
            .forEach { assertFalse(it.valid(revision())) }
    }
    @Test fun missingOrdinaryLegOrChangedWalkingGeometryIsRejected() {
        assertFalse(reply.copy(segments = reply.segments.dropLast(1)).valid(revision()))
        assertFalse(reply.copy(segments = reply.segments.map { if (it.kind == "walk") it.copy(geometry = LineString(coordinates = listOf(listOf(0.0,0.0),listOf(1.0,1.0)))) else it }).valid(revision()))
    }
    @Test fun unavailableAndStaleResultsCannotGuide() {
        listOf("no_service","stale","provider_unavailable","unavailable","unsupported","needs_review").forEach {
            val value = unavailable(it); assertTrue(value.valid(revision()), it); assertNotNull(value.unavailableReason(now), it)
        }
        assertNotNull(reply.unavailableReason(now.plusSeconds(2 * 86400)))
        assertNotNull(reply.unavailableReason(Instant.parse("2026-11-08T21:13:00Z")))
    }
    @Test fun explicitAcceptAndResumeNeverPostAnotherPlan() = runTest {
        val c = controller(); c.request(Instant.parse(reply.requestedDepartureAt))
        assertNull(c.state.value.selected); assertNotNull(c.state.value.proposed); c.accept(); assertTrue(c.canGuide(now))
        val count = requests.count { it.method == "POST" }
        val restored = controller(); assertEquals(c.state.value.selected, restored.state.value.selected); assertFalse(restored.canGuide(now))
        restored.refreshGate(); assertTrue(restored.canGuide(now)); assertEquals(count, requests.count { it.method == "POST" })
    }
    @Test fun requestUsesCapabilityAndOnlyVersionDeparturePayload() = runTest {
        retained(); val request = requests.first { it.method == "POST" }
        assertEquals("scheduled-transit-v1", request.getHeader("X-Byl-Capabilities"))
        val body = MuseumJson.parseToJsonElement(request.body.readUtf8()) as kotlinx.serialization.json.JsonObject
        assertEquals(setOf("version","departure_at"), body.keys); assertNull(request.requestUrl!!.query)
    }
    @Test fun disabledGatePreservesChoiceButPreventsRequestsAndDirections() = runTest {
        val c = retained(); enabled = false; c.refreshGate(); assertFalse(c.canGuide(now)); assertNotNull(c.state.value.selected)
        val count = requests.count { it.method == "POST" }; c.request(Instant.parse(reply.requestedDepartureAt))
        assertEquals(count, requests.count { it.method == "POST" })
    }
    @Test fun gateLeaseExpiresAndStaleSourceMissedBoardingSuspendGuidance() = runTest {
        val c = retained(); now = now.plusSeconds(61); assertFalse(c.canGuide(now))
        c.refreshGate(); assertTrue(c.canGuide(now)); now = now.plusSeconds(2 * 86400); assertFalse(c.canGuide(now))
    }
    @Test fun wrongDepartureCannotBeAccepted() = runTest {
        val c = controller(); c.request(now.plusSeconds(1000)); assertNull(c.state.value.proposed)
        assertThrows(ApiException::class.java) { c.accept() }
    }
    @Test fun storageFailureDoesNotAdoptProposal() = runTest {
        val c = controller(); c.request(Instant.parse(reply.requestedDepartureAt)); store.fail = true
        assertThrows(IllegalStateException::class.java) { c.accept() }; assertNull(c.state.value.selected); assertNotNull(c.state.value.proposed)
    }
    @Test fun silentStorageFailureIsDetectedByReadback() = runTest {
        val c = controller(); c.request(Instant.parse(reply.requestedDepartureAt)); store.silent = true
        assertThrows(ApiException::class.java) { c.accept() }; assertNull(c.state.value.selected)
    }
    @Test fun accountOriginAndRevisionIsolateChoices() = runTest {
        val original = retained(); assertNull(controller("bob").state.value.selected); assertNull(controller(null).state.value.selected)
        assertNull(controller(rev = revision().copy(id = "other")).state.value.selected)
        api.client.baseUrl = "https://other.test"; assertNull(controller().state.value.selected); assertFalse(original.canGuide(now))
    }
    @Test fun reviewSuspensionSurvivesReopenWithoutReplacingChoice() = runTest {
        val c = retained(); val chosen = c.state.value.selected; val scheduled = reply
        reply = unavailable("needs_review"); c.request(Instant.parse(reply.requestedDepartureAt))
        assertEquals(chosen, c.state.value.selected); assertTrue(c.state.value.guidanceBlocked)
        val reopened = controller(); reopened.refreshGate(); assertTrue(reopened.state.value.guidanceBlocked); assertFalse(reopened.canGuide(now))
        reply = scheduled; reopened.request(Instant.parse(reply.requestedDepartureAt)); reopened.accept(); assertFalse(reopened.state.value.guidanceBlocked)
    }
    @Test fun corruptRecordRequiresExplicitRemoval() = runTest {
        retained(); store.map[store.map.keys.first { it.startsWith("transit-plan.v1.") }] = "{invalid"
        val c = controller(); c.request(Instant.parse(reply.requestedDepartureAt)); assertNull(c.state.value.proposed); assertNotNull(c.state.value.message); assertTrue(c.state.value.requiresRemoval)
        c.remove(); assertFalse(c.state.value.requiresRemoval); c.request(Instant.parse(reply.requestedDepartureAt)); assertNotNull(c.state.value.proposed)
    }
    @Test fun removeAndEndCancelLateAcceptance() = runTest {
        val c = retained(); c.remove(); assertNull(controller().state.value.selected)
        c.request(Instant.parse(reply.requestedDepartureAt)); c.invalidate(); assertNull(c.state.value.proposed); assertThrows(ApiException::class.java) { c.accept() }
    }
    @Test fun accountLossDuringResponsePreventsLateProposal() = runTest {
        var current = true; val c = controller(current = { current }); late = { current = false }
        c.request(Instant.parse(reply.requestedDepartureAt)); assertNull(c.state.value.proposed); assertNull(c.state.value.selected)
    }
    @Test fun endDuringResponsePreventsLateProposal() = runTest {
        val c = controller(); late = { c.invalidate() }; c.request(Instant.parse(reply.requestedDepartureAt))
        assertNull(c.state.value.proposed); assertFalse(c.state.value.busy)
    }
    @Test fun publicWalkingHandoffRequiresCurrentAdmission() = runTest {
        val c = retained()
        listOf(true,false).forEach { boarding ->
            val url = java.net.URI(c.walkingUrl(boarding, now)!!)
            assertEquals("www.google.com", url.host); assertTrue(url.query.contains("travelmode=walking")); assertTrue(url.query.contains("origin=")); assertTrue(url.query.contains("destination="))
            assertFalse(url.toString().contains("alice")); assertFalse(url.toString().contains("departure"))
        }
        c.invalidate(); assertNull(c.walkingUrl(true, now))
    }
    @Test fun mixedProgressIsPinnedWithNoRailWalkingGuidance() {
        val r = revision(); assertNull(r.unavailableReason("walk")); assertThrows(ApiException::class.java) { active(r, null) }
        val c = active(r); assertNull(c.state.value.route); assertNotNull(c.transit); assertEquals(2,c.state.value.futureLegs.size)
        c.markArrived(); assertTrue(c.transitInterval); assertNull(c.navigationUri()); assertNull(c.geoUri()); assertNull(c.state.value.activeLeg)
        val restored = ActiveTour.restore(api, store, { false }, scope, "alice")!!; assertEquals(c.visitedStopIds, restored.visitedStopIds)
        restored.markArrived(); assertFalse(restored.transitInterval); assertNotNull(restored.state.value.activeLeg)
    }
    @Test fun authoritativeRefusalRevokesSavedMixedTrip() = runTest {
        val c = active(); refuse = true; c.loadRoute()
        assertTrue(c.state.value.revoked); assertNull(c.state.value.nextStop); assertTrue(c.state.value.futureLegs.isEmpty())
        assertNull(ActiveTour.restore(api,store,{false},scope,"alice"))
    }
    @Test fun malformedSelectionCannotBecomeOrdinaryWalk() {
        val r = revision(); val s = r.snapshot!!
        assertNotNull(r.copy(snapshot = s.copy(tour = s.tour.copy(transit = JsonPrimitive(true)))).unavailableReason("walk"))
    }
    @Test fun revisionAdoptionRequiresGateAndKeepsOldChoiceSeparate() = runTest {
        retained(); val first = revision(); val c = active(first); c.markArrived()
        val s = first.snapshot!!
        candidate = first.copy(id = "next-revision",number = first.number + 1,snapshot = s.copy(tour = s.tour.copy(revisionId = "next-revision",revisionNumber = first.number + 1)))
        c.checkForUpdate(); assertEquals(candidate!!.id,c.state.value.availableUpdate?.id)
        enabled = false
        try { c.adoptingUpdate(); fail("disabled gate must refuse update") } catch (_: ApiException) { }
        enabled = true; val next = c.adoptingUpdate()
        assertEquals(candidate!!.id,next.revision?.id); assertEquals(c.visitedStopIds,next.visitedStopIds)
        assertNull(next.transit?.state?.value?.selected); assertNotNull(controller(rev = first).state.value.selected)
    }

    @Test fun `parses timetable times serialized with a Chicago offset`() {
        // JVM tests run on a parser that accepts offsets in Instant.parse; Android 8–13 does not.
        assertEquals(Instant.parse("2026-11-10T14:15:30Z"), transitInstant("2026-11-10T08:15:30-06:00"))
        assertEquals(Instant.parse("2026-11-10T14:15:30.250Z"), transitInstant("2026-11-10T14:15:30.25Z"))
        assertNull(transitInstant("2026-11-10 08:15"))
    }
}
