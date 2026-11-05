package tv.subcult.museum.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable

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
    val syncedAt: Long? = null
) {
    val needsSync: Boolean
        get() = visitedPositions.isNotEmpty() && (syncedAt == null || syncedAt < updatedAt)

    companion object {
        fun key(tourId: String, mode: String) = "progress.$tourId.$mode"
    }
}

/** Which tour is being walked, kept on disk so a relaunch can resume it. */
@Serializable
data class ActiveTourRecord(val tourId: String, val mode: String) {
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
    val futureLegs: List<RouteLeg> = emptyList()
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
    private val scope: CoroutineScope
) {
    private val _state = MutableStateFlow(ActiveTourState())
    val state: StateFlow<ActiveTourState> = _state

    private val stops: List<TourStop> = tour.stops.sortedBy { it.position }
    private var progress: LocalProgress
    private var monitor: ArrivalMonitor? = null
    private val lock = Any()
    @Volatile private var chain: Job? = null

    init {
        progress = store.getJson<LocalProgress>(LocalProgress.key(tour.id, mode))
            ?: LocalProgress(tour.id, mode)
        store.putJson(tourKey(tour.id), tour)
        store.putJson(ActiveTourRecord.KEY, ActiveTourRecord(tour.id, mode))
        _state.update { it.copy(visited = progress.visitedPositions.toSet()) }
        advance()
    }

    val progressText: String get() = "${_state.value.visited.size} of ${stops.size} stops"
    val needsSync: Boolean get() = progress.needsSync

    // MARK: Route

    /** Loads the route from the API, falling back to the cached copy when the request fails. */
    suspend fun loadRoute() {
        _state.update { it.copy(routeStatus = LoadStatus.LOADING) }
        val key = routeKey(tour.id, mode)
        try {
            val fresh = api.route(tour.id, mode)
            if (fresh.status == "ready") store.putJson(key, fresh)
            _state.update {
                it.copy(
                    route = fresh, routeFromCache = false, routeStatus = LoadStatus.READY,
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
    }

    // MARK: Stops and arrival

    /** Feeds a location fix: updates the distance and lets the monitor decide on arrival. */
    fun handleFix(fix: Fix) {
        val arrived: TourStop?
        synchronized(lock) {
            val m = monitor
            _state.update { it.copy(lastFix = fix, distanceToNextM = m?.distance(fix)) }
            arrived = if (m != null && m.update(fix)) _state.value.nextStop else null
        }
        if (arrived != null) arrive(arrived)
    }

    /** "I'm here": arrival by hand when GPS cannot confirm it. */
    fun markArrived() {
        _state.value.nextStop?.let { arrive(it) }
    }

    private fun arrive(stop: TourStop) {
        synchronized(lock) {
            if (stop.position in _state.value.visited) return
            _state.update { it.copy(visited = it.visited + stop.position, arrivedStop = stop) }
            recordVisited()
        }
        sync()
    }

    /** Writes `visited` into the local record and advances to the next stop. Call under lock. */
    private fun recordVisited() {
        val visited = _state.value.visited.sorted()
        val complete = visited.size >= stops.size && stops.isNotEmpty()
        progress = progress.copy(
            visitedPositions = visited,
            updatedAt = maxOf(System.currentTimeMillis(), progress.updatedAt + 1),
            completedAt = if (complete) progress.completedAt ?: System.currentTimeMillis() else progress.completedAt
        )
        store.putJson(LocalProgress.key(tour.id, mode), progress)
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
            val point = Point(next.place.latitude, next.place.longitude)
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
                pastLegs = if (next == null) legs else legs.filter { l -> l.toPosition < next.position },
                activeLeg = if (next == null) null else legs.firstOrNull { l -> l.toPosition == next.position && l.fromPosition in visited },
                futureLegs = if (next == null) emptyList() else legs.filter { l -> l.toPosition > next.position }
            )
        }
    }

    // MARK: Progress sync

    /** Merges the server's record for this tour (any mode) into the local one by union. */
    suspend fun loadRemoteProgress() {
        if (!signedIn()) return
        val remote = try { api.progress().firstOrNull { it.tourId == tour.id } } catch (_: Exception) { null } ?: return
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
                store.putJson(LocalProgress.key(tour.id, mode), progress)
                changed = false
            }
        }
        if (changed || progress.needsSync) sync()
    }

    /** Sends progress the server is missing (after an offline stretch, on resume). */
    suspend fun syncIfNeeded() {
        if (!signedIn() || !progress.needsSync) return
        sync()
        flush()
    }

    /** PUTs progress one request at a time, in order, each carrying every stop known when it leaves. */
    private fun sync() {
        if (!signedIn()) return
        val previous = chain
        chain = scope.launch {
            previous?.join()
            val snapshot = synchronized(lock) { progress }
            val body = ProgressRequest(mode, snapshot.visitedPositions, snapshot.completedAt != null)
            try {
                api.putProgress(tour.id, body)
                synchronized(lock) {
                    progress = progress.copy(syncedAt = maxOf(snapshot.updatedAt, progress.syncedAt ?: 0))
                    store.putJson(LocalProgress.key(tour.id, mode), progress)
                }
                _state.update { it.copy(syncError = null) }
            } catch (e: Exception) {
                _state.update { it.copy(syncError = (e as? ApiException)?.message ?: e.message) }
            }
        }
    }

    /** Waits for in-flight progress posts (tests, and the end-tour button). */
    suspend fun flush() {
        chain?.join()
    }

    /** Ends the walk: the tour is no longer resumed on launch. Progress stays. */
    fun end() {
        store.remove(ActiveTourRecord.KEY)
    }

    /** Forgets local progress and, when signed in, clears the server record too. */
    fun resetProgress() {
        synchronized(lock) {
            progress = LocalProgress(tour.id, mode, updatedAt = maxOf(System.currentTimeMillis(), progress.updatedAt + 1))
            store.remove(LocalProgress.key(tour.id, mode))
            _state.update { it.copy(visited = emptySet(), arrivedStop = null) }
            advance()
        }
        if (signedIn()) {
            val previous = chain
            chain = scope.launch {
                previous?.join()
                try {
                    api.putProgress(tour.id, ProgressRequest(mode, emptyList(), false))
                    synchronized(lock) { progress = progress.copy(syncedAt = progress.updatedAt) }
                    _state.update { it.copy(syncError = null) }
                } catch (e: Exception) {
                    _state.update { it.copy(syncError = (e as? ApiException)?.message ?: e.message) }
                }
            }
        }
    }

    /** Turn-by-turn to the next stop in Google Maps; the app falls back to a geo: URI. */
    fun navigationUri(): String? = _state.value.nextStop?.let { navigationUri(it.place.latitude, it.place.longitude, mode) }

    fun geoUri(): String? = _state.value.nextStop?.let { geoUri(it.place.latitude, it.place.longitude, it.place.name) }

    companion object {
        fun tourKey(tourId: String) = "tour.$tourId"
        fun routeKey(tourId: String, mode: String) = "route.$tourId.$mode"

        /** Rebuilds the tour that was active when the app last ran, from the cached detail alone. */
        fun restore(api: Endpoints, store: KeyValueStore, signedIn: () -> Boolean, scope: CoroutineScope): ActiveTour? {
            val record = store.getJson<ActiveTourRecord>(ActiveTourRecord.KEY) ?: return null
            val tour = store.getJson<TourDetail>(tourKey(record.tourId)) ?: return null
            return ActiveTour(api, tour, record.mode, store, signedIn, scope)
        }
    }
}
