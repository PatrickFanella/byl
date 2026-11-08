package tv.subcult.museum.ui.profile

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import tv.subcult.museum.core.TripContributions
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.theme.Museum

@Composable
fun ContributionControls(controller: TripContributions, allowOptIn: Boolean) {
    val state by controller.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var confirmErase by remember { mutableStateOf(false) }
    Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Overline("Route improvement")
        Text(state.message, color = Museum.text)
        Text("Optional contributions use location only while this trip is visible and you have chosen to contribute. Raw fixes stay in memory. Qualified summaries contain a route segment, direction, UTC day and counts. They are linked to your account, not anonymous. Navigation works with contributions off.", color = Museum.muted)
        Text("Unsent summaries expire after seven UTC days. Server contributions and consent expire after 30 days. Restart pauses collection until you choose Resume. Contributions do not report closures or prove that a route is safe or accessible.", color = Museum.muted)
        if (allowOptIn && controller.eligible && controller.identityAvailable) {
            if (state.optedIn) OutlinedButton(onClick = { scope.launch { controller.setPaused(!state.paused) } }, enabled = !state.busy) { Text(if (state.paused) "Resume contributions" else "Pause contributions") }
            else Button(onClick = { scope.launch { controller.optIn() } }, enabled = state.admitted && !state.busy) { Text("Contribute for this revision and mode") }
        }
        if (!allowOptIn) Text("Saved summaries wait until you explicitly resume the matching trip. Deletion retries are available here.", color = Museum.muted)
        Text("${state.pending} saved summaries · ${state.deletionPending} pending deletions", color = Museum.muted)
        OutlinedButton(onClick = { scope.launch { controller.refresh() } }, enabled = !state.busy) { Text("Retry checks and pending work") }
        OutlinedButton(onClick = { confirmErase = true }, enabled = state.retainedCount > 0) { Text("Stop and delete retained contributions") }
        Text("Deletion covers retained choices on this device, including other servers. A lost consent receipt needs the original account and server to recover it. Keep pending deletion details until confirmation. Other devices and lost deletion handles need separate attention.", color = Museum.muted)
    }
    if (confirmErase) AlertDialog(onDismissRequest = { confirmErase = false }, title = { Text("Stop and delete retained contributions?") },
        text = { Text("Collection stops and unsent summaries are erased immediately. Saved deletion details remain until each server confirms removal or the original consent expires.") },
        confirmButton = { TextButton(onClick = { confirmErase = false; scope.launch { controller.eraseAll() } }) { Text("Stop and delete") } },
        dismissButton = { TextButton(onClick = { confirmErase = false }) { Text("Cancel") } })
}
