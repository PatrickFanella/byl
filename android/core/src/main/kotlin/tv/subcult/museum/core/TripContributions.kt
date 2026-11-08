package tv.subcult.museum.core

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.util.UUID

/** UI-thread confined, foreground-only state. Navigation never waits on contribution IO. */
class TripContributions(
    private val store: ContributionStore, private val api: ContributionApi,
    private val owner: String?, origin: String, private val revision: TourRevision?, private val mode: String,
    private val sameIdentity: () -> Boolean, private val now: () -> Long = System::currentTimeMillis,
    private val key: () -> String = { UUID.randomUUID().toString() }
) {
    data class State(
        val message: String = "Route improvement is off. Navigation does not contribute location data.",
        val admitted: Boolean = false, val optedIn: Boolean = false, val paused: Boolean = true,
        val busy: Boolean = false, val pending: Int = 0, val retainedCount: Int = 0, val deletionPending: Int = 0
    )
    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state
    private fun change(f: (State) -> State) { _state.value = f(_state.value) }
    private val origin = origin.trimEnd('/')
    private val contexts = contributionContexts(revision, mode)
    val eligible = contexts.isNotEmpty()
    val identityAvailable get() = owner != null && sameIdentity()
    private val recordId = if (owner != null && revision != null) listOf(this.origin, owner, revision.tourId, revision.id, mode).joinToString("|") else null
    private var foreground = false
    private var ended = false
    private var stopped = false
    private var uploadsAuthorized = false
    private var storageBroken = false
    private var manualSuspended = false
    private var permissionSuspended = false
    private var gateChecked: Long? = null
    private var confirmed: Pair<String, Long>? = null
    private var samplingSince = Long.MAX_VALUE
    private var sampleDay = ""
    private var gateRequest = 0L
    private var choiceEpoch = 0L
    private var acceptedGeneration = store.choiceGeneration
    private var actions = 0
    private val flushing = Mutex()
    private var detectors = emptyList<DeviationDetector>()
    init { try { store.prune(); restore() } catch (_: Exception) { storageFailed() } }
    private val freshGate get() = state.value.admitted && gateChecked?.let { now() - it in 0..60_000 } == true
    private val mayResume get() = foreground && !ended && !stopped && acceptedGeneration == store.choiceGeneration && !state.value.paused && !manualSuspended && !permissionSuspended && freshGate && sameIdentity() && state.value.optedIn
    private fun restore() {
        val records = store.read(); val current = records.firstOrNull { it.id == recordId }
        change { it.copy(pending = records.sumOf { r -> r.queue.size }, retainedCount = records.size, deletionPending = records.count { r -> r.withdrawing },
            optedIn = !stopped && acceptedGeneration == store.choiceGeneration && sameIdentity() && current?.enabled == true && !current.withdrawing && (current.consent?.expiresOn ?: "") > contributionDay(now())) }
    }
    private fun storageFailed() {
        storageBroken = true; stopped = true; confirmed = null; detectors = emptyList(); samplingSince = Long.MAX_VALUE
        change { it.copy(admitted = false, optedIn = false, message = "Private contribution storage is unavailable. Collection and uploads stopped; navigation still works. Retry storage, then make an explicit choice.") }
    }
    private fun pauseEpisode() { detectors.forEach { it.control(DeviationDetector.Control.PAUSE) }; samplingSince = Long.MAX_VALUE }
    private fun resumeSampling() {
        if (!mayResume || confirmed?.let { now() - it.second in 0..60_000 } != true) return
        if (detectors.isEmpty()) detectors = contexts.map(::DeviationDetector)
        detectors.forEach { it.control(DeviationDetector.Control.RESUME) }
        samplingSince = now(); sampleDay = contributionDay(now())
        change { it.copy(message = "Route improvement is on for this revision and mode. Only qualified day-level counts can be sent.") }
    }
    fun setForeground(value: Boolean) {
        if (value == foreground) return
        foreground = value
        if (value) { confirmed = null; samplingSince = Long.MAX_VALUE } else pauseEpisode()
    }
    fun identityChanged() {
        if (sameIdentity()) return
        choiceEpoch++; stopped = true; uploadsAuthorized = false; confirmed = null; detectors = emptyList(); samplingSince = Long.MAX_VALUE
        change { it.copy(optedIn = false, message = "Reopen this trip after changing accounts or servers before choosing to contribute. Saved deletion details remain in Profile.") }
    }
    fun lifecycle(control: DeviationDetector.Control) {
        when (control) {
            DeviationDetector.Control.END -> { ended = true; uploadsAuthorized = false; choiceEpoch++ }
            DeviationDetector.Control.PERMISSION_LOST -> permissionSuspended = true
            DeviationDetector.Control.PAUSE, DeviationDetector.Control.MANUAL_ARRIVAL -> manualSuspended = true
            DeviationDetector.Control.RESUME -> manualSuspended = false
        }
        detectors.forEach { it.control(control) }
        if (control == DeviationDetector.Control.RESUME) resumeSampling() else samplingSince = Long.MAX_VALUE
        if (ended) { detectors = emptyList(); confirmed = null }
    }
    suspend fun setPaused(value: Boolean) {
        if (value) { choiceEpoch++; uploadsAuthorized = false; change { it.copy(paused = true) }; pauseEpisode(); return }
        if (ended || storageBroken || stopped || acceptedGeneration != store.choiceGeneration || !sameIdentity()) return
        uploadsAuthorized = true
        awaitRefresh()
        if (!state.value.optedIn || !freshGate || stopped) return
        change { it.copy(paused = false) }; resumeSampling()
    }
    suspend fun refresh() = awaitRefresh()
    private suspend fun awaitRefresh() {
        val request = ++gateRequest
        try {
            store.prune(); storageBroken = false; restore()
            val wasFresh = freshGate
            val gate = api.gate()
            if (request != gateRequest) return
            val admitted = gate.enabled && !gate.calibrated && gate.detectorVersion == DeviationDetector.VERSION
            change { it.copy(admitted = admitted) }; gateChecked = now()
            if (!admitted) { confirmed = null; pauseEpisode(); change { it.copy(message = "Route improvement collection is unavailable. Navigation still works.") } }
            flush()
            if (state.value.admitted && (!wasFresh || samplingSince == Long.MAX_VALUE)) resumeSampling()
        } catch (e: Exception) {
            if (e is CancellationException) throw e
            if (request != gateRequest) return
            if (e is ContributionStorageException) { storageFailed(); return }
            confirmed = null; pauseEpisode()
            change { it.copy(admitted = false, message = "Collection could not be checked. Observation stopped; saved summaries wait for a retry.") }
        }
    }
    suspend fun optIn() {
        val revision = revision ?: return; val owner = owner ?: return; val id = recordId ?: return
        if (state.value.busy || ended || !eligible || !foreground || !sameIdentity()) return
        begin(); val epoch = choiceEpoch; val generation = store.choiceGeneration
        try {
            store.prune(); storageBroken = false; refresh()
            if (epoch != choiceEpoch || generation != store.choiceGeneration || ended || !foreground || !freshGate || !sameIdentity() || storageBroken) return
            val records = store.read().toMutableList(); val index = records.indexOfFirst { it.id == id }
            if (index >= 0) {
                if (records[index].withdrawing) { change { it.copy(message = "Finish pending deletion before choosing again for this revision.") }; return }
                records[index] = records[index].copy(enabled = true)
            } else records += ContributionRecord(origin, owner, ContributionChoice(tourId = revision.tourId, revisionId = revision.id, mode = mode, requestedOn = contributionDay(now())), key(), true, false)
            store.write(records); acceptedGeneration = generation; stopped = false; uploadsAuthorized = true; restore()
            flush(); restore()
            if (epoch != choiceEpoch || generation != store.choiceGeneration || ended || !state.value.optedIn || !freshGate || stopped || !sameIdentity()) return
            change { it.copy(paused = false) }; resumeSampling()
        } catch (e: Exception) { if (e is CancellationException) throw e; storageFailed() }
        finally { finish() }
    }
    fun observe(fix: Fix, authorizationGranted: Boolean) {
        if (acceptedGeneration != store.choiceGeneration) {
            stopped = true; confirmed = null; detectors = emptyList(); samplingSince = Long.MAX_VALUE
            change { it.copy(optedIn = false, paused = true) }; return
        }
        if (!authorizationGranted) { lifecycle(DeviationDetector.Control.PERMISSION_LOST); return }
        if (state.value.admitted && !freshGate) {
            confirmed = null; pauseEpisode(); change { it.copy(admitted = false, message = "The collection check expired. Observation stopped; retry when online. Navigation still works.") }; return
        }
        if (fix.timestampMillis > now() + 1000 || now() - fix.timestampMillis > 30_000) { pauseEpisode(); resumeSampling(); return }
        if (permissionSuspended) { permissionSuspended = false; resumeSampling() }
        val confirmation = confirmed
        if (!mayResume || confirmation == null || now() - confirmation.second !in 0..60_000) { pauseEpisode(); return }
        if (fix.timestampMillis < samplingSince) return
        try {
            val record = store.read().firstOrNull { it.id == recordId }
            if (record == null || !record.enabled || record.withdrawing || record.key != confirmation.first || (record.consent?.expiresOn ?: "") <= contributionDay(now())) {
                change { it.copy(optedIn = false) }; detectors = emptyList(); confirmed = null; return
            }
            if (sampleDay != contributionDay(now())) { detectors = emptyList(); resumeSampling() }
            for (detector in detectors) for (summary in detector.push(fix)) {
                require(ContributionStore.validSummary(summary))
                val current = store.read().firstOrNull { it.id == record.id && it.key == record.key }
                if (current == null || !current.enabled || current.withdrawing) { pauseEpisode(); return }
                if (current.queue.any { it.summary.segmentId == summary.segmentId && it.summary.direction == summary.direction && it.summary.observationPeriod == summary.observationPeriod }) continue
                if (current.queue.size >= 32) { change { it.copy(paused = true, message = "Saved summary limit reached. Retry uploads or erase before resuming.") }; pauseEpisode(); return }
                store.update(current.id, current.key) { it.copy(queue = it.queue + PendingDeviation(key(), summary)) }
            }
            restore()
        } catch (_: Exception) { storageFailed() }
    }
    suspend fun eraseAll() {
        begin(); choiceEpoch++; stopped = true; uploadsAuthorized = false; confirmed = null; detectors = emptyList(); samplingSince = Long.MAX_VALUE
        change { it.copy(optedIn = false, paused = true) }
        try { store.stopAll(); restore(); flush() }
        catch (e: Exception) { if (e is CancellationException) throw e; storageFailed() }
        finally { finish() }
    }
    private fun begin() { actions++; change { it.copy(busy = true) } }
    private fun finish() { actions--; change { it.copy(busy = actions > 0) } }
    suspend fun flush() = flushing.withLock {
        try {
            store.prune()
            for (initial in store.read()) {
                var record = store.read().firstOrNull { it.id == initial.id && it.key == initial.key } ?: continue
                var freshlyConfirmed = false
                if (record.consent == null) {
                    if ((!record.enabled && !record.withdrawing) || (!record.withdrawing && acceptedGeneration != store.choiceGeneration) || !foreground || !freshGate || !sameIdentity() || record.owner != owner || record.origin != origin) continue
                    val consent = api.grant(record.choice, record.key)
                    require(ContributionStore.validConsent(consent, record.choice))
                    store.update(record.id, record.key) { it.copy(consent = consent) }; freshlyConfirmed = true
                }
                record = store.read().firstOrNull { it.id == record.id && it.key == record.key } ?: continue
                val consent = record.consent ?: continue
                if (record.withdrawing) {
                    api.withdraw(consent, record.origin)
                    store.write(store.read().filterNot { it.id == record.id && it.key == record.key }); continue
                }
                if (stopped || !uploadsAuthorized || acceptedGeneration != store.choiceGeneration || !foreground || !record.enabled || !freshGate || !sameIdentity() || record.owner != owner || record.origin != origin) continue
                if (!freshlyConfirmed) require(api.grant(record.choice, record.key) == consent)
                val current = store.read().firstOrNull { it.id == record.id && it.key == record.key } ?: continue
                if (!current.enabled || current.withdrawing || !sameIdentity() || !foreground || stopped || !uploadsAuthorized || acceptedGeneration != store.choiceGeneration || !freshGate) continue
                if (current.id == recordId) confirmed = current.key to now()
                for (pending in current.queue) {
                    store.prune()
                    val before = store.read().firstOrNull { it.id == current.id && it.key == current.key } ?: break
                    if (!foreground || stopped || !uploadsAuthorized || acceptedGeneration != store.choiceGeneration || !freshGate || !sameIdentity() || !before.enabled || before.withdrawing || before.queue.none { it.key == pending.key }) break
                    try { api.submit(before.consent ?: break, pending.summary, pending.key) }
                    catch (e: ApiException) { if (e.status != 409 || e.code != "already_contributed") throw e }
                    store.update(before.id, before.key) { it.copy(queue = it.queue.filterNot { p -> p.key == pending.key }) }
                }
            }
            restore()
            change { it.copy(message = when {
                it.deletionPending > 0 -> "Deletion is pending. Unknown receipts need the original account and server. Keep saved deletion details until confirmation or expiry."
                stopped && it.retainedCount == 0 -> "Route improvement is off. No saved contribution work remains on this device."
                stopped -> "Collection is off. Saved choices and deletion details remain in Profile."
                it.optedIn && it.paused -> "Route improvement is paused. Resume explicitly for this trip."
                it.optedIn -> "Route improvement is on for this revision and mode. Only qualified day-level counts can be sent."
                it.admitted -> "Route improvement is off. Navigation does not contribute location data."
                else -> it.message
            }) }
        } catch (e: Exception) {
            if (e is CancellationException) throw e
            if (e is ContributionStorageException) { storageFailed(); return@withLock }
            confirmed = null; pauseEpisode(); runCatching { restore() }
            change { it.copy(admitted = false, message = if (it.deletionPending > 0) "Deletion could not be confirmed. Local queues are cleared; keep deletion details and retry." else "Contribution work is pending. Collection stopped; retry with the original choice and keys, or erase saved contributions.") }
        }
    }
}
