package tv.subcult.museum.ui.places

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiClient
import tv.subcult.museum.core.Endpoints
import tv.subcult.museum.core.Happening
import tv.subcult.museum.ui.theme.Museum
import java.time.Instant

/** Independent, initially hidden public notices. No trip, audio or progress callbacks. */
@Composable
fun HappeningsSection(placeId: String, origin: String, tripContext: Boolean = false) {
    val api = remember(origin) { Endpoints(ApiClient(origin) { null }) }
    var enabled by remember(placeId, origin) { mutableStateOf(false) }
    var busy by remember(placeId, origin) { mutableStateOf(false) }
    var items by remember(placeId, origin) { mutableStateOf(emptyList<Happening>()) }
    var cursor by remember(placeId, origin) { mutableStateOf("") }
    var message by remember(placeId, origin) { mutableStateOf("") }
    var generation by remember(placeId, origin) { mutableStateOf(0) }
    var request by remember(placeId, origin) { mutableStateOf<Job?>(null) }
    var now by remember(placeId, origin) { mutableStateOf(Instant.now()) }
    val scope = rememberCoroutineScope()
    val uri = LocalUriHandler.current
    fun load(more: Boolean = false) {
        val current = ++generation
        val next = if (more) cursor else ""
        request?.cancel(); busy = true; message = ""
        if (!more) { items = emptyList(); cursor = "" }
        request = scope.launch {
            try {
                val page = api.happenings(placeId, next)
                if (current != generation || !enabled) return@launch
                require(page.items.size <= 50 && page.items.all { it.placeId == placeId })
                require(page.nextCursor.isEmpty() || (page.nextCursor.all { it in '0'..'9' } && (page.nextCursor.toIntOrNull() ?: -1) in ((next.toIntOrNull() ?: 0) + 1)..10000))
                items = ((if (more) items else emptyList()) + page.items).distinctBy { it.id }
                cursor = page.nextCursor; now = Instant.now()
            } catch (e: CancellationException) { throw e }
            catch (_: Exception) {
                if (current == generation && enabled) { items = emptyList(); cursor = ""; message = "Current notices are unavailable. The historical story and tour still work." }
            } finally { if (current == generation) busy = false }
        }
    }
    LaunchedEffect(enabled, placeId, origin) { if (enabled) while (true) { delay(60000); now = Instant.now() } }
    DisposableEffect(placeId, origin) { onDispose { generation++; request?.cancel() } }
    Column(Modifier.border(1.dp, Museum.border, RoundedCornerShape(4.dp)).padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        TextButton(onClick = {
            enabled = !enabled
            if (enabled) load() else { generation++; request?.cancel(); busy = false; items = emptyList(); cursor = ""; message = "" }
        }) { Text(if (enabled) "Hide current notices" else "Show current notices") }
        if (enabled) {
            Text("Current announcements are separate from ${if (tripContext) "this started historical itinerary" else "the historical stories"}. Dates do not provide an exact start time. Confirm plans with the organizer.", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
            if (message.isNotEmpty()) Text(message, color = Museum.text)
            if (!busy && message.isEmpty() && items.isEmpty()) Text("No current notices have been published here.", color = Museum.text)
            items.forEach { notice ->
                HorizontalDivider(color = Museum.border)
                Text(notice.presentationStatus(now).label, style = MaterialTheme.typography.titleSmall, color = Museum.text)
                Text(notice.title, style = MaterialTheme.typography.titleMedium, color = Museum.text)
                Text(notice.occurrenceText, style = MaterialTheme.typography.bodySmall, color = Museum.text)
                Text(notice.summary, style = MaterialTheme.typography.bodyMedium, color = Museum.text)
                Text("Organizer: ${notice.organizer}", style = MaterialTheme.typography.bodySmall, color = Museum.text)
                Text("${notice.checkedText}. Source status: ${notice.sourceHealth.replace('_', ' ')}.", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                Text("Review by ${notice.reviewOn}; expires ${notice.expiresOn}.", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                if (notice.sources.isEmpty()) Text("Source attribution unavailable.", color = Museum.muted)
                notice.sources.forEach { source ->
                    val link = Happening.sourceURL(source.url)
                    if (link != null) TextButton(onClick = { uri.openUri(link) }) { Text(source.title.ifEmpty { source.publisher.ifEmpty { "Organizer announcement" } }) }
                    else Text("Source link unavailable", color = Museum.muted)
                    Text("${source.publisher} · captured ${source.retrievedOn} · media reuse ${source.license}", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                }
            }
            if (busy) Text("Loading current notices…", color = Museum.muted)
            TextButton(onClick = { load() }, enabled = !busy) { Text("Refresh current notices") }
            if (cursor.isNotEmpty()) TextButton(onClick = { load(true) }, enabled = !busy) { Text("Show more notices") }
        }
    }
}
