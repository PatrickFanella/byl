package tv.subcult.museum.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import tv.subcult.museum.AppContainer
import tv.subcult.museum.core.ActiveTour
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.City
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.Session
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

    init {
        container.client.onUnauthorized = { token -> handleUnauthorized(token) }
        viewModelScope.launch { container.session.state.collect { s -> _state.update { it.copy(session = s) } } }
        bootstrap()
    }

    private fun bootstrap() {
        viewModelScope.launch {
            if (container.session.restore()) {
                runCatching { api.me() }.onSuccess { container.session.update(it) }
                refreshSaved()
            }
            loadCities()
            resumeActiveTour()
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

    fun signOut() {
        viewModelScope.launch {
            runCatching { api.signOut() }
            container.session.clear()
            _state.update { it.copy(savedTourIds = emptySet()) }
        }
    }

    /** A 401 clears the session only when the rejected token is still current; sign-in is shown now or after the tour. */
    private fun handleUnauthorized(token: String?) {
        if (!container.session.isSignedIn) return
        if (token != null && token != container.session.token) return
        container.session.clear()
        _state.update { it.copy(savedTourIds = emptySet()) }
        if (_state.value.active == null) _state.update { it.copy(showSignIn = true) } else signInAfterTour = true
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

    fun startTour(tour: TourDetail, mode: String) {
        attach(ActiveTour(api, tour, mode, container.store, { container.session.isSignedIn }, container.scope))
    }

    private fun resumeActiveTour() {
        if (_state.value.active != null) return
        ActiveTour.restore(api, container.store, { container.session.isSignedIn }, container.scope)?.let { attach(it) }
    }

    private fun attach(tour: ActiveTour) {
        _state.update { it.copy(active = tour) }
        viewModelScope.launch {
            tour.loadRoute()
            tour.loadRemoteProgress()
            tour.syncIfNeeded()
        }
    }

    fun endTour(reset: Boolean) {
        val tour = _state.value.active ?: return
        viewModelScope.launch {
            if (reset) tour.resetProgress()
            tour.flush()
            tour.end()
            container.narration.stop()
            _state.update { it.copy(active = null) }
            if (signInAfterTour) {
                signInAfterTour = false
                _state.update { it.copy(showSignIn = true) }
            }
        }
    }

    fun onResume() {
        _state.value.active?.let { tour -> viewModelScope.launch { tour.syncIfNeeded() } }
    }

    // Audio guide

    fun setAudioGuide(on: Boolean) {
        container.audioGuideEnabled = on
        _state.update { it.copy(audioEnabled = on) }
        if (!on) container.narration.stop()
    }

    /** Plays the featured moment at a stop: recorded narration, else the body read aloud. */
    fun playNarration(stop: TourStop) {
        val featured = stop.featuredMoment ?: return
        viewModelScope.launch {
            val key = "moment.${featured.id}"
            val detail = runCatching { api.moment(featured.id) }.getOrNull()?.also { container.store.putJson(key, it) }
                ?: container.store.getJson(key)
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
