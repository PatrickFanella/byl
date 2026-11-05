@file:OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)

package tv.subcult.museum.ui.tours

import android.Manifest
import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.PlaceDetail
import tv.subcult.museum.core.TourStop
import tv.subcult.museum.core.distanceText
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.EmptyCard
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.MomentCard
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

/** Walking or riding mode: map, next stop with distance, arrival by GPS or by hand, directions, audio, progress. */
@Composable
fun WalkScreen(model: AppViewModel, onOpenMoment: (String) -> Unit, onEnded: () -> Unit) {
    val app by model.state.collectAsStateWithLifecycle()
    val active = app.active
    if (active == null) {
        LaunchedEffect(Unit) { onEnded() }
        return
    }
    val state by active.state.collectAsStateWithLifecycle()
    val fix by model.container.location.fixes.collectAsStateWithLifecycle()
    val playing by model.container.narration.isPlaying.collectAsStateWithLifecycle()
    val context = LocalContext.current
    var granted by remember { mutableStateOf(model.container.location.hasPermission()) }
    var showEnd by remember { mutableStateOf(false) }
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { r ->
        granted = r.values.any { it }
        if (granted) model.container.location.start()
    }

    LaunchedEffect(Unit) {
        if (!granted) permission.launch(arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION))
    }
    LocationWhileStarted(model, granted)
    LaunchedEffect(fix) { fix?.let { active.handleFix(it) } }
    // Arrival: with the guide on, play the featured moment once per announcement. The sheet is
    // driven by state.arrivedStop and cleared through acknowledgeArrival, so rotation does not replay it.
    LaunchedEffect(state.arrivedStop?.id) {
        val stop = state.arrivedStop ?: return@LaunchedEffect
        if (app.audioEnabled) model.playNarration(stop, requireGuide = true)
    }

    val stops = active.tour.stops.sortedBy { it.position }
    val next = state.nextStop

    Box(Modifier.fillMaxSize().background(Museum.background)) {
        TourMap(stops, state, fix, Modifier.fillMaxSize())

        Row(Modifier.fillMaxWidth().statusBarsPadding().padding(12.dp), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
            Pill(onClick = { showEnd = true }) { Text("End") }
            Pill { Text(active.tour.title, style = MaterialTheme.typography.titleMedium) }
            Pill(onClick = { model.setAudioGuide(!app.audioEnabled) }) { Text(if (app.audioEnabled) "🔊" else "🔇") }
        }

        Column(Modifier.align(Alignment.BottomCenter).navigationBarsPadding().padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (state.routeFromCache) Pill { Text("Offline: using the saved route", style = MaterialTheme.typography.bodySmall, color = Museum.muted) }
            state.routeMessage?.let { Pill { Text("$it. Stops still work; use Directions.", style = MaterialTheme.typography.bodySmall, color = Color(0xFFFDE68A)) } }

            Box(Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp)).background(Museum.background).padding(16.dp)) {
                if (next != null) {
                    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Column(Modifier.weight(1f)) {
                                Overline("Next · stop ${next.position} of ${stops.size}")
                                Text(next.place.name, style = MaterialTheme.typography.titleLarge, color = Museum.text)
                                next.featuredMoment?.let { Text(it.title, style = MaterialTheme.typography.bodyMedium, color = Museum.muted, maxLines = 1) }
                            }
                            Column(horizontalAlignment = Alignment.End) {
                                Text(state.distanceToNextM?.let { distanceText(it) } ?: "—", style = MaterialTheme.typography.titleLarge, color = Museum.text)
                                Text(active.progressText, style = MaterialTheme.typography.bodySmall, color = Museum.faint)
                            }
                        }
                        when {
                            !granted -> Text("Location is off. Tap I'm here at each stop, or allow location in Settings.", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                            fix == null -> Text("Waiting for your location…", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                        }
                        state.syncError?.let { Text("Progress saved on this device; the server will catch up later.", style = MaterialTheme.typography.bodySmall, color = Color(0xFFFDE68A)) }
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Button(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { active.markArrived() }, colors = ButtonDefaults.buttonColors(containerColor = Museum.action, contentColor = Museum.actionText)) { Text("I'm here") }
                            OutlinedButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = {
                                val nav = active.navigationUri()?.let { Intent(Intent.ACTION_VIEW, Uri.parse(it)) }
                                try {
                                    if (nav != null) context.startActivity(nav) else throw ActivityNotFoundException()
                                } catch (_: ActivityNotFoundException) {
                                    runCatching { active.geoUri()?.let { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(it))) } }
                                }
                            }) { Text("Directions") }
                            if (playing) OutlinedButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { model.container.narration.stop() }) { Text("■") }
                        }
                    }
                } else {
                    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        Overline("Tour complete", Museum.success)
                        Text("You walked every stop of ${active.tour.title}.", style = MaterialTheme.typography.titleMedium, color = Museum.text)
                        if (!app.isSignedIn) Text("Sign in to keep this on your account.", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                        Button(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { model.endTour(false); onEnded() }, colors = ButtonDefaults.buttonColors(containerColor = Museum.action, contentColor = Museum.actionText)) { Text("Done") }
                    }
                }
            }
        }
    }

    state.arrivedStop?.let { stop ->
        val dismiss = { active.acknowledgeArrival(); model.container.narration.stop() }
        ModalBottomSheet(onDismissRequest = dismiss, containerColor = Museum.background) {
            ArrivalSheet(model, stop, isLast = state.isComplete, playing = playing, onOpenMoment = { active.acknowledgeArrival(); onOpenMoment(it) }, onContinue = dismiss)
        }
    }

    if (showEnd) {
        AlertDialog(
            onDismissRequest = { showEnd = false },
            title = { Text("End this tour?") },
            text = { Text(active.progressText) },
            confirmButton = { TextButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { showEnd = false; model.endTour(false); onEnded() }) { Text("Keep my progress and end") } },
            dismissButton = {
                Row {
                    TextButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { showEnd = false; model.endTour(true); onEnded() }) { Text("Start over next time", color = Museum.danger) }
                    TextButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { showEnd = false }) { Text("Cancel") }
                }
            }
        )
    }
}

/** Starts location updates while the screen is started and stops them when it is not. */
@Composable
fun LocationWhileStarted(model: AppViewModel, granted: Boolean) {
    val lifecycle = androidx.lifecycle.compose.LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle, granted) {
        val observer = androidx.lifecycle.LifecycleEventObserver { _, event ->
            when (event) {
                androidx.lifecycle.Lifecycle.Event.ON_START -> if (granted) model.container.location.start()
                androidx.lifecycle.Lifecycle.Event.ON_STOP -> model.container.location.stop()
                else -> Unit
            }
        }
        if (granted && lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.STARTED)) model.container.location.start()
        lifecycle.addObserver(observer)
        onDispose {
            lifecycle.removeObserver(observer)
            model.container.location.stop()
        }
    }
}

@Composable
private fun Pill(onClick: (() -> Unit)? = null, content: @Composable () -> Unit) {
    val m = Modifier.clip(RoundedCornerShape(4.dp)).background(Museum.background).padding(horizontal = 12.dp, vertical = 6.dp)
    if (onClick != null) TextButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = onClick, contentPadding = androidx.compose.foundation.layout.PaddingValues(0.dp)) { Box(m) { content() } } else Box(m) { content() }
}

/** Shown on arrival: the featured moment with Listen, and the place's other moments. */
@Composable
private fun ArrivalSheet(model: AppViewModel, stop: TourStop, isLast: Boolean, playing: Boolean, onOpenMoment: (String) -> Unit, onContinue: () -> Unit) {
    var place by remember(stop.id) { mutableStateOf(UiState<PlaceDetail>(LoadStatus.IDLE)) }
    LaunchedEffect(stop.id) {
        if (stop.place.momentCount > 1) {
            place = UiState(LoadStatus.LOADING)
            place = try { UiState(LoadStatus.READY, model.api.place(stop.place.id)) } catch (e: Exception) { UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "") }
        }
    }
    Column(Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 18.dp).padding(bottom = 32.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Overline("You have arrived · stop ${stop.position}", Museum.success)
        Text(stop.place.name, style = MaterialTheme.typography.headlineMedium, color = Museum.text)
        if (stop.note.isNotEmpty()) Text(stop.note, color = Museum.muted, style = MaterialTheme.typography.bodyMedium)
        val featured = stop.featuredMoment
        if (featured != null) {
            MomentCard(featured, prominent = true) { onOpenMoment(featured.id) }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { onOpenMoment(featured.id) }) { Text("Read the full story") }
                OutlinedButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { if (playing) model.container.narration.stop() else model.playNarration(stop) }) { Text(if (playing) "■ Stop" else "▶ Listen") }
            }
        } else {
            EmptyCard("No moment featured here", "This stop is a waypoint; look around and carry on.")
        }
        val others = place.data?.moments?.filter { it.id != featured?.id } ?: emptyList()
        if (others.isNotEmpty()) {
            Overline("Also at this place")
            others.forEach { m -> MomentCard(m) { onOpenMoment(m.id) } }
        }
        Button(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = onContinue, modifier = Modifier.fillMaxWidth(), colors = ButtonDefaults.buttonColors(containerColor = Museum.action, contentColor = Museum.actionText)) {
            Text(if (isLast) "Finish" else "Continue to the next stop")
        }
    }
}
