package tv.subcult.museum.ui.tours

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.TourSummary
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.TourCard
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

/** Every tour in the selected city, filtered by topic and travel mode. */
@Composable
fun TourListScreen(model: AppViewModel, onOpenTour: (slug: String) -> Unit) {
    val app by model.state.collectAsStateWithLifecycle()
    var topic by remember { mutableStateOf<String?>(null) }
    var mode by remember { mutableStateOf<String?>(null) }
    var tours by remember { mutableStateOf(UiState<List<TourSummary>>()) }
    val scope = rememberCoroutineScope()

    suspend fun load() {
        val city = app.city ?: return
        tours = UiState(LoadStatus.LOADING)
        tours = try {
            val items = model.api.tours(city.slug, topic, mode)
            UiState(LoadStatus.READY, items, empty = items.isEmpty())
        } catch (e: Exception) {
            UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load tours.")
        }
    }
    LaunchedEffect(app.city?.id, topic, mode) { load() }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Overline(app.city?.name ?: "Tours")
        Text("Tours", style = MaterialTheme.typography.headlineLarge, color = Museum.text)
        Text("Each tour is a gallery: one topic, a handful of places, and the moment at each that belongs to the story.", color = Museum.muted, style = MaterialTheme.typography.bodyLarge)
        app.city?.topics?.takeIf { it.isNotEmpty() }?.let { topics ->
            Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = topic == null, onClick = { topic = null }, label = { Text("All topics") })
                topics.forEach { t -> FilterChip(selected = topic == t.slug, onClick = { topic = if (topic == t.slug) null else t.slug }, label = { Text(t.name) }) }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(selected = mode == null, onClick = { mode = null }, label = { Text("Any") })
            FilterChip(selected = mode == "walk", onClick = { mode = if (mode == "walk") null else "walk" }, label = { Text("Walking") })
            FilterChip(selected = mode == "bike", onClick = { mode = if (mode == "bike") null else "bike" }, label = { Text("Biking") })
        }
        LoadState(tours, emptyTitle = "No tours match", emptyMessage = if (topic == null && mode == null) "Curated tours for this city are still being written." else "Try another topic or travel mode.", onRetry = { scope.launch { load() } }) { items ->
            items.forEach { t -> TourCard(t) { onOpenTour(t.slug) } }
        }
    }
}
