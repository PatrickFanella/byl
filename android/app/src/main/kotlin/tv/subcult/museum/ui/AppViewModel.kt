package tv.subcult.museum.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import tv.subcult.museum.AppContainer
import tv.subcult.museum.core.ApiClient
import tv.subcult.museum.core.Endpoints
import tv.subcult.museum.core.ActiveTour
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.City
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.Session
import tv.subcult.museum.core.RouteSuggestions
import tv.subcult.museum.core.TourDetail
import tv.subcult.museum.core.TourStop
import tv.subcult.museum.core.getJson
import tv.subcult.museum.core.planNarration
import tv.subcult.museum.core.putJson

data class AppState(
    val session: Session? = null,
    val cities: List<City> = emptyList(),
    val city: City? = null,
    val citiesStatus: LoadStatus = LoadStatus.IDLE,
    val citiesMessage: String = "",
    val savedTourIds: Set<String> = emptySet(),
    val active: ActiveTour? = null,
    val contributions: tv.subcult.museum.core.TripContributions? = null,
    val contributionError: String? = null,
    val pendingTourSlug: String? = null,
    val showSignIn: Boolean = false,
    val audioEnabled: Boolean = false
) {
    val isSignedIn: Boolean get() = session != null
}

/** App-wide state: session, cities, the saved set, the tour being walked, and sign-in. */
class AppViewModel(val container: AppContainer) : ViewModel() {
    private val _state = MutableStateFlow(AppState(audioEnabled = container.audioGuideEnabled))
    val state: StateFlow<AppState> = _state
    val api get() = container.api
    private var signInAfterTour = false
    private var narrationJob: Job? = null
    private var endingProgress: Job? = null
    private var foreground = true

    init {
        container.client.onUnauthorized = { token -> handleUnauthorized(token) }
        viewModelScope.launch { container.session.state.collect { s -> _state.value.contributions?.identityChanged(); _state.update { it.copy(session = s) } } }
        // Disk only, before any network: an interrupted walk is back before the first frame.
        container.session.restore()
        resumeActiveTour()
        bootstrap()
    }

    private fun bootstrap() {
        viewModelScope.launch {
            if (container.session.isSignedIn) {
                runCatching { api.me() }.onSuccess { container.session.update(it) }
                refreshSaved()
            }
            loadCities()
        }
    }

    fun loadCities() {
        viewModelScope.launch {
            _state.update { it.copy(citiesStatus = LoadStatus.LOADING) }
            try {
                val cities = api.cities()
                container.store.putJson("cities", cities)
                select(cities)
                _state.update { it.copy(citiesStatus = LoadStatus.READY, citiesMessage = "") }
            } catch (e: Exception) {
                val cached = container.store.getJson<List<City>>("cities")
                if (!cached.isNullOrEmpty()) select(cached)
                _state.update { it.copy(citiesStatus = if (cached.isNullOrEmpty()) LoadStatus.ERROR else LoadStatus.READY, citiesMessage = (e as? ApiException)?.message ?: e.message ?: "Could not load cities.") }
            }
        }
    }

    private fun select(cities: List<City>) {
        val remembered = container.citySlug
        val current = cities.firstOrNull { it.slug == remembered } ?: _state.value.city ?: cities.firstOrNull()
        _state.update { it.copy(cities = cities, city = current) }
    }

    fun selectCity(city: City) {
        container.citySlug = city.slug
        _state.update { it.copy(city = city) }
    }

    // Session

    fun signIn(session: Session) {
        container.session.set(session)
        _state.update { it.copy(showSignIn = false) }
        viewModelScope.launch { refreshSaved() }
    }

    fun showSignIn(show: Boolean) = _state.update { it.copy(showSignIn = show) }

    /** Bind suggestion requests to the session that opened this screen. */
    fun routeSuggestions(owner: Session): RouteSuggestions {
        val origin = container.client.baseUrl
        val client = ApiClient(origin) { owner.token }
        client.onUnauthorized = { token -> if (container.client.baseUrl == origin) handleUnauthorized(token) }
        return RouteSuggestions(Endpoints(client), container.store, owner.account.id) {
            val current = container.session.state.value
            current != null && current.account.id == owner.account.id && current.token == owner.token && container.client.baseUrl == origin
        }
    }

    fun routeReportApi(owner: Session): Endpoints {
        val origin = container.client.baseUrl
        val client = ApiClient(origin) { owner.token }
        client.onUnauthorized = { token -> if (container.client.baseUrl == origin) handleUnauthorized(token) }
        return Endpoints(client)
    }

    fun routeReports(owner: Session, context: tv.subcult.museum.core.RouteReportContext? = null, initialTarget: String? = null): tv.subcult.museum.core.RouteReports {
        val origin = container.client.baseUrl
        return tv.subcult.museum.core.RouteReports(routeReportApi(owner), container.reportStore, owner.account.id, context, initialTarget) {
            val current = container.session.state.value
            current != null && current.account.id == owner.account.id && current.token == owner.token && container.client.baseUrl == origin
        }
    }

    fun signOut() {
        val token = container.session.token
        val previousApi = Endpoints(ApiClient(container.client.baseUrl) { token })
        container.session.clear()
        _state.update { it.copy(savedTourIds = emptySet()) }
        if (token != null) viewModelScope.launch { runCatching { previousApi.signOut() } }
    }

    /**
     * Runs on OkHttp's thread. The session store decides atomically whether the rejected token is
     * still the current one (a stale 401 after a fresh sign-in changes nothing); UI state moves
     * to the main thread. Sign-in is shown now, or after the active tour closes.
     */
    private fun handleUnauthorized(token: String?) {
        if (!container.session.clearIfToken(token)) return
        viewModelScope.launch {
            _state.update { it.copy(savedTourIds = emptySet()) }
            if (_state.value.active == null) _state.update { it.copy(showSignIn = true) } else signInAfterTour = true
        }
    }

    suspend fun refreshSaved() {
        if (!container.session.isSignedIn) return
        runCatching { api.saved() }.onSuccess { saved ->
            _state.update { it.copy(savedTourIds = saved.flatMap { t -> listOf(t.id, t.slug) }.toSet()) }
        }
    }

    fun setSaved(tourId: String, saved: Boolean) = _state.update {
        it.copy(savedTourIds = if (saved) it.savedTourIds + tourId else it.savedTourIds - tourId)
    }

    // Active tour

    suspend fun startTour(tour: TourDetail, mode: String) {
        // Finish the previous trip's queued writes before starting another progress generation.
        endingProgress?.join()
        val owner = container.session.state.value
        val accountId = owner?.account?.id
        val origin = container.client.baseUrl.trimEnd('/')
        val revisionId = tour.revisionId ?: throw ApiException(0, "revision_unavailable", "This tour has no reviewed revision to start.")
        val api = activeTourApi(owner)
        val revision = api.revision(tour.id, revisionId)
        if (!sameSession(owner, origin) || _state.value.active != null || revision.id != revisionId || revision.tourId != tour.id)
            throw ApiException(0, "trip_changed", "The trip or session changed. Open the tour again before starting.")
        val active = ActiveTour.startPinned(api, revision, mode, container.store, { owner != null && sameSession(owner, origin) }, container.scope, accountId) { sameSession(owner, origin) }
        if (!sameSession(owner, origin)) throw ApiException(0, "trip_changed", "The session changed before this trip could open.")
        attach(active)
    }

    suspend fun adoptUpdate(expected: ActiveTour) {
        if (_state.value.active !== expected) throw ApiException(0, "trip_changed", "The active trip changed. Review it again.")
        val replacement = expected.adoptingUpdate()
        if (_state.value.active !== expected || !replacement.canUse) throw ApiException(0, "trip_changed", "The trip or session changed. Review it again.")
        expected.end()
        narrationJob?.cancel()
        container.narration.stop()
        attach(replacement)
    }

    private fun sameSession(owner: Session?, origin: String): Boolean {
        val current = container.session.state.value
        return current?.account?.id == owner?.account?.id && current?.token == owner?.token && container.client.baseUrl.trimEnd('/') == origin
    }

    private fun resumeActiveTour() {
        if (_state.value.active != null) return
        val owner = container.session.state.value
        val accountId = owner?.account?.id
        val origin = container.client.baseUrl.trimEnd('/')
        runCatching {
            ActiveTour.restore(activeTourApi(owner), container.store, { owner != null && sameSession(owner, origin) }, container.scope, accountId) { sameSession(owner, origin) }
        }.getOrNull()?.let { attach(it) }
    }

    private fun activeTourApi(owner: Session?): Endpoints {
        val origin = container.client.baseUrl.trimEnd('/')
        val client = ApiClient(origin) { owner?.token }
        client.onUnauthorized = { token -> if (container.client.baseUrl.trimEnd('/') == origin) handleUnauthorized(token) }
        return Endpoints(client)
    }

    fun contributionModel(tour: ActiveTour? = null): tv.subcult.museum.core.TripContributions? {
        return try {
            val session = container.session.state.value
            val owner = if (tour == null) session?.account?.id else tour.accountId
            val origin = tour?.origin ?: container.client.baseUrl.trimEnd('/')
            val token = session?.token
            tv.subcult.museum.core.TripContributions(container.contributionStore,
                tv.subcult.museum.core.ContributionHttpApi(origin, token), owner, origin, tour?.revision, tour?.mode ?: "walk", {
                    val current = container.session.state.value
                    owner != null && current?.account?.id == owner && current.token == token && container.client.baseUrl.trimEnd('/') == origin && (tour == null || tour.canUse)
                }).also { _state.update { it.copy(contributionError = null) } }
        } catch (_: Exception) {
            _state.update { it.copy(contributionError = "Private contribution storage is unavailable. Route improvement is off; navigation still works.") }
            null
        }
    }

    private fun attach(tour: ActiveTour) {
        _state.value.contributions?.lifecycle(tv.subcult.museum.core.DeviationDetector.Control.END)
        _state.value.contributions?.setForeground(false)
        val contributions = contributionModel(tour)
        _state.update { it.copy(active = tour, contributions = contributions) }
        viewModelScope.launch {
            tour.loadRoute()
            if (tour.state.value.revoked) container.narration.stop()
            tour.loadRemoteProgress()
            tour.syncIfNeeded()
            tour.checkForUpdate()
        }
    }

    /** Ends the walk now (state and resume record first) and lets any pending PUT finish behind it. */
    fun endTour(reset: Boolean): Boolean {
        val tour = _state.value.active ?: return true
        _state.value.contributions?.lifecycle(tv.subcult.museum.core.DeviationDetector.Control.END)
        _state.value.contributions?.setForeground(false)
        if (reset) tour.resetProgress()
        if (!tour.end()) return false
        narrationJob?.cancel()
        container.narration.stop()
        _state.update { if (it.active === tour) it.copy(active = null, contributions = null) else it }
        if (signInAfterTour) {
            signInAfterTour = false
            _state.update { it.copy(showSignIn = true) }
        }
        endingProgress = container.scope.launch { tour.flush() }
        return true
    }

    fun onResume() {
        foreground = true
        _state.value.active?.let { tour -> viewModelScope.launch {
            tour.loadRoute()
            if (!tour.canUse) container.narration.stop()
            tour.syncIfNeeded()
        } }
    }

    /** The activity left the foreground: no narration and no GPS until it is back. */
    fun onStop() {
        foreground = false
        _state.value.contributions?.setForeground(false)
        narrationJob?.cancel()
        container.narration.stop()
        container.location.stop()
    }

    // Audio guide

    fun setAudioGuide(on: Boolean) {
        container.audioGuideEnabled = on
        _state.update { it.copy(audioEnabled = on) }
        if (!on) {
            narrationJob?.cancel()
            container.narration.stop()
        }
    }

    /** Plays the featured moment at a stop: recorded narration, else the body read aloud. */
    fun playNarration(stop: TourStop, requireGuide: Boolean = false) {
        val expected = _state.value.active ?: return
        if (!expected.canUse) return
        val featured = stop.featuredMoment ?: return
        narrationJob?.cancel()
        narrationJob = viewModelScope.launch {
            val key = "moment.${featured.id}"
            val detail = if (expected.revision != null || expected.state.value.legacyResume) expected.retainedMoment(featured.id)
                else runCatching { api.moment(featured.id) }.getOrNull()?.also { container.store.putJson(key, it) } ?: container.store.getJson(key)
            coroutineContext.ensureActive()
            if (!foreground || _state.value.active !== expected || !expected.canUse || (requireGuide && !_state.value.audioEnabled) || expected.state.value.arrivedStop?.id != stop.id) return@launch
            container.narration.play(planNarration(detail?.narration?.publicUrl, detail?.body ?: featured.excerpt))
        }
    }

    fun openTourLink(slug: String) = _state.update { it.copy(pendingTourSlug = slug) }
    fun consumeTourLink() = _state.update { it.copy(pendingTourSlug = null) }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = AppViewModel(container) as T
    }
}
