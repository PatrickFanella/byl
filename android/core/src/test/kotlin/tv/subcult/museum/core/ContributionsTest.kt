package tv.subcult.museum.core

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.File
import java.time.Instant
import java.util.UUID
import kotlin.math.*

internal fun contributionRevision(): TourRevision {
    val original: TourRevision = Fixtures.decode("tour_revision_1")
    val snapshot = original.snapshot!!
    val journey: TourRoute = Fixtures.decode("journey-route-return")
    val source = journey.journey!!.segments.first().source
    val segment = ReviewedCorridor.Segment("signal-fixture", geometry = MuseumJson.encodeToJsonElement(LineString(coordinates = listOf(listOf(-88.0, 41.8), listOf(-88.0 + 400 / (6_371_000 * cos(41.8 * PI / 180)) * 180 / PI, 41.8)))),
        signalReferences = buildJsonObject { put("walk", MuseumJson.encodeToJsonElement(ReviewedSignalReference(1, true, emptyList(), emptyList(), source))) })
    return original.copy(snapshot = snapshot.copy(tour = snapshot.tour.copy(corridor = ReviewedCorridor(ReviewedCorridor.Intent(ReviewedCorridor.Reference(), listOf(segment), listOf(ReviewedCorridor.Selection("walk", listOf(ReviewedCorridor.Traversal(segment.id))))))),
        routes = mapOf("walk" to journey.copy(journey = journey.journey!!.copy(segments = listOf(journey.journey!!.segments.first().copy(id = segment.id, reverse = false, access = "allowed")))))))
}

private class TestContributionApi : ContributionApi {
    var enabled = true
    var grantError: Exception? = null
    var submitError: Exception? = null
    var withdrawalError: Exception? = null
    val choices = mutableListOf<Pair<ContributionChoice, String>>()
    val summaries = mutableListOf<Pair<DeviationSummary, String>>()
    val withdrawals = mutableListOf<Pair<ContributionConsent, String>>()
    var onGrant: (suspend () -> Unit)? = null
    var onGate: (suspend () -> Unit)? = null
    override suspend fun gate(): ContributionGate { val result = ContributionGate(enabled, false, DeviationDetector.VERSION); onGate?.invoke(); return result }
    override suspend fun grant(choice: ContributionChoice, key: String): ContributionConsent {
        choices += choice to key; onGrant?.invoke(); grantError?.let { throw it }
        return ContributionConsent("60000000-0000-4000-8000-000000000001", "a".repeat(64), day(choice.requestedOn)!!.plusDays(30).toString())
    }
    override suspend fun submit(consent: ContributionConsent, summary: DeviationSummary, key: String) { summaries += summary to key; submitError?.let { throw it } }
    override suspend fun withdraw(consent: ContributionConsent, origin: String) { withdrawals += consent to origin; withdrawalError?.let { throw it } }
}

@OptIn(ExperimentalCoroutinesApi::class)
class ContributionsTest {
    @TempDir lateinit var root: File
    private var now = Instant.parse("2026-11-07T12:00:00Z").toEpochMilli()
    private var identity = true
    private val owner = "50000000-0000-4000-8000-000000000001"
    private val api = TestContributionApi()
    private val store by lazy { ContributionStore(root) { now } }
    private fun model(settings: Boolean = false) = TripContributions(store, api, owner, "https://stub.test", if (settings) null else contributionRevision(), "walk", { identity }, { now }).also { it.setForeground(true) }
    private fun fix(x: Double, time: Long = now) = Fix(41.8, -88 + x / (6_371_000 * cos(41.8 * PI / 180)) * 180 / PI, 3.0, time)
    private suspend fun passage(model: TripContributions) {
        val start = now
        for (i in 0..20) { now = start + i * 10_000; if (i % 3 == 0) model.refresh(); model.observe(fix(i * 20.0), true) }
    }

    @Test fun `optional malformed metadata and transit never authorize observation`() {
        val revision = contributionRevision(); assertEquals(1, contributionContexts(revision, "walk").size)
        val snapshot = revision.snapshot!!; val tour = snapshot.tour; val corridor = tour.corridor!!; val intent = corridor.intent!!
        val malformed = tour.copy(corridor = corridor.copy(intent = intent.copy(segments = intent.segments.map { it.copy(signalReferences = JsonPrimitive("bad")) })))
        val raw = MuseumJson.encodeToString(revision.copy(snapshot = snapshot.copy(tour = malformed)))
        assertTrue(contributionContexts(MuseumJson.decodeFromString(raw), "walk").isEmpty())
        assertTrue(contributionContexts(revision.copy(snapshot = snapshot.copy(tour = tour.copy(transit = buildJsonObject { put("unsupported", true) }))), "walk").isEmpty())
        val roundTrip = MuseumJson.decodeFromString<TourRevision>(MuseumJson.encodeToString(revision.copy(snapshot = snapshot.copy(tour = tour.copy(transit = JsonPrimitive(false))))))
        assertTrue(contributionContexts(roundTrip, "walk").isEmpty())
        val disallowed = revision.copy(snapshot = snapshot.copy(routes = snapshot.routes.mapValues { (_, route) -> route.copy(journey = route.journey!!.copy(segments = route.journey!!.segments.map { it.copy(access = "unknown") })) }))
        assertTrue(contributionContexts(disallowed, "walk").isEmpty())
    }

    @Test fun `navigation never grants consent and closed gate refuses explicit choice`() = runTest {
        val model = model(); assertTrue(model.eligible); assertTrue(model.state.value.paused)
        passage(model); assertTrue(api.choices.isEmpty()); assertTrue(store.read().isEmpty())
        api.enabled = false; model.optIn(); assertTrue(api.choices.isEmpty())
    }
    @Test fun `choice and receipt are durable before minimal summaries and exact retries`() = runTest {
        val model = model()
        api.onGrant = { assertNotNull(store.read().first().key) }
        model.optIn(); assertTrue(model.state.value.optedIn); assertNotNull(store.read().single().consent)
        passage(model); val pending = store.read().single().queue.single()
        api.submitError = ApiException(0, "network", "Lost response")
        model.flush(); assertEquals(1, model.state.value.pending); assertFalse(model.state.value.admitted)
        api.submitError = ApiException(409, "already_contributed", "Duplicate")
        model.refresh(); assertEquals(0, model.state.value.pending)
        assertEquals(listOf(pending.summary to pending.key, pending.summary to pending.key), api.summaries)
    }
    @Test fun `lost grant replays original choice after restart without automatic resume`() = runTest {
        val first = model(); api.grantError = ApiException(0, "network", "Lost response"); first.optIn()
        val retained = store.read().single(); assertNull(retained.consent)
        api.grantError = null; now += 86_400_000
        val restored = model(); restored.refresh()
        assertEquals(api.choices.first(), api.choices.last()); assertEquals(retained.key, store.read().single().key)
        assertTrue(restored.state.value.paused); assertTrue(restored.state.value.optedIn)
    }
    @Test fun `restored trips and Profile cannot upload queues before explicit Resume`() = runTest {
        val active = model(); active.optIn(); passage(active)
        val restored = model(); val profile = model(settings = true)
        restored.refresh(); profile.refresh()
        assertTrue(api.summaries.isEmpty()); assertTrue(restored.state.value.paused)
        restored.setPaused(false)
        assertEquals(1, api.summaries.size); assertFalse(restored.state.value.paused)
    }
    @Test fun `seven UTC days physically expire unsent summaries before uploads`() = runTest {
        val model = model(); model.optIn(); passage(model); assertEquals(1, model.state.value.pending)
        now = Instant.parse("2026-11-14T00:00:00Z").toEpochMilli(); model.refresh()
        assertTrue(api.summaries.isEmpty()); assertEquals(0, model.state.value.pending)
        assertFalse(root.resolve("contributions-v1.json").readText().contains("observation_period"))
    }
    @Test fun `manual arrival pause background permission loss and end discard raw episodes`() = runTest {
        for (control in listOf(DeviationDetector.Control.MANUAL_ARRIVAL, DeviationDetector.Control.PAUSE)) {
            val model = model(); model.optIn(); model.lifecycle(control); passage(model)
            assertEquals(0, model.state.value.pending)
            model.lifecycle(DeviationDetector.Control.RESUME); passage(model); assertEquals(1, model.state.value.pending)
            model.eraseAll()
        }
        val model = model(); model.optIn(); model.setForeground(false); passage(model); assertEquals(0, model.state.value.pending)
        model.setForeground(true); passage(model); assertEquals(1, model.state.value.pending); model.flush()
        model.lifecycle(DeviationDetector.Control.PERMISSION_LOST); model.observe(fix(0.0), false)
        passage(model); assertEquals(0, model.state.value.pending) // replay latch within the same UTC day
        model.lifecycle(DeviationDetector.Control.END); model.lifecycle(DeviationDetector.Control.RESUME)
        now += 86_400_000; passage(model); assertEquals(0, model.state.value.pending)
    }
    @Test fun `prechoice cached fixes and stale admission cannot qualify a passage`() = runTest {
        val model = model(); model.optIn()
        model.observe(fix(0.0, now - 1), true)
        for (i in 3..20) { now += 10_000; if (i % 3 == 0) model.refresh(); model.observe(fix(i * 20.0), true) }
        assertEquals(0, model.state.value.pending)
        now += 61_000; model.observe(fix(0.0), true); assertFalse(model.state.value.admitted)
    }
    @Test fun `account change stops collection and signed out deletion uses retained capability`() = runTest {
        val model = model(); model.optIn(); passage(model)
        identity = false; model.identityChanged(); model.flush()
        assertTrue(api.summaries.isEmpty()); assertFalse(model.state.value.optedIn)
        model.eraseAll(); assertTrue(store.read().isEmpty()); assertEquals("https://stub.test", api.withdrawals.single().second)
    }
    @Test fun `permission loss discards an incomplete passage until a new authorized fix`() = runTest {
        val model = model(); model.optIn()
        model.observe(fix(0.0), true); now += 10_000; model.observe(fix(20.0), true)
        model.lifecycle(DeviationDetector.Control.PERMISSION_LOST)
        for (i in 3..20) { now += 10_000; if (i % 3 == 0) model.refresh(); model.observe(fix(i * 20.0), true) }
        assertEquals(0, model.state.value.pending)
        now += 120_000; model.refresh()
        passage(model); assertEquals(1, model.state.value.pending)
    }
    @Test fun `record queue limit and file write failure stop work without losing the original`() = runTest {
        val model = model(); model.optIn(); passage(model); val record = store.read().single()
        val original = root.resolve("contributions-v1.json").readBytes()
        val overflow = record.copy(queue = (0..32).map { record.queue.single().copy(key = UUID.randomUUID().toString()) })
        assertThrows(ContributionStorageException::class.java) { store.write(listOf(overflow)) }
        assertArrayEquals(original, root.resolve("contributions-v1.json").readBytes())
        val profile = model(settings = true)
        root.resolve("contributions-v1.pending").mkdir()
        profile.eraseAll(); assertFalse(profile.state.value.optedIn); assertFalse(profile.state.value.admitted)
        model.observe(fix(0.0), true)
        assertFalse(model.state.value.optedIn)
        assertArrayEquals(original, root.resolve("contributions-v1.json").readBytes())
        val reopenedProfile = model(settings = true)
        reopenedProfile.refresh()
        model.flush(); assertTrue(api.summaries.isEmpty())
        assertEquals(1, api.withdrawals.size)
        assertTrue(store.read().isEmpty())
    }
    @Test fun `wrong withdrawal capability remains pending instead of assuming deleted`() = runTest {
        val model = model(); model.optIn(); passage(model)
        api.withdrawalError = ApiException(404, "not_found", "Not found"); model.eraseAll()
        assertEquals(0, model.state.value.pending); assertEquals(1, model.state.value.deletionPending)
        assertNotNull(store.read().single().consent)
        api.withdrawalError = null; model.refresh(); assertTrue(store.read().isEmpty())
    }
    @Test fun `late admission success cannot override a newer closed gate`() = runTest {
        val started = CompletableDeferred<Unit>(); val reply = CompletableDeferred<Unit>(); var calls = 0
        api.onGate = { if (calls++ == 0) { started.complete(Unit); reply.await() } }
        val model = model(); val old = async { model.refresh() }; started.await()
        api.enabled = false; model.refresh(); assertFalse(model.state.value.admitted)
        reply.complete(Unit); old.await(); assertFalse(model.state.value.admitted)
    }
    @Test fun `account loss during grant retains the late capability for signed out erasure`() = runTest {
        val started = CompletableDeferred<Unit>(); val reply = CompletableDeferred<Unit>()
        api.onGrant = { started.complete(Unit); reply.await() }
        val model = model(); val choice = async { model.optIn() }; started.await()
        identity = false; model.identityChanged(); reply.complete(Unit); choice.await()
        assertFalse(model.state.value.optedIn); assertNotNull(store.read().single().consent)
        val profile = model(settings = true); profile.eraseAll()
        assertTrue(store.read().isEmpty()); assertEquals(1, api.withdrawals.size); assertTrue(api.summaries.isEmpty())
    }
    @Test fun `global erasure during pregrant gate check cancels a choice on another controller`() = runTest {
        val started = CompletableDeferred<Unit>(); val reply = CompletableDeferred<Unit>()
        api.onGate = { started.complete(Unit); reply.await() }
        val model = model(); val choice = async { model.optIn() }; started.await()
        val profile = model(settings = true); profile.eraseAll()
        reply.complete(Unit); choice.await()
        assertTrue(api.choices.isEmpty()); assertTrue(store.read().isEmpty())
    }
    @Test fun `deletion during grant saves and withdraws the late capability before removing local record`() = runTest {
        val started = CompletableDeferred<Unit>(); val reply = CompletableDeferred<Unit>()
        api.onGrant = { started.complete(Unit); reply.await() }
        val model = model(); val choice = async { model.optIn() }; started.await()
        val deletion = async { model.eraseAll() }; runCurrent()
        assertTrue(store.read().single().withdrawing); reply.complete(Unit)
        choice.await(); deletion.await()
        assertTrue(store.read().isEmpty()); assertEquals(1, api.withdrawals.size); assertFalse(model.state.value.optedIn)
    }
    @Test fun `bounds and hostile storage stop new grants without overwriting retained state`() = runTest {
        val model = model(); model.optIn(); val record = store.read().single(); val original = root.resolve("contributions-v1.json").readBytes()
        assertThrows(ContributionStorageException::class.java) { store.write((0..16).map { record.copy(choice = record.choice.copy(revisionId = UUID.randomUUID().toString())) }) }
        assertArrayEquals(original, root.resolve("contributions-v1.json").readBytes())
        root.resolve("contributions-v1.json").writeText("x".repeat(131073))
        val blocked = model(); blocked.optIn(); assertEquals(1, api.choices.size); assertFalse(blocked.state.value.admitted)
    }
    @Test fun `unknown fields and mismatched summary identity are rejected`() = runTest {
        val model = model(); model.optIn(); passage(model); val record = store.read().single()
        assertThrows(ContributionStorageException::class.java) { store.write(listOf(record.copy(queue = record.queue.map { it.copy(summary = it.summary.copy(revisionId = UUID.randomUUID().toString())) }))) }
        val file = root.resolve("contributions-v1.json")
        file.writeText(file.readText().replace("\"owner\":", "\"raw_fixes\":[],\"owner\":"))
        assertThrows(ContributionStorageException::class.java) { store.read() }
    }
    @Test fun `expiry of original consent and unknown receipt never renews retention`() = runTest {
        val model = model(); model.optIn(); val expiry = store.read().single().consent!!.expiresOn
        now = Instant.parse(expiry + "T00:00:00Z").toEpochMilli(); model.refresh(); assertTrue(store.read().isEmpty())
        val other = model(); api.grantError = ApiException(0, "network", "Lost response"); other.optIn()
        now += 31L * 86_400_000; store.prune(); assertTrue(store.read().isEmpty())
    }
}
