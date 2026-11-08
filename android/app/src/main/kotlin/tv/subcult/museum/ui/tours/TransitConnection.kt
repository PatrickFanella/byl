package tv.subcult.museum.ui.tours

import android.app.DatePickerDialog
import android.app.TimePickerDialog
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import tv.subcult.museum.core.*
import tv.subcult.museum.ui.theme.Museum
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

@Composable
fun TransitConnection(trip: TransitTrip, onDone: () -> Unit) {
    val state by trip.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val lifecycle = androidx.lifecycle.compose.LocalLifecycleOwner.current.lifecycle
    var now by remember(trip) { mutableStateOf(Instant.now()) }
    var departure by remember(trip) { mutableStateOf(Instant.now().plusSeconds(3600)) }
    var handoff by remember(trip) { mutableStateOf<Boolean?>(null) }
    var error by remember(trip) { mutableStateOf<String?>(null) }
    LaunchedEffect(trip) {
        while (true) {
            if (lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.RESUMED)) trip.refreshGate()
            delay(30_000)
        }
    }
    LaunchedEffect(trip) { while (true) { now = Instant.now(); delay(1000) } }
    DisposableEffect(trip, lifecycle) {
        val observer = androidx.lifecycle.LifecycleEventObserver { _, event ->
            if (event == androidx.lifecycle.Lifecycle.Event.ON_RESUME) scope.launch { trip.refreshGate() }
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }
    val tour = trip.revision.snapshot?.tour
    val choice = tour?.approvedTransit
    Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).navigationBarsPadding().padding(18.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("Scheduled CTA connection", style = MaterialTheme.typography.headlineSmall, color = Museum.text)
        Text("Revision ${trip.revision.number} · timetable estimates, not live arrivals. Byl does not detect boarding, alighting or train movement.", color = Museum.muted)
        if (choice != null) {
            Text("Board ${choice.board.name} · platform ${choice.board.id}", color = Museum.text)
            Text("Alight ${choice.alight.name} · platform ${choice.alight.id}", color = Museum.text)
            Text("Walking estimates: ${choice.intent.walkToStopSeconds / 60} min to the platform, ${choice.intent.walkFromStopSeconds / 60} min after alighting, plus ${choice.intent.boardingBufferSeconds / 60} min boarding buffer.", color = Museum.muted)
            Text(choice.intent.walkingAssumptions, color = Museum.muted)
        }
        TextButton(onClick = { runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("https://www.transitchicago.com/blueline/"))) }.onFailure { error = "No app could open CTA service information." } }) { Text("Check CTA Blue Line alerts") }
        state.selected?.let { selected ->
            Text("Retained choice", color = Museum.text, style = MaterialTheme.typography.titleLarge)
            TransitPlan(selected)
            selected.unavailableReason(now)?.let { Text(it, color = Museum.danger) }
            if (state.guidanceBlocked) Text("This connection needs review. Retained details remain readable; directions are suspended until you explicitly accept a fresh valid plan.", color = Museum.danger)
            if (trip.canGuide(now)) {
                OutlinedButton(onClick = { handoff = true }) { Text("Walking directions to boarding platform") }
                OutlinedButton(onClick = { handoff = false }) { Text("Walking directions after alighting") }
            }
        }
        if (state.selected != null || state.requiresRemoval) TextButton(onClick = { runCatching { trip.remove() }.onFailure { error = "Could not remove the saved choice. Check device storage." } }) { Text("Remove retained transit choice") }
        Text("Choose a departure", color = Museum.text, style = MaterialTheme.typography.titleLarge)
        val fromName = tour?.stops?.firstOrNull { it.id == choice?.intent?.fromStopId }?.place?.name ?: "the first connector story stop"
        Text("Choose when you expect to leave $fromName after its stories. Earlier walking and story time are outside this query. The picker uses your device timezone; results use Chicago time.", color = Museum.muted)
        OutlinedButton(onClick = {
            val selected = departure.atZone(ZoneId.systemDefault())
            DatePickerDialog(context, { _, year, month, day ->
                TimePickerDialog(context, { _, hour, minute ->
                    departure = LocalDateTime.of(year, month + 1, day, hour, minute).atZone(ZoneId.systemDefault()).toInstant()
                }, selected.hour, selected.minute, true).show()
            }, selected.year, selected.monthValue - 1, selected.dayOfMonth).show()
        }) { Text("Leave story stop: ${DateTimeFormatter.ofPattern("MMM d, yyyy HH:mm z").withZone(ZoneId.systemDefault()).format(departure)}") }
        Button(onClick = { error = null; scope.launch { trip.request(departure) } }, enabled = !state.busy) { Text(if (state.busy) "Requesting…" else "Request a plan for review") }
        state.proposed?.let { proposal ->
            Text("Proposed choice · review before retaining", color = Museum.text, style = MaterialTheme.typography.titleLarge)
            TransitPlan(proposal)
            val reason = proposal.unavailableReason(now)
            if (reason != null) Text(reason, color = Museum.danger)
            else Button(onClick = { runCatching { trip.accept() }.onFailure { error = it.message ?: "Could not retain this plan." } }, enabled = !state.busy && state.enabled == true) { Text("Accept and retain this plan") }
        }
        if (state.enabled != true) Text("Scheduled transit availability is unconfirmed or disabled. Directions are suspended.", color = Museum.danger)
        OutlinedButton(onClick = { scope.launch { trip.refreshGate() } }) { Text("Refresh availability") }
        state.message?.let { Text(it, color = Museum.danger) }
        error?.let { Text(it, color = Museum.danger) }
        TextButton(onClick = onDone) { Text("Done") }
    }
    if (handoff != null) AlertDialog(
        onDismissRequest = { handoff = null }, title = { Text("Open walking directions in a maps app?") },
        text = { Text("The maps app takes over directions between public story/platform references. Timetable points are not reviewed street entrances or proof of accessibility. Check station signs and current service. No device location is included in this link.") },
        confirmButton = { TextButton(onClick = {
            handoff?.let { boarding -> trip.walkingUrl(boarding, Instant.now())?.let { url ->
                runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url))) }.onFailure { error = "No app could open walking directions." }
            } }; handoff = null
        }) { Text("Open maps") } }, dismissButton = { TextButton(onClick = { handoff = null }) { Text("Cancel") } }
    )
}

@Composable
private fun TransitPlan(plan: ScheduledTransitPlan) {
    Text("Result: ${plan.state.replace('_', ' ')}", color = Museum.text)
    Text("Leave story stop: ${chicagoTime(plan.requestedDepartureAt)}", color = Museum.muted)
    plan.service.source?.let { Text("CTA timetable retrieved ${chicagoTime(it.retrievedAt)} · America/Chicago", color = Museum.muted) }
    plan.service.headsign?.let { Text("Blue Line toward $it", color = Museum.text) }
    plan.segments.forEach { segment ->
        Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(segment.label, color = Museum.text, style = MaterialTheme.typography.titleMedium)
            Text("${segment.durationS / 60} min · ${segment.basis.replace('_', ' ')}", color = Museum.muted)
            val departure = segment.departureAt; val arrival = segment.arrivalAt
            if (departure != null && arrival != null) Text("${chicagoTime(departure)} → ${chicagoTime(arrival)}", color = Museum.muted)
        }
    }
    plan.assumptions.forEach { Text(it, color = Museum.muted) }
}
private fun chicagoTime(value: String): String = transitInstant(value)?.let {
    DateTimeFormatter.ofPattern("MMM d, yyyy HH:mm z").withZone(ZoneId.of("America/Chicago")).format(it)
} ?: "Unavailable"
