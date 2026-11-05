package tv.subcult.museum.ui.routes

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.City
import tv.subcult.museum.core.Endpoints
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.PlaceSummary
import tv.subcult.museum.core.RouteBuilder
import tv.subcult.museum.core.TourDetail
import tv.subcult.museum.ui.components.UiState

data class RouteBuilderUi(
    val editing: TourDetail? = null,
    val editingError: String? = null,
    val builder: RouteBuilder? = null,
    val places: UiState<List<PlaceSummary>> = UiState(LoadStatus.IDLE),
    val created: TourDetail? = null
)

/** Keeps the draft (title, ordered stops, modes) across rotation and tab switches. */
class RouteBuilderViewModel(private val api: Endpoints, private val editingId: String?) : ViewModel() {
    private val _ui = MutableStateFlow(RouteBuilderUi())
    val ui: StateFlow<RouteBuilderUi> = _ui

    init {
        if (editingId != null) {
            viewModelScope.launch {
                try {
                    _ui.update { it.copy(editing = api.tour(editingId)) }
                } catch (e: Exception) {
                    _ui.update { it.copy(editingError = (e as? ApiException)?.message ?: "Could not load the route.") }
                }
            }
        }
    }

    /** Builds the RouteBuilder once the city is known (the route's own city when editing). */
    fun ensureBuilder(cities: List<City>, current: City?) {
        val u = _ui.value
        if (u.builder != null || (editingId != null && u.editing == null)) return
        val owning = u.editing?.let { e -> cities.firstOrNull { it.id == e.cityId } }
        val city = owning ?: current ?: return
        _ui.update { it.copy(builder = RouteBuilder(city, u.editing)) }
        loadPlaces(city)
    }

    fun loadPlaces(city: City) {
        viewModelScope.launch {
            _ui.update { it.copy(places = UiState(LoadStatus.LOADING)) }
            val result = try {
                val items = api.places(city.slug)
                UiState(LoadStatus.READY, items, empty = items.isEmpty())
            } catch (e: Exception) {
                UiState<List<PlaceSummary>>(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load places.")
            }
            _ui.update { it.copy(places = result) }
        }
    }

    fun submit() {
        val b = _ui.value.builder ?: return
        viewModelScope.launch { b.submit(api)?.let { tour -> _ui.update { it.copy(created = tour) } } }
    }

    class Factory(private val api: Endpoints, private val editingId: String?) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = RouteBuilderViewModel(api, editingId) as T
    }
}
