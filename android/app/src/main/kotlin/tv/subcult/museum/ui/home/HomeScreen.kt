package tv.subcult.museum.ui.home

import android.Manifest
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
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
import tv.subcult.museum.core.PlaceSummary
import tv.subcult.museum.core.TourSummary
import tv.subcult.museum.core.distanceText
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.EmptyCard
import tv.subcult.museum.ui.components.ErrorCard
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.LoadingRow
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SectionHeader
import tv.subcult.museum.ui.components.TourCard
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

/** City picker, tours, and places near the visitor. */
@Composable
fun HomeScreen(model: AppViewModel, onOpenTour: (slug: String) -> Unit, onOpenPlace: (id: String) -> Unit) {
    val app by model.state.collectAsStateWithLifecycle()
    val fix by model.container.location.fixes.collectAsStateWithLifecycle(null)
    var tours by remember { mutableStateOf(UiState<List<TourSummary>>()) }
    var nearby by remember { mutableStateOf(UiState<List<PlaceSummary>>(LoadStatus.IDLE)) }
    var granted by remember { mutableStateOf(model.container.location.hasPermission()) }
    val scope = rememberCoroutineScope()
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { result ->
        granted = result.values.any { it }
        if (granted) model.container.location.start()
    }

    suspend fun loadTours() {
        val city = app.city ?: return
        tours = UiState(LoadStatus.LOADING)
        tours = try {
            val items = model.api.tours(city.slug)
            UiState(LoadStatus.READY, items, empty = items.isEmpty())
        } catch (e: Exception) {
            UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load tours.")
        }
    }
    LaunchedEffect(app.city?.id) { loadTours() }
    LaunchedEffect(Unit) {
        if (!granted) permission.launch(arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION))
        else model.container.location.start()
    }
    LaunchedEffect(app.city?.id, fix?.let { "${(it.lat * 1000).toInt()},${(it.lng * 1000).toInt()}" }) {
        val city = app.city ?: return@LaunchedEffect
        val f = fix ?: return@LaunchedEffect
        nearby = UiState(LoadStatus.LOADING)
        nearby = try {
            val items = model.api.places(city.slug, near = f.lat to f.lng)
            UiState(LoadStatus.READY, items, empty = items.isEmpty())
        } catch (e: Exception) {
            UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load places.")
        }
    }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(22.dp)) {
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Overline("Invisible Museum")
            Text(app.city?.let { "${it.name} as a museum" } ?: "Your city as a museum", style = MaterialTheme.typography.headlineLarge, color = Museum.text)
            Text("Walk or ride through the stories that happened right where you are standing.", color = Museum.muted, style = MaterialTheme.typography.bodyLarge)
            if (app.cities.size > 1) {
                var open by remember { mutableStateOf(false) }
                OutlinedButton(onClick = { open = true }) { Text(app.city?.name ?: "Choose a city") }
                DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
                    app.cities.forEach { c -> DropdownMenuItem(text = { Text(c.name) }, onClick = { model.selectCity(c); open = false }) }
                }
            }
        }

        if (app.citiesStatus == LoadStatus.ERROR && app.cities.isEmpty()) {
            ErrorCard(app.citiesMessage) { model.loadCities() }
        } else {
            Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                SectionHeader("Tours", eyebrow = "By topic")
                if (app.city == null) LoadingRow("Finding your city…")
                else LoadState(tours, emptyTitle = "No tours yet", emptyMessage = "Curated tours for this city are still being written.", onRetry = { scope.launch { loadTours() } }) { items ->
                    items.forEach { t -> TourCard(t) { onOpenTour(t.slug) } }
                }
            }
            Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                SectionHeader("Near you", eyebrow = "Places")
                when {
                    !granted -> EmptyCard("Location is off", "Allow location access to see the places around you.")
                    fix == null -> LoadingRow("Waiting for your location…")
                    else -> LoadState(nearby, emptyTitle = "Nothing within 2 km", emptyMessage = "Pick a tour to travel to its first stop.") { items ->
                        items.take(8).forEach { p ->
                            Card(onClick = { onOpenPlace(p.id) }) {
                                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                                    Column {
                                        Text(p.name, style = MaterialTheme.typography.titleMedium, color = Museum.text)
                                        val meta = listOfNotNull(p.distanceM?.let { distanceText(it) }, if (p.momentCount == 1) "1 moment" else "${p.momentCount} moments").joinToString(" · ")
                                        Text(meta, style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                                    }
                                    Text("›", color = Museum.faint)
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
