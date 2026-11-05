package tv.subcult.museum.ui.routes

import android.content.Intent
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
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.PlaceSummary
import tv.subcult.museum.core.Point
import tv.subcult.museum.core.RouteBuilder
import tv.subcult.museum.core.TourDetail
import tv.subcult.museum.core.distanceM
import tv.subcult.museum.core.shareUrl
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.EmptyCard
import tv.subcult.museum.ui.components.ErrorCard
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.LoadingRow
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SignInPrompt
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

/** Pick published places, order them, name the route, choose modes and visibility, then share. */
@Composable
fun RouteBuilderScreen(model: AppViewModel, editingId: String?, onDone: () -> Unit) {
    val app by model.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val fix by model.container.location.fixes.collectAsStateWithLifecycle()
    var editing by remember { mutableStateOf<TourDetail?>(null) }
    var editingError by remember { mutableStateOf<String?>(null) }
    var builder by remember { mutableStateOf<RouteBuilder?>(null) }
    var places by remember { mutableStateOf(UiState<List<PlaceSummary>>()) }
    var search by remember { mutableStateOf("") }
    var created by remember { mutableStateOf<TourDetail?>(null) }

    LaunchedEffect(editingId) {
        if (editingId != null && editing == null) {
            try { editing = model.api.tour(editingId) } catch (e: Exception) { editingError = (e as? ApiException)?.message ?: "Could not load the route." }
        }
    }
    LaunchedEffect(app.city?.id, editing?.id, app.cities.size) {
        if (builder != null || (editingId != null && editing == null)) return@LaunchedEffect
        val owning = editing?.let { e -> app.cities.firstOrNull { it.id == e.cityId } }
        val city = owning ?: app.city ?: return@LaunchedEffect
        val b = RouteBuilder(city, editing)
        builder = b
        places = UiState(LoadStatus.LOADING)
        places = try {
            val items = model.api.places(city.slug)
            UiState(LoadStatus.READY, items, empty = items.isEmpty())
        } catch (e: Exception) {
            UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load places.")
        }
    }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        TextButton(onClick = onDone) { Text("← Your routes", color = Museum.muted) }
        Text(if (editingId == null) "New route" else "Edit route", style = MaterialTheme.typography.headlineLarge, color = Museum.text)

        val done = created
        val b = builder
        when {
            !app.isSignedIn -> SignInPrompt("Sign in to build routes that follow you to every device.") { model.showSignIn(true) }
            editingError != null -> ErrorCard(editingError!!)
            done != null -> {
                Card {
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Overline(if (editingId == null) "Route created" else "Route updated", Museum.green)
                        Text(done.title, style = MaterialTheme.typography.headlineMedium, color = Museum.text)
                        Text(if (done.visibility == "private") "Only you can open it." else "Anyone with the link can open it; it is not listed publicly.", color = Museum.muted, style = MaterialTheme.typography.bodyMedium)
                    }
                }
                Button(onClick = {
                    val url = shareUrl(model.container.resolveBaseUrl(), done.slug)
                    val send = Intent(Intent.ACTION_SEND).apply { type = "text/plain"; putExtra(Intent.EXTRA_SUBJECT, done.title); putExtra(Intent.EXTRA_TEXT, url) }
                    context.startActivity(Intent.createChooser(send, "Share route"))
                }, modifier = Modifier.fillMaxWidth(), colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color.Black)) { Text("Share the link") }
                OutlinedButton(onClick = onDone, modifier = Modifier.fillMaxWidth()) { Text("Done") }
            }
            b == null -> LoadingRow()
            else -> {
                val s by b.state.collectAsStateWithLifecycle()
                OutlinedTextField(value = s.title, onValueChange = { b.title = it }, label = { Text("Name") }, placeholder = { Text("e.g. Depot to the river") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
                OutlinedTextField(value = s.summary, onValueChange = { b.summary = it }, label = { Text("Summary") }, placeholder = { Text("Optional: what ties these places together?") }, modifier = Modifier.fillMaxWidth(), minLines = 2)
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Overline("Travel")
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilterChip(selected = s.walk, onClick = { b.walk = !s.walk }, label = { Text("Walking") })
                        FilterChip(selected = s.bike, onClick = { b.bike = !s.bike }, label = { Text("Biking") })
                    }
                    if (s.modes.isEmpty()) Text("Pick at least one way to travel.", color = Museum.danger, style = MaterialTheme.typography.bodySmall)
                    Overline("Who can open it")
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilterChip(selected = s.visibility == "unlisted", onClick = { b.visibility = "unlisted" }, label = { Text("Anyone with the link") })
                        FilterChip(selected = s.visibility == "private", onClick = { b.visibility = "private" }, label = { Text("Only me") })
                    }
                }

                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                        Text("Stops", style = MaterialTheme.typography.titleLarge, color = Museum.text)
                        Text("${s.stops.size} of ${RouteBuilder.MAX_STOPS}", color = Museum.faint, style = MaterialTheme.typography.bodySmall)
                    }
                    if (s.stops.isEmpty()) EmptyCard("No stops yet", "Add places from the list; the first is where the route starts.")
                    s.stops.forEachIndexed { i, place ->
                        Card {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                                Text("${i + 1}", color = Museum.blue, style = MaterialTheme.typography.labelSmall)
                                Text(place.name, Modifier.weight(1f), style = MaterialTheme.typography.titleMedium, color = Museum.text, maxLines = 1)
                                TextButton(onClick = { b.move(i, i - 1) }, enabled = i > 0) { Text("↑") }
                                TextButton(onClick = { b.move(i, i + 1) }, enabled = i < s.stops.size - 1) { Text("↓") }
                                TextButton(onClick = { b.remove(place.id) }) { Text("✕", color = Museum.danger) }
                            }
                        }
                    }
                    // Review Focus 4: the API's 422 field message sits next to the stops list.
                    s.stopsError?.let { Text(it, color = Museum.danger, style = MaterialTheme.typography.bodyMedium) }
                        ?: s.stopsHint?.let { Text(it, color = Museum.muted, style = MaterialTheme.typography.bodySmall) }
                }

                if (s.error != null && s.stopsError == null) Text(s.error!!, color = Museum.danger, style = MaterialTheme.typography.bodyMedium)
                Button(
                    onClick = { scope.launch { b.submit(model.api)?.let { created = it } } },
                    enabled = s.canSubmit, modifier = Modifier.fillMaxWidth(),
                    colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color.Black)
                ) { Text(if (s.busy) "Saving…" else if (b.isEditing) "Save changes" else "Create route") }

                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("Places in ${b.city.name}", style = MaterialTheme.typography.titleLarge, color = Museum.text)
                    OutlinedTextField(value = search, onValueChange = { search = it }, label = { Text("Search by name or street") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
                    LoadState(places, emptyTitle = "No places match", onRetry = null) { all ->
                        val needle = search.trim().lowercase()
                        var list = all.filter { needle.isEmpty() || it.name.lowercase().contains(needle) || it.address.lowercase().contains(needle) }
                        fix?.let { f -> list = list.sortedBy { distanceM(Point(it.latitude, it.longitude), f.point) } }
                        if (list.isEmpty()) EmptyCard("No places match")
                        list.take(40).forEach { place ->
                            val added = b.contains(place)
                            Card {
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                                    Column(Modifier.weight(1f)) {
                                        Text(place.name, style = MaterialTheme.typography.titleMedium, color = Museum.text, maxLines = 1)
                                        if (place.address.isNotEmpty()) Text(place.address, style = MaterialTheme.typography.bodySmall, color = Museum.muted, maxLines = 1)
                                    }
                                    OutlinedButton(onClick = { if (added) b.remove(place.id) else b.add(place) }, enabled = added || s.stops.size < RouteBuilder.MAX_STOPS) { Text(if (added) "Added" else "Add") }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
