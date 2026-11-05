package tv.subcult.museum.ui.tours

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.TourDetail
import tv.subcult.museum.core.distanceText
import tv.subcult.museum.core.estimateText
import tv.subcult.museum.core.modeLabel
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.EmptyCard
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SectionHeader
import tv.subcult.museum.ui.components.SignInPrompt
import tv.subcult.museum.ui.components.StopRow
import tv.subcult.museum.ui.components.TourCard
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

/** One tour: summary, per-mode estimates, save, start, and the stops with their featured moments. */
@Composable
fun TourDetailScreen(model: AppViewModel, ref: String, onOpenPlace: (placeId: String, featuredMomentId: String?) -> Unit, onBack: () -> Unit) {
    val app by model.state.collectAsStateWithLifecycle()
    var state by remember(ref) { mutableStateOf(UiState<TourDetail>()) }
    var saved by remember { mutableStateOf(false) }
    var saveError by remember { mutableStateOf<String?>(null) }
    var wantSave by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    suspend fun load() {
        state = UiState(LoadStatus.LOADING)
        state = try {
            UiState(LoadStatus.READY, model.api.tour(ref))
        } catch (e: Exception) {
            UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load the tour.")
        }
    }
    LaunchedEffect(ref) { load() }
    LaunchedEffect(state.data?.id, app.savedTourIds) { state.data?.let { saved = it.id in app.savedTourIds } }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(18.dp)) {
        TextButton(onClick = onBack) { Text("← Tours", color = Museum.muted) }
        LoadState(state, loadingText = "Opening the gallery…", onRetry = { scope.launch { load() } }) { tour ->
            TourCard(tour.summaryOnly) {}
            if (tour.summary.isNotEmpty()) Text(tour.summary, color = Museum.muted, style = MaterialTheme.typography.bodyLarge)

            Card {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Overline("Getting around")
                    tour.modes.forEach { mode ->
                        val r = tour.routes[mode]
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Text(modeLabel(mode), style = MaterialTheme.typography.titleMedium, color = Museum.text)
                            val est = estimateText(r)
                            Text(
                                when {
                                    est != null -> "$est · ${distanceText(r?.distanceM ?: 0.0)}"
                                    r?.status == "failed" -> "Route unavailable"
                                    else -> "Route being computed"
                                },
                                color = if (r?.status == "failed") Museum.danger else Museum.muted,
                                style = MaterialTheme.typography.bodyMedium
                            )
                        }
                    }
                }
            }

            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                tour.modes.forEachIndexed { i, mode ->
                    val label = if (mode == "bike") "Start riding" else "Start walking"
                    if (i == 0) Button(onClick = { model.startTour(tour, mode) }, enabled = tour.stops.isNotEmpty(), modifier = Modifier.fillMaxWidth(), colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color.Black)) { Text(label) }
                    else OutlinedButton(onClick = { model.startTour(tour, mode) }, enabled = tour.stops.isNotEmpty(), modifier = Modifier.fillMaxWidth()) { Text(label) }
                }
                OutlinedButton(
                    onClick = {
                        if (!app.isSignedIn) { wantSave = true; return@OutlinedButton }
                        scope.launch {
                            saveError = null
                            try {
                                if (saved) model.api.unsave(tour.id) else model.api.save(tour.id)
                                saved = !saved
                                model.setSaved(tour.id, saved)
                            } catch (e: Exception) {
                                saveError = (e as? ApiException)?.message ?: "Could not save."
                            }
                        }
                    },
                    modifier = Modifier.fillMaxWidth()
                ) { Text(if (saved) "♥ Saved" else "♡ Save for later") }
                if (wantSave && !app.isSignedIn) SignInPrompt("Sign in to save tours and keep your progress on every device.") { model.showSignIn(true) }
                saveError?.let { Text(it, color = Museum.danger, style = MaterialTheme.typography.bodySmall) }
            }

            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                SectionHeader("Stops", eyebrow = "${tour.stops.size} places")
                if (tour.stops.isEmpty()) EmptyCard("No stops yet", "This tour has no published places.")
                tour.stops.sortedBy { it.position }.forEach { stop ->
                    StopRow(stop) { onOpenPlace(stop.place.id, stop.featuredMoment?.id ?: stop.place.primaryMomentId) }
                }
            }
        }
    }
}
