package tv.subcult.museum.core

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update

data class RouteBuilderState(
    val title: String = "",
    val summary: String = "",
    val walk: Boolean = true,
    val bike: Boolean = true,
    val visibility: String = "unlisted",
    val stops: List<PlaceSummary> = emptyList(),
    val busy: Boolean = false,
    val error: String? = null,
    val fieldErrors: Map<String, String> = emptyMap(),
    val foreignStops: List<PlaceSummary> = emptyList(),
    val stopsHint: String? = null,
    val canSubmit: Boolean = false
) {
    val stopsError: String? get() = fieldErrors["stops"]
    val modes: List<String> get() = buildList { if (walk) add("walk"); if (bike) add("bike") }
}

/** The link a visitor shares: `{base}/t/{slug}`. */
fun shareUrl(base: String, slug: String): String = base.trimEnd('/') + "/t/" + slug

/**
 * Builds or edits a visitor route: ordered published places in one city, a name, modes and
 * visibility. Validation mirrors the API (2–30 stops, one city) so Create stays disabled until
 * the request can succeed; a 422 still maps its field messages next to the stops.
 */
class RouteBuilder(val city: City, val editing: TourDetail? = null) {
    private val _state = MutableStateFlow(RouteBuilderState())
    val state: StateFlow<RouteBuilderState> = _state

    init {
        if (editing != null) {
            _state.value = RouteBuilderState(
                title = editing.title,
                summary = editing.summary,
                walk = "walk" in editing.modes,
                bike = "bike" in editing.modes,
                visibility = if (editing.visibility == "private") "private" else "unlisted",
                stops = editing.stops.sortedBy { it.position }.map { it.place }
            )
        }
        revalidate()
    }

    val isEditing: Boolean get() = editing != null

    var title: String
        get() = _state.value.title
        set(value) = change { it.copy(title = value) }

    var summary: String
        get() = _state.value.summary
        set(value) = change { it.copy(summary = value) }

    var walk: Boolean
        get() = _state.value.walk
        set(value) = change { it.copy(walk = value) }

    var bike: Boolean
        get() = _state.value.bike
        set(value) = change { it.copy(bike = value) }

    var visibility: String
        get() = _state.value.visibility
        set(value) = change { it.copy(visibility = if (value == "private") "private" else "unlisted") }

    fun contains(place: PlaceSummary): Boolean = _state.value.stops.any { it.id == place.id }

    fun add(place: PlaceSummary) {
        if (contains(place) || _state.value.stops.size >= MAX_STOPS) return
        change { it.copy(stops = it.stops + place, fieldErrors = it.fieldErrors - "stops") }
    }

    fun remove(placeId: String) = change { it.copy(stops = it.stops.filter { s -> s.id != placeId }) }

    fun move(from: Int, to: Int) {
        val stops = _state.value.stops
        if (from == to || from !in stops.indices || to !in stops.indices) return
        val next = stops.toMutableList()
        val item = next.removeAt(from)
        next.add(to, item)
        change { it.copy(stops = next) }
    }

    /** Creates (POST) or updates (PATCH) the route; null on failure with error and fieldErrors set. */
    suspend fun submit(api: Endpoints): TourDetail? {
        if (!_state.value.canSubmit) return null
        change { it.copy(busy = true, error = null, fieldErrors = emptyMap()) }
        val s = _state.value
        val stops = s.stops.map { StopInput(it.id) }
        return try {
            val result = if (editing != null) {
                api.updateTour(editing.id, UpdateUserTourRequest(s.title.trim(), s.summary, s.modes, s.visibility, stops))
            } else {
                api.createTour(CreateUserTourRequest(city.slug, s.title.trim(), s.summary, s.modes, s.visibility, stops))
            }
            change { it.copy(busy = false) }
            result
        } catch (e: ApiException) {
            change { it.copy(busy = false, error = e.message, fieldErrors = e.fields) }
            null
        } catch (e: Exception) {
            change { it.copy(busy = false, error = e.message ?: "Could not save the route.") }
            null
        }
    }

    private fun change(f: (RouteBuilderState) -> RouteBuilderState) {
        _state.update(f)
        revalidate()
    }

    private fun revalidate() {
        _state.update { s ->
            val foreign = s.stops.filter { it.cityId != city.id }
            val hint = when {
                foreign.isNotEmpty() -> "Every stop must be in ${city.name}"
                s.stops.size < MIN_STOPS -> "Add at least $MIN_STOPS stops (${s.stops.size} so far)"
                s.stops.size >= MAX_STOPS -> "$MAX_STOPS stops is the maximum"
                else -> null
            }
            val can = !s.busy && s.title.isNotBlank() && s.modes.isNotEmpty() &&
                s.stops.size in MIN_STOPS..MAX_STOPS && foreign.isEmpty()
            s.copy(foreignStops = foreign, stopsHint = hint, canSubmit = can)
        }
    }

    companion object {
        const val MIN_STOPS = 2
        const val MAX_STOPS = 30
    }
}
