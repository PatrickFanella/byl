package tv.subcult.museum.ui.saved

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.TourProgress
import tv.subcult.museum.core.TourSummary
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SignInPrompt
import tv.subcult.museum.ui.components.TourCard
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

private data class SavedData(val tours: List<TourSummary>, val progress: Map<String, TourProgress>)

/** Tours the visitor saved, with how far along each one they are. */
@Composable
fun SavedScreen(model: AppViewModel, onOpenTour: (String) -> Unit) {
    val app by model.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var state by remember { mutableStateOf(UiState<SavedData>()) }

    suspend fun load() {
        if (!app.isSignedIn) return
        state = UiState(LoadStatus.LOADING)
        state = try {
            val tours = model.api.saved()
            val best = HashMap<String, TourProgress>()
            for (rec in model.api.progress()) {
                if ((best[rec.tourId]?.visitedPositions?.size ?: -1) < rec.visitedPositions.size) best[rec.tourId] = rec
            }
            UiState(LoadStatus.READY, SavedData(tours, best), empty = tours.isEmpty())
        } catch (e: Exception) {
            UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load saved tours.")
        }
    }
    LaunchedEffect(app.isSignedIn, app.savedTourIds.size) { load() }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 16.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Overline("Saved")
        Text("Tours to come back to", style = MaterialTheme.typography.headlineLarge, color = Museum.text)
        if (!app.isSignedIn) {
            SignInPrompt("Sign in to save tours and keep your progress on every device.") { model.showSignIn(true) }
        } else {
            LoadState(state, emptyTitle = "Nothing saved yet", emptyMessage = "Tap Save for later on any tour.", onRetry = { scope.launch { load() } }) { data ->
                data.tours.forEach { tour ->
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        TourCard(tour) { onOpenTour(tour.slug) }
                        data.progress[tour.id]?.let { p ->
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                LinearProgressIndicator(progress = { p.visitedPositions.size.toFloat() / maxOf(tour.stopCount, 1) }, modifier = Modifier.weight(1f), color = if (p.completedAt == null) Museum.action else Museum.success)
                                Text(if (p.completedAt == null) "${p.visitedPositions.size} of ${tour.stopCount} stops" else "Completed", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                            }
                        }
                    }
                }
            }
        }
    }
}
