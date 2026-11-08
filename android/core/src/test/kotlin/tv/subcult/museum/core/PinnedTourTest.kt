package tv.subcult.museum.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.cancel
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import tv.subcult.museum.storage.FileKeyValueStore
import java.io.File
import java.io.IOException
import java.nio.file.Path
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

class PinnedTourTest {
    @TempDir lateinit var directory: Path
    private lateinit var server: MockWebServer
    private lateinit var store: FileKeyValueStore
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val owner = "alice"
    private fun api(origin: String = server.url("/").toString()) = Endpoints(ApiClient(origin) { "alice-token" })

    @BeforeEach
    fun setup() { server = MockWebServer(); server.start(); store = FileKeyValueStore(directory.toFile()) }

    @AfterEach
    fun stop() { scope.cancel(); server.shutdown() }

    /** The shared straight-line capture is not a qualified public path. These review and
     * arrival assertions are synthetic test input for client eligibility and transitions. */
    private fun revision(number: Int = 1): TourRevision {
        val raw: TourRevision = Fixtures.decode("tour_revision_$number")
        val snapshot = raw.snapshot!!
        val source: TourRoute = Fixtures.decode("journey-route-return")
        val stops = snapshot.tour.stops.map { stop ->
            val arrival = ApprovedArrival("arrival-${stop.place.id}", 1, ArrivalPoint("Synthetic entrance", "entrance", stop.place.latitude + 0.0002, stop.place.longitude + 0.0002,
                listOf("walk", "bike").map { ArrivalAccess(it, "allowed", "paved", "https://example.org/test", "Synthetic test reference", "2026-11-07", "Test only") }, "Synthetic test access only."), "2026-11-07T00:00:00Z")
            stop.copy(place = stop.place.copy(arrivalPoint = arrival))
        }
        return raw.copy(snapshot = snapshot.copy(tour = snapshot.tour.copy(revisionId = raw.id, revisionNumber = raw.number, stops = stops,
            journeys = snapshot.routes.keys.associateWith { source.journey!! }), routes = snapshot.routes.mapValues { it.value.copy(journey = source.journey) }))
    }

    private fun start(revision: TourRevision = revision(), signedIn: Boolean = false, mode: String = "walk", current: () -> Boolean = { true }) =
        ActiveTour.startPinned(api(), revision, mode, store, { signedIn }, scope, owner, current)

    private fun response(value: TourRevision) = MockResponse().setBody(MuseumJson.encodeToString(value))
    private fun latest(value: TourRevision) = MockResponse().setBody(MuseumJson.encodeToString(ListPage(items = listOf(value.copy(snapshot = null)))))
    private fun progress(value: TourRevision, ids: List<String>, account: String = owner) = RevisionProgress(account, value.tourId, value.id, "walk", ids)
    private fun request(): RecordedRequest = server.takeRequest(3, TimeUnit.SECONDS) ?: error("Expected a synthetic API request")
    private suspend fun offer(model: ActiveTour, candidate: TourRevision) {
        server.enqueue(latest(candidate))
        server.enqueue(response(candidate))
        model.checkForUpdate()
    }

    @Test
    fun sharedCapturesKeepStopIdentityAcrossReorderWithoutClaimingReadiness() {
        listOf("revision-1" to "tour_revision_1", "revision-2" to "tour_revision_2", "progress-1" to "tour_progress_1").forEach { (canonical, local) ->
            assertEquals(File("../../backend/api/fixtures/tour-revisions-v1/$canonical.json").readText(), Fixtures.text(local))
        }
        val first: TourRevision = Fixtures.decode("tour_revision_1")
        val second: TourRevision = Fixtures.decode("tour_revision_2")
        val progress: RevisionProgress = Fixtures.decode("tour_progress_1")
        val id = progress.visitedStopIds.first()
        assertEquals(1, first.snapshot!!.tour.stops.first { it.id == id }.position)
        assertEquals(2, second.snapshot!!.tour.stops.first { it.id == id }.position)
        assertNotNull(first.unavailableReason("walk"))
    }

    @Test
    fun olderResponsesDecodeButDoNotOfferANewReviewedStart() {
        val tour: TourDetail = Fixtures.decode("tour")
        assertNull(tour.revisionId)
        assertNull(tour.stops.first().place.arrivalPoint)
        assertFalse(tour.canStart("walk"))
    }

    @Test
    fun retainedGeometryAndStoriesNeverJoinTheCurrentCatalogue() = runTest {
        val first = revision()
        val model = start(first)
        server.enqueue(response(revision(2)))
        model.loadRoute()
        assertEquals(first.snapshot!!.routes["walk"], model.state.value.route)
        assertEquals(first.snapshot.tour, model.tour)
        val id = model.tour.stops.first().featuredMoment!!.id
        assertEquals(first.snapshot.moments[id], model.retainedMoment(id))
        assertEquals("/api/v1/tours/${first.tourId}/revisions/${first.id}", request().path)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun offlineDiskRestoreKeepsStartedRevisionDespiteANewerTourCache() = runTest {
        val first = revision()
        val model = start(first)
        model.markArrived()
        store.putJson(ActiveTour.tourKey(first.tourId), revision(2).snapshot!!.tour)
        val restored = ActiveTour.restore(api(), FileKeyValueStore(directory.toFile()), { false }, scope, owner)!!
        server.enqueue(MockResponse().setResponseCode(503))
        restored.loadRoute()
        assertEquals(first.id, restored.revision?.id)
        assertEquals(first.snapshot!!.tour.stops, restored.tour.stops)
        assertEquals(model.visitedStopIds, restored.visitedStopIds)
        assertEquals(first.snapshot.routes["walk"], restored.state.value.route)
        assertTrue(restored.state.value.routeFromCache)
        directory.toFile().listFiles()!!.filter { it.isFile }.forEach { assertFalse(it.readText().contains("alice-token")) }
    }

    @Test
    fun missingSnapshotCannotFallBackToTheLatestTour() {
        val first = revision()
        start(first)
        store.remove(ActiveTour.revisionKey(api(), owner, first.id))
        store.putJson(ActiveTour.tourKey(first.tourId), revision(2).snapshot!!.tour)
        assertNull(ActiveTour.restore(api(), store, { false }, scope, owner))
    }

    @Test
    fun anotherAccountOrOriginCannotRestoreTheTrip() {
        start()
        assertNull(ActiveTour.restore(api(), store, { true }, scope, "bob"))
        assertNull(ActiveTour.restore(api("https://other.test"), store, { true }, scope, owner))
    }

    @Test
    fun startingRequiresTheActualSnapshotFileToBeWritten() {
        val first = revision()
        val blocked = directory.resolve(ActiveTour.revisionKey(api(), owner, first.id) + ".json").toFile()
        assertTrue(blocked.mkdir())
        blocked.resolve("retained").writeText("Keep")
        assertThrows(IOException::class.java) { start(first) }
        assertNull(store.getJson<ActiveTourRecord>(ActiveTourRecord.KEY))
    }

    @Test
    fun nestedSnapshotRevisionIdentityMustMatchTheRevisionEnvelope() {
        val first = revision()
        val mismatched = first.copy(snapshot = first.snapshot!!.copy(tour = first.snapshot.tour.copy(revisionId = "other-revision", revisionNumber = first.number + 1)))
        assertThrows(ApiException::class.java) { start(mismatched) }

        store.putJson(ActiveTour.revisionKey(api(), owner, first.id), mismatched)
        store.putJson(ActiveTourRecord.KEY, ActiveTourRecord(first.tourId, "walk", owner, first.id, api().client.baseUrl.trimEnd('/')))
        assertNull(ActiveTour.restore(api(), store, { false }, scope, owner))
    }

    @Test
    fun updateAdoptionRejectsAMismatchedNestedSnapshotRevision() = runTest {
        val first = revision()
        val target = revision(2)
        val retained = target.snapshot!!
        val mismatched = target.copy(snapshot = retained.copy(tour = retained.tour.copy(revisionId = first.id, revisionNumber = first.number)))
        val model = start(first)
        offer(model, mismatched)
        assertNotNull(model.state.value.updateError)
        server.enqueue(response(mismatched))
        try { model.adoptingUpdate(); fail<Unit>("Mismatched retained snapshot must not be adopted") }
        catch (_: ApiException) { }
    }

    @Test
    fun progressUsesStableIdsOnTheExactRevisionEndpoint() = runTest {
        val first = revision()
        val id = first.snapshot!!.tour.stops.first().id
        val model = start(first, signedIn = true)
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(progress(first, listOf(id)))))
        model.markArrived()
        model.flush()
        val req = request()
        assertEquals("/api/v1/me/progress/${first.tourId}/revisions/${first.id}", req.path)
        val body = MuseumJson.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals(listOf(id), body["visited_stop_ids"]!!.jsonArray.map { it.jsonPrimitive.content })
        assertFalse(body.containsKey("visited_positions"))
    }

    @Test
    fun remoteProgressRejectsOtherOwnersAndRevisionsAndFiltersUnknownIds() = runTest {
        val first = revision()
        val id = first.snapshot!!.tour.stops.first().id
        val model = start(first, signedIn = true)
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(progress(first, listOf(id), "bob"))))
        model.loadRemoteProgress()
        assertTrue(model.visitedStopIds.isEmpty())
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(progress(revision(2), listOf(id)))))
        model.loadRemoteProgress()
        assertTrue(model.visitedStopIds.isEmpty())
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(progress(first, listOf(id, "unknown")))))
        model.loadRemoteProgress()
        assertEquals(setOf(id), model.visitedStopIds)
    }

    @Test
    fun missingRevisionProgressIsAnEmptyRemoteRecordWithoutASyncErrorOrEmptyPut() = runTest {
        val first = revision()
        val model = start(first, signedIn = true)
        server.enqueue(MockResponse().setResponseCode(404).setBody(Fixtures.text("error_not_found")))

        model.loadRemoteProgress()

        assertNull(model.state.value.syncError)
        assertFalse(model.needsSync)
        assertEquals(1, server.requestCount)
        assertEquals("GET", request().method)
    }

    @Test
    fun updateDiscoveryUsesRevisionMetadataInsteadOfLegacyPinnedTourDetail() = runTest {
        val first = revision()
        val target = revision(2)
        val model = start(first)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/api/v1/tours/${first.tourId}/revisions?limit=1" -> latest(target)
                "/api/v1/tours/${first.tourId}/revisions/${target.id}" -> response(target)
                "/api/v1/tours/${first.tourId}" -> MockResponse().setBody(MuseumJson.encodeToString(first.snapshot!!.tour))
                else -> MockResponse().setResponseCode(404)
            }
        }

        model.checkForUpdate()

        assertEquals(target.id, model.state.value.availableUpdate?.id)
        assertEquals(2, server.requestCount)
        assertEquals("/api/v1/tours/${first.tourId}/revisions?limit=1", request().path)
        assertEquals("/api/v1/tours/${first.tourId}/revisions/${target.id}", request().path)
    }

    @Test
    fun explicitUpdateCarriesReorderedIdsAndKeepsOldRevisionProgressSeparate() = runTest {
        val first = revision()
        val target = revision(2)
        val model = start(first)
        model.markArrived()
        val ids = model.visitedStopIds
        offer(model, target)
        assertEquals(first.id, model.revision?.id)
        assertEquals(setOf(1), model.state.value.visited)
        assertEquals(target.id, model.state.value.availableUpdate?.id)
        server.enqueue(response(target))
        val adopted = model.adoptingUpdate()
        assertEquals(target.id, adopted.revision?.id)
        assertEquals(ids, adopted.visitedStopIds)
        assertEquals(setOf(2), adopted.state.value.visited)
        assertEquals(1, adopted.state.value.nextStop?.position)
        assertTrue(model.end(), "Ending the old model must not remove the target record")
        assertEquals(target.id, ActiveTour.restore(api(), store, { false }, scope, owner)?.revision?.id)
        assertEquals(setOf(1), start(first).state.value.visited)
    }

    @Test
    fun changedArrivalDoesNotCarryThePreviousVisit() = runTest {
        val model = start()
        model.markArrived()
        val raw = revision(2)
        val retained = raw.snapshot!!
        val target = raw.copy(snapshot = retained.copy(tour = retained.tour.copy(stops = retained.tour.stops.map { stop ->
            if (stop.id in model.visitedStopIds) stop.copy(place = stop.place.copy(arrivalPoint = stop.place.arrivalPoint!!.copy(version = 2))) else stop
        })))
        offer(model, target)
        server.enqueue(response(target))
        assertTrue(model.adoptingUpdate().visitedStopIds.isEmpty())
    }

    @Test
    fun anUnavailableModeDoesNotReplaceTheStartedRevision() = runTest {
        val model = start(mode = "bike")
        val raw = revision(2)
        val retained = raw.snapshot!!
        val target = raw.copy(snapshot = retained.copy(routes = retained.routes + ("bike" to retained.routes.getValue("bike").copy(status = "failed"))))
        offer(model, target)
        assertNotNull(model.state.value.updateError)
        server.enqueue(response(target))
        try { model.adoptingUpdate(); fail<Unit>("Unavailable bike route must not be adopted") }
        catch (_: ApiException) { }
        assertEquals(1, model.revision?.number)
    }

    @Test
    fun knownRevocationClearsThePinnedCacheAndDisablesArrivals() = runTest {
        val first = revision()
        val model = start(first)
        server.enqueue(MockResponse().setResponseCode(404).setBody(Fixtures.text("error_not_found")))
        model.loadRoute()
        assertTrue(model.state.value.revoked)
        assertFalse(model.canUse)
        assertFalse(model.state.value.isComplete)
        assertNull(model.state.value.nextStop)
        assertNull(model.state.value.route)
        assertNull(store.get(ActiveTour.revisionKey(api(), owner, first.id)))
        assertNull(ActiveTour.restore(api(), store, { false }, scope, owner))
        model.markArrived()
        assertTrue(model.visitedStopIds.isEmpty())
    }

    @Test
    fun navigationUsesReviewedEntranceInsteadOfTheSiteCoordinates() {
        val model = start()
        val stop = model.state.value.nextStop!!
        val point = model.navigationPoint(stop)!!
        assertEquals(stop.place.latitude + 0.0002, point.lat, 0.0000001)
        assertEquals(stop.place.longitude + 0.0002, point.lng, 0.0000001)
        assertEquals("google.navigation:q=${point.lat},${point.lng}&mode=w", model.navigationUri())
    }

    @Test
    fun oldSavedTripsUseOnlyCachedGeometryAndLocalProgress() = runTest {
        val tour: TourDetail = Fixtures.decode("tour")
        val original = ActiveTour(api(), tour, "walk", store, { false }, scope, owner)
        original.markArrived()
        val restored = ActiveTour.restore(api(), store, { true }, scope, owner)!!
        restored.loadRoute()
        restored.loadRemoteProgress()
        restored.markArrived()
        restored.syncIfNeeded()
        assertTrue(restored.state.value.legacyResume)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun offlineRevisionResetResumesBeforeAnyRemoteMerge() = runTest {
        val first = revision()
        val original = start(first, signedIn = true)
        server.enqueue(MockResponse().setResponseCode(503))
        original.resetProgress()
        original.flush()
        assertTrue(original.needsSync)
        request()
        val restored = ActiveTour.restore(api(), store, { true }, scope, owner)!!
        server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(progress(first, emptyList()))))
        restored.loadRemoteProgress()
        assertFalse(restored.needsSync)
        assertTrue(restored.visitedStopIds.isEmpty())
        val req = request()
        assertEquals("PUT", req.method)
        val body = MuseumJson.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals("true", body["reset"]!!.jsonPrimitive.content)
        assertTrue(body["visited_stop_ids"]!!.jsonArray.isEmpty())
    }

    @Test
    fun reviewedNoticesStayWithSelectedModeAndSegments() {
        val corridor = ReviewedCorridor(ReviewedCorridor.Intent(ReviewedCorridor.Reference("Retained guidance"), listOf(
            ReviewedCorridor.Segment("selected", listOf(ReviewedCorridor.Access("walk", "Walking notice"), ReviewedCorridor.Access("bike", "Biking notice"))),
            ReviewedCorridor.Segment("other", listOf(ReviewedCorridor.Access("walk", "Unselected notice")))),
            listOf("walk", "bike").map { ReviewedCorridor.Selection(it, listOf(ReviewedCorridor.Traversal("selected"))) }))
        assertEquals(listOf("Retained guidance", "Walking notice"), corridor.notices("walk"))
        assertEquals(listOf("Retained guidance", "Biking notice"), corridor.notices("bike"))
    }

    @Test
    fun aLateOwnerResponseCannotPopulateAnOldSession() = runTest {
        val current = AtomicBoolean(true)
        val first = revision()
        val model = start(first, signedIn = true, current = current::get)
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                entered.countDown()
                release.await(3, TimeUnit.SECONDS)
                return MockResponse().setBody(MuseumJson.encodeToString(progress(first, listOf(first.snapshot!!.tour.stops[0].id))))
            }
        }
        val pending = scope.async { model.loadRemoteProgress() }
        assertTrue(entered.await(3, TimeUnit.SECONDS))
        current.set(false)
        release.countDown()
        pending.await()
        assertTrue(model.visitedStopIds.isEmpty())
        assertFalse(model.canUse)
        assertNull(model.navigationUri())
    }

    @Test
    fun anOlderPutCannotUndoAResetThatHappensWhileItIsInFlight() = runTest {
        val first = revision()
        val model = start(first, signedIn = true)
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        val sequence = AtomicInteger()
        val bodies = CopyOnWriteArrayList<String>()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                bodies.add(request.body.readUtf8())
                if (sequence.incrementAndGet() == 1) { entered.countDown(); release.await(3, TimeUnit.SECONDS) }
                return MockResponse().setBody(MuseumJson.encodeToString(progress(first, emptyList())))
            }
        }
        model.markArrived()
        assertTrue(entered.await(3, TimeUnit.SECONDS))
        model.resetProgress()
        release.countDown()
        model.flush()
        assertTrue(model.visitedStopIds.isEmpty())
        assertFalse(model.needsSync)
        val last = MuseumJson.parseToJsonElement(bodies.last()).jsonObject
        assertEquals("true", last["reset"]!!.jsonPrimitive.content)
        assertTrue(last["visited_stop_ids"]!!.jsonArray.isEmpty())
    }
}
