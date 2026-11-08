package tv.subcult.museum.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.delay
import kotlinx.serialization.Serializable
import kotlinx.coroutines.CancellationException
import java.security.MessageDigest

enum class LoadStatus { IDLE, LOADING, READY, ERROR }

/** Progress on one tour kept on the device, so a walk continues without signal and resumes later. */
@Serializable
data class LocalProgress(
    val tourId: String,
    val mode: String,
    val visitedPositions: List<Int> = emptyList(),
    val startedAt: Long = System.currentTimeMillis(),
    val completedAt: Long? = null,
    /** Strictly increasing per change; compared with syncedAt. */
    val updatedAt: Long = 0,
    /** The updatedAt the server last accepted; null until a PUT succeeds. */
    val syncedAt: Long? = null,
    val resetPending: Boolean = false,
    val revisionId: String? = null,
    val visitedStopIds: List<String> = emptyList()
) {
    val needsSync: Boolean
        get() = resetPending || (visitedPositions.isNotEmpty() && (syncedAt == null || syncedAt < updatedAt))

    companion object {
        fun key(tourId: String, mode: String, accountId: String? = null) = "progress.v2.${accountId ?: "anonymous"}.$tourId"
    }
}

/** Which tour is being walked, kept on disk so a relaunch can resume it. */
@Serializable
data class ActiveTourRecord(val tourId: String, val mode: String, val accountId: String? = null, val revisionId: String? = null, val origin: String? = null) {
    companion object {
        const val KEY = "active"
    }
}

data class ActiveTourState(
    val route: TourRoute? = null,
    val routeStatus: LoadStatus = LoadStatus.IDLE,
    val routeMessage: String? = null,
    val routeFromCache: Boolean = false,
    val visited: Set<Int> = emptySet(),
    val nextStop: TourStop? = null,
    /** The stop just reached, for the arrival sheet; the UI clears it when dismissed. */
    val arrivedStop: TourStop? = null,
    val distanceToNextM: Double? = null,
    val lastFix: Fix? = null,
    val syncError: String? = null,
    val isComplete: Boolean = false,
    val pastLegs: List<RouteLeg> = emptyList(),
    val activeLeg: RouteLeg? = null,
    val futureLegs: List<RouteLeg> = emptyList(),
    val revoked: Boolean = false,
    val legacyResume: Boolean = false,
    val availableUpdate: TourRevision? = null,
    val checkingUpdate: Boolean = false,
    val updateError: String? = null
)

/**
 * Drives a tour being walked or ridden: the route (API with cached fallback), the next stop,
 * arrival by GPS or by hand, local progress merged with the server, and the legs for the map.
 */
class ActiveTour(
    private val api: Endpoints,
    val tour: TourDetail,
    val mode: String,
    private val store: KeyValueStore,
    private val signedIn: () -> Boolean,
    private val scope: CoroutineScope,
    val accountId: String? = null,
    val revision: TourRevision? = null,
    private val isCurrent: () -> Boolean = { true }
) {
    private val _state = MutableStateFlow(ActiveTourState())
    val state: StateFlow<ActiveTourState> = _state

    private val stops: List<TourStop> = tour.stops.sortedBy { it.position }
    val origin = api.client.baseUrl.trimEnd('/')
    private val progressKey = revision?.let { progressKey(api, accountId, it.id) } ?: LocalProgress.key(tour.id, mode, accountId)
    private val snapshotKey = revisionKey(api, accountId, revision?.id ?: "legacy")
    @Volatile private var progress: LocalProgress
    private var monitor: ArrivalMonitor? = null
    private val lock = Any()
    @Volatile private var chain: Job? = null
    private var routePoll: Job? = null
    private var routePolls = 0
    @Volatile private var ended = false
    @Volatile private var revoked = false
    private var legacyResume = false
    val canUse: Boolean get() = !ended && !revoked && isCurrent()
    private fun canSync() = !revoked && !legacyResume && signedIn() && isCurrent()

    init {
        if (revision != null) {
            revision.unavailableReason(mode)?.let { throw ApiException(0, "revision_unavailable", it) }
            if (revision.snapshot?.tour != tour) throw ApiException(0, "revision_unavailable", "The retained tour does not match this revision.")
            store.putJson(snapshotKey, revision)
        }
        progress = store.getJson<LocalProgress>(progressKey)
            ?: LocalProgress(tour.id, mode)
        if (revision != null) {
            val ids = if (progress.tourId == tour.id && progress.revisionId == revision.id) progress.visitedStopIds.toSet() else emptySet()
            val visited = stops.filter { it.id in ids }.map { it.position }.toSet()
            progress = progress.copy(tourId = tour.id, revisionId = revision.id, visitedStopIds = stops.filter { it.position in visited }.map { it.id }, visitedPositions = visited.sorted(), completedAt = progress.completedAt.takeIf { visited.size == stops.size })
            _state.value = ActiveTourState(visited = visited, route = revision.snapshot!!.routes[mode], routeStatus = LoadStatus.READY, routeFromCache = true)
        } else {
            store.putJson(tourKey(tour.id), tour)
            _state.update { it.copy(visited = progress.visitedPositions.toSet()) }
        }
        advance()
        store.putJson(ActiveTourRecord.KEY, ActiveTourRecord(tour.id, mode, accountId, revision?.id, origin))
    }

    val progressText: String get() = "${_state.value.visited.size} of ${stops.size} stops"
    val needsSync: Boolean get() = progress.needsSync
    val visitedStopIds: Set<String> get() = stops.filter { it.position in _state.value.visited }.map { it.id }.toSet()

    fun retainedMoment(id: String): MomentDetail? = if (canUse) revision?.snapshot?.moments?.get(id) else null

    fun navigationPoint(stop: TourStop): Point? = when {
        !canUse -> null
        revision != null -> stop.place.arrivalPoint?.navigationPoint(mode)
        else -> Point(stop.place.latitude, stop.place.longitude)
    }

    suspend fun checkForUpdate() {
        val original = revision ?: return
        synchronized(lock) {
            if (!canUse || _state.value.checkingUpdate) return
            _state.update { it.copy(checkingUpdate = true, updateError = null) }
        }
        try {
            val latest = api.latestRevision(tour.id) ?: throw ApiException(0, "unavailable", "No reviewed revision")
            if (!canUse) return
            val id = latest.id
            if (latest.tourId != tour.id) throw ApiException(0, "unavailable", "Wrong tour")
            if (id == original.id) {
                _state.update { if (!canUse) it else it.copy(availableUpdate = null) }
                return
            }
            val candidate = api.revision(tour.id, id)
            if (!canUse) return
            if (candidate.id != id || candidate.tourId != tour.id || candidate.number <= original.number) throw ApiException(0, "unavailable", "Update not confirmed")
            _state.update { if (!canUse) it else it.copy(availableUpdate = candidate, updateError = candidate.unavailableReason(mode)) }
        } catch (e: CancellationException) { throw e }
        catch (_: Exception) {
            _state.update { if (!canUse) it else it.copy(updateError = "Could not check for an update. Your started itinerary is unchanged.") }
        } finally {
            _state.update { it.copy(checkingUpdate = false) }
        }
    }

    /** Called only after review/confirmation; rechecks visibility before changing local records. */
    suspend fun adoptingUpdate(): ActiveTour {
        val target = _state.value.availableUpdate ?: throw ApiException(0, "unavailable", "Check for a reviewed update first.")
        val confirmed = api.revision(tour.id, target.id)
        flush()
        synchronized(lock) {
            if (!canUse || _state.value.availableUpdate?.id != target.id || confirmed.id != target.id || confirmed.tourId != tour.id || confirmed.unavailableReason(mode) != null || target.unavailableReason(mode) != null)
                throw ApiException(0, "unavailable", "The trip, session or update changed. Review it again.")
            val nextStops = target.snapshot!!.tour.stops
            val oldById = stops.associateBy { it.id }
            val carry = nextStops.filter { stop ->
                val old = oldById[stop.id]
                stop.id in visitedStopIds && old != null && old.place.id == stop.place.id && old.place.arrivalPoint == stop.place.arrivalPoint && old.featuredMoment?.id == stop.featuredMoment?.id
            }.map { it.id }.toSet()
            val key = progressKey(api, accountId, target.id)
            val old = store.getJson<LocalProgress>(key)?.takeIf { it.tourId == tour.id && it.revisionId == target.id }
            val known = nextStops.map { it.id }.toSet()
            val existing = if (old?.resetPending == false) old.visitedStopIds.toSet().intersect(known) else emptySet()
            val ids = existing + carry
            val changed = maxOf(System.currentTimeMillis(), (old?.updatedAt ?: 0) + 1)
            store.putJson(key, (old ?: LocalProgress(tour.id, mode)).copy(revisionId = target.id, visitedStopIds = ids.sorted(), visitedPositions = nextStops.filter { it.id in ids }.map { it.position }.sorted(), updatedAt = changed, syncedAt = null, completedAt = if (ids == known) changed else null))
            return startPinned(api, target, mode, store, signedIn, scope, accountId, isCurrent)
        }
    }

    private fun revoke() {
        synchronized(lock) {
            revoked = true
            monitor = null
            _state.value = ActiveTourState(revoked = true, routeStatus = LoadStatus.ERROR, routeMessage = "This revision is no longer available to this account.")
            runCatching { store.remove(snapshotKey) }
            runCatching { store.remove(progressKey) }
            end()
        }
    }

    // MARK: Route

    /** Loads the route from the API, falling back to the cached copy when the request fails. */
    suspend fun loadRoute() {
        if (!canUse) return
        if (revision != null) {
            try {
                val retained = api.revision(tour.id, revision.id)
                if (!canUse) return
                if (retained.id != revision.id || retained.tourId != tour.id) throw ApiException(0, "unavailable", "Saved revision not confirmed")
                _state.update { if (!canUse) it else it.copy(routeFromCache = false) }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                if (!canUse) return
                if (e is ApiException && e.status in listOf(403, 404)) revoke()
                else _state.update { if (!canUse) it else it.copy(routeFromCache = true) }
            }
            return
        }
        if (legacyResume) {
            val cached = store.getJson<TourRoute>(routeKey(tour.id, mode))
            _state.update { it.copy(route = cached, routeFromCache = cached != null, routeStatus = if (cached == null) LoadStatus.ERROR else LoadStatus.READY, routeMessage = if (cached == null) "This older trip has no saved route. End it to choose a reviewed revision when connected." else null) }
            recomputeLegs()
            return
        }
        _state.update { it.copy(routeStatus = LoadStatus.LOADING) }
        val key = routeKey(tour.id, mode)
        try {
            val fresh = api.route(tour.id, mode)
            if (fresh.status == "ready") store.putJson(key, fresh)
            val cached = store.getJson<TourRoute>(key)?.takeIf { it.status == "ready" }
            val useCached = fresh.status != "ready" && cached != null
            _state.update {
                it.copy(
                    route = if (useCached) cached else fresh, routeFromCache = useCached, routeStatus = LoadStatus.READY,
                    routeMessage = when (fresh.status) { "ready" -> null; "failed" -> "Route unavailable"; else -> "Route being computed" }
                )
            }
        } catch (e: Exception) {
            val cached = store.getJson<TourRoute>(key)
            if (cached != null) {
                _state.update { it.copy(route = cached, routeFromCache = true, routeStatus = LoadStatus.READY, routeMessage = null) }
            } else {
                _state.update { it.copy(route = null, routeStatus = LoadStatus.ERROR, routeMessage = (e as? ApiException)?.message ?: e.message ?: "Route unavailable") }
            }
        }
        recomputeLegs()
        if (_state.value.routeMessage == "Route being computed" && routePolls < 30 && !ended) {
            routePolls++
            routePoll?.cancel()
            routePoll = scope.launch { delay(2000); loadRoute() }
        }
    }

    // MARK: Stops and arrival

    /** Feeds a location fix: updates the distance and lets the monitor decide on arrival. */
    fun handleFix(fix: Fix) {
        if (!canUse) return
        val arrived: TourStop?
        synchronized(lock) {
            val m = monitor
            _state.update { it.copy(lastFix = fix, distanceToNextM = m?.distance(fix)) }
            arrived = if (m != null && m.update(fix)) _state.value.nextStop else null
        }
        if (arrived != null) arrive(arrived)
    }

    /** The UI showed the arrival sheet for `arrivedStop`; clear it so rotation does not replay it. */
    fun acknowledgeArrival() {
        _state.update { it.copy(arrivedStop = null) }
    }

    /** "I'm here": arrival by hand when GPS cannot confirm it. */
    fun markArrived() {
        _state.value.nextStop?.let { arrive(it) }
    }

    private fun arrive(stop: TourStop) {
        synchronized(lock) {
            if (!canUse || stop.position in _state.value.visited) return
            _state.update { it.copy(visited = it.visited + stop.position, arrivedStop = stop) }
            recordVisited()
        }
        sync()
    }

    /** Writes `visited` into the local record and advances to the next stop. Call under lock. */
    private fun recordVisited() {
        synchronized(store) {
            val stored = store.getJson<LocalProgress>(progressKey)
            val savedPositions = if (revision == null) stored?.visitedPositions ?: emptyList()
                else if (stored?.revisionId == revision.id && stored.tourId == tour.id && stored.startedAt == progress.startedAt) stops.filter { it.id in stored.visitedStopIds }.map { it.position } else emptyList()
            val visited = (_state.value.visited + savedPositions).filter { position -> stops.any { it.position == position } }.sorted()
            _state.update { it.copy(visited = visited.toSet()) }
            val complete = visited.size >= stops.size && stops.isNotEmpty()
            progress = progress.copy(
                visitedPositions = visited,
                updatedAt = maxOf(System.currentTimeMillis(), progress.updatedAt + 1, stored?.updatedAt ?: 0),
                completedAt = if (complete) progress.completedAt ?: System.currentTimeMillis() else null,
                visitedStopIds = if (revision != null) stops.filter { it.position in visited }.map { it.id } else emptyList()
            )
            saveProgress()
        }
        advance()
    }

    /** Points `nextStop` and the monitor at the first unvisited stop. */
    private fun advance() {
        val visited = _state.value.visited
        val next = stops.firstOrNull { it.position !in visited }
        if (next == null) {
            monitor = null
            _state.update { it.copy(nextStop = null, distanceToNextM = null, isComplete = stops.isNotEmpty()) }
        } else {
            val point = navigationPoint(next) ?: return
            val m = monitor?.also { it.retarget(point, next.place.arrivalRadiusM) } ?: ArrivalMonitor(point, next.place.arrivalRadiusM).also { monitor = it }
            val fix = _state.value.lastFix
            _state.update { it.copy(nextStop = next, isComplete = false, distanceToNextM = fix?.let { f -> m.distance(f) }) }
        }
        recomputeLegs()
    }

    private fun recomputeLegs() {
        val legs = _state.value.route?.legs ?: emptyList()
        val next = _state.value.nextStop
        val visited = _state.value.visited
        _state.update {
            it.copy(
                pastLegs = if (next == null) legs else legs.filter { l -> l.fromPosition < l.toPosition && l.toPosition < next.position },
                activeLeg = if (next == null) null else legs.firstOrNull { l -> l.toPosition == next.position && l.fromPosition in visited },
                futureLegs = if (next == null) emptyList() else legs.filter { l -> l.toPosition > next.position || l.toPosition < l.fromPosition }
            )
        }
    }

    // MARK: Progress sync

    /** Merges the server's record for this tour (any mode) into the local one by union. */
    suspend fun loadRemoteProgress() {
        if (!canUse || !canSync()) return
        if (progress.resetPending) {
            syncIfNeeded()
            return
        }
        val generation = progress.startedAt
        if (revision != null) {
            try {
                val remote = api.revisionProgress(tour.id, revision.id) ?: return
                if (!canUse || !canSync()) return
                var changed = false
                synchronized(lock) {
                    if (!canSync() || progress.startedAt != generation || progress.resetPending || remote.accountId != accountId || remote.tourId != tour.id || remote.revisionId != revision.id) return
                    val positions = stops.filter { it.id in remote.visitedStopIds }.map { it.position }.toSet()
                    val merged = _state.value.visited + positions
                    if (merged != _state.value.visited) {
                        _state.update { it.copy(visited = merged) }
                        recordVisited()
                    }
                    if (positions == merged) {
                        progress = progress.copy(syncedAt = progress.updatedAt)
                        saveProgress()
                    } else changed = true
                }
                if (changed) sync()
            } catch (e: CancellationException) { throw e }
            catch (e: ApiException) {
                if (e.status == 404) return
                _state.update { if (!canUse) it else it.copy(syncError = "Could not read progress for this revision.") }
            }
            catch (_: Exception) { _state.update { if (!canUse) it else it.copy(syncError = "Could not read progress for this revision.") } }
            return
        }
        val remote = try { api.progress().firstOrNull { it.tourId == tour.id } } catch (_: Exception) { null } ?: return
        if (!signedIn() || progress.startedAt != generation) return
        var changed = false
        synchronized(lock) {
            val merged = _state.value.visited + remote.visitedPositions
            if (merged != _state.value.visited) {
                changed = true
                _state.update { it.copy(visited = merged) }
                recordVisited()
            }
            if (remote.visitedPositions.toSet() == merged) {
                progress = progress.copy(syncedAt = progress.updatedAt)
                store.putJson(progressKey, progress)
                changed = false
            }
        }
        if (changed || progress.needsSync) sync()
    }

    /** Sends progress the server is missing (after an offline stretch, on resume). */
    suspend fun syncIfNeeded() {
        if (!canSync() || !progress.needsSync) return
        sync()
        flush()
    }

    /** PUTs progress one request at a time, in order, each carrying every stop known when it leaves. */
    private fun sync() {
        if (!canSync()) return
        enqueue { snapshotAndPut() }
    }

    /** Chains a job after the previous one; the swap happens under the lock. */
    private fun enqueue(block: suspend () -> Unit) {
        synchronized(lock) {
            val previous = chain
            chain = scope.launch {
                previous?.join()
                block()
            }
        }
    }

    private suspend fun snapshotAndPut() {
        run {
            val snapshot = synchronized(lock) { progress }
            if (!canSync() || !snapshot.needsSync) return
            val body = ProgressRequest(mode, snapshot.visitedPositions, snapshot.completedAt != null)
            try {
                if (snapshot.resetPending) {
                    if (revision != null) api.putRevisionProgress(tour.id, revision.id, RevisionProgressRequest(mode, emptyList(), false, reset = true))
                    else api.putProgress(tour.id, ProgressRequest(mode, emptyList(), false, reset = true))
                    synchronized(lock) {
                        if (!canSync() || progress.startedAt != snapshot.startedAt) return
                        progress = progress.copy(resetPending = false)
                    }
                }
                if (!canSync() || progress.startedAt != snapshot.startedAt) return
                if (snapshot.visitedPositions.isNotEmpty()) {
                    if (revision != null) api.putRevisionProgress(tour.id, revision.id, RevisionProgressRequest(mode, snapshot.visitedStopIds, snapshot.completedAt != null))
                    else api.putProgress(tour.id, body)
                }
                synchronized(lock) {
                    if (!canSync() || progress.startedAt != snapshot.startedAt) return
                    progress = progress.copy(syncedAt = maxOf(snapshot.updatedAt, progress.syncedAt ?: 0))
                    saveProgress()
                }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                _state.update { if (!canSync()) it else it.copy(syncError = (e as? ApiException)?.message ?: e.message) }
            }
        }
    }

    /** Waits for in-flight progress posts (tests, and the end-tour button). */
    suspend fun flush() {
        chain?.join()
    }

    /** Ends the walk: the tour is no longer resumed on launch (only if the record still names this walk). Progress stays. */
    fun end(): Boolean {
        try {
            val record = store.getJson<ActiveTourRecord>(ActiveTourRecord.KEY)
            if (record == null || (record.tourId == tour.id && record.mode == mode && record.accountId == accountId && record.revisionId == revision?.id && (record.origin == null || record.origin == origin))) store.remove(ActiveTourRecord.KEY)
        } catch (_: Exception) {
            _state.update { it.copy(syncError = "The trip could not be closed on this device. Try ending it again.") }
            return false
        }
        ended = true
        routePoll?.cancel()
        return true
    }

    /** Forgets local progress and, when signed in, clears the server record too. */
    fun resetProgress() {
        if (!canUse) return
        synchronized(lock) {
            val changedAt = maxOf(System.currentTimeMillis(), progress.updatedAt + 1)
            progress = LocalProgress(tour.id, mode, startedAt = changedAt, updatedAt = changedAt, resetPending = canSync(), revisionId = revision?.id)
            saveProgress()
            _state.update { it.copy(visited = emptySet(), arrivedStop = null) }
            advance()
        }
        sync()
    }

    /** Turn-by-turn to the next stop in Google Maps; the app falls back to a geo: URI. */
    fun navigationUri(): String? = _state.value.nextStop?.let { stop -> navigationPoint(stop)?.let { navigationUri(it.lat, it.lng, mode) } }

    fun geoUri(): String? = _state.value.nextStop?.let { stop -> navigationPoint(stop)?.let { geoUri(it.lat, it.lng, stop.place.name) } }

    private fun saveProgress() {
        try {
            store.putJson(progressKey, progress)
            _state.update { it.copy(syncError = null) }
        } catch (_: Exception) {
            _state.update { it.copy(syncError = "Progress could not be saved on this device. Keep the trip open and try syncing when connected.") }
        }
    }

    companion object {
        fun tourKey(tourId: String) = "tour.$tourId"
        fun routeKey(tourId: String, mode: String) = "route.$tourId.$mode"

        private fun namespace(api: Endpoints, accountId: String?): String {
            val hash = MessageDigest.getInstance("SHA-256").digest(api.client.baseUrl.trimEnd('/').toByteArray()).joinToString("") { "%02x".format(it) }
            return "$hash.${accountId ?: "anonymous"}"
        }
        fun revisionKey(api: Endpoints, accountId: String?, revisionId: String) = "revision.v1.${namespace(api, accountId)}.$revisionId"
        private fun progressKey(api: Endpoints, accountId: String?, revisionId: String) = "progress.v3.${namespace(api, accountId)}.$revisionId"

        fun startPinned(api: Endpoints, revision: TourRevision, mode: String, store: KeyValueStore, signedIn: () -> Boolean, scope: CoroutineScope, accountId: String? = null, isCurrent: () -> Boolean = { true }): ActiveTour {
            revision.unavailableReason(mode)?.let { throw ApiException(0, "revision_unavailable", it) }
            return ActiveTour(api, revision.snapshot!!.tour, mode, store, signedIn, scope, accountId, revision, isCurrent)
        }

        /** Rebuilds the tour that was active when the app last ran, from the cached detail alone. */
        fun restore(api: Endpoints, store: KeyValueStore, signedIn: () -> Boolean, scope: CoroutineScope, accountId: String? = null, isCurrent: () -> Boolean = { true }): ActiveTour? {
            val record = store.getJson<ActiveTourRecord>(ActiveTourRecord.KEY) ?: return null
            if (record.accountId != accountId || (record.origin != null && record.origin != api.client.baseUrl.trimEnd('/'))) return null
            if (record.revisionId != null) {
                val revision = store.getJson<TourRevision>(revisionKey(api, accountId, record.revisionId)) ?: return null
                if (revision.id != record.revisionId || revision.tourId != record.tourId) return null
                return runCatching { startPinned(api, revision, record.mode, store, signedIn, scope, accountId, isCurrent) }.getOrNull()
            }
            val tour = store.getJson<TourDetail>(tourKey(record.tourId)) ?: return null
            return ActiveTour(api, tour, record.mode, store, signedIn, scope, accountId, isCurrent = isCurrent).also {
                it.legacyResume = true
                it._state.update { state -> state.copy(legacyResume = true) }
            }
        }
    }
}
