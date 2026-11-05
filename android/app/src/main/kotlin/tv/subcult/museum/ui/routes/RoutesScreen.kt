package tv.subcult.museum.ui.routes

import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.TourSummary
import tv.subcult.museum.core.shareUrl
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SignInPrompt
import tv.subcult.museum.ui.components.TourCard
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

/** The visitor's own routes: build, share, edit, delete. Signed-in only. */
@Composable
fun RoutesScreen(model: AppViewModel, onNew: () -> Unit, onEdit: (String) -> Unit, onOpenTour: (String) -> Unit) {
    val app by model.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var routes by remember { mutableStateOf(UiState<List<TourSummary>>()) }
    var pendingDelete by remember { mutableStateOf<TourSummary?>(null) }
    var deleteError by remember { mutableStateOf<String?>(null) }

    suspend fun load() {
        if (!app.isSignedIn) return
        routes = UiState(LoadStatus.LOADING)
        routes = try {
            val items = model.api.myTours()
            UiState(LoadStatus.READY, items, empty = items.isEmpty())
        } catch (e: Exception) {
            UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load your routes.")
        }
    }
    LaunchedEffect(app.isSignedIn) { load() }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Overline("Your routes")
        Text("Curate your own gallery", style = MaterialTheme.typography.headlineLarge, color = Museum.text)
        Text("Order the places you love into a walk or ride and share it by link.", color = Museum.muted, style = MaterialTheme.typography.bodyLarge)
        if (!app.isSignedIn) {
            SignInPrompt("Sign in to build routes that follow you to every device.") { model.showSignIn(true) }
        } else {
            Button(onClick = onNew, colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color.Black), enabled = app.city != null) { Text("+ New route") }
            LoadState(routes, emptyTitle = "No routes yet", emptyMessage = "Tap New route to pick your first two places.", onRetry = { scope.launch { load() } }) { items ->
                items.forEach { tour ->
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        TourCard(tour) { onOpenTour(tour.slug) }
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            OutlinedButton(onClick = {
                                val url = shareUrl(model.container.resolveBaseUrl(), tour.slug)
                                val send = Intent(Intent.ACTION_SEND).apply { type = "text/plain"; putExtra(Intent.EXTRA_SUBJECT, tour.title); putExtra(Intent.EXTRA_TEXT, url) }
                                context.startActivity(Intent.createChooser(send, "Share route"))
                            }) { Text("Share") }
                            OutlinedButton(onClick = { onEdit(tour.id) }) { Text("Edit") }
                            TextButton(onClick = { pendingDelete = tour }) { Text("Delete", color = Museum.danger) }
                        }
                    }
                }
            }
        }
    }

    pendingDelete?.let { tour ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text("Delete this route?") },
            text = { Text(deleteError ?: tour.title) },
            confirmButton = {
                TextButton(onClick = {
                    scope.launch {
                        try {
                            model.api.deleteTour(tour.id)
                            pendingDelete = null
                            load()
                        } catch (e: Exception) {
                            deleteError = (e as? ApiException)?.message ?: "Could not delete the route."
                        }
                    }
                }) { Text("Delete", color = Museum.danger) }
            },
            dismissButton = { TextButton(onClick = { pendingDelete = null }) { Text("Cancel") } }
        )
    }
}
