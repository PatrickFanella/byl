package tv.subcult.museum.ui.moments

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
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
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.MomentDetail
import tv.subcult.museum.core.plainText
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.EmptyCard
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.MediaGallery
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SectionHeader
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

/** A moment in full: date, media, body, topics, and the sources it rests on. */
@Composable
fun MomentScreen(model: AppViewModel, id: String) {
    var state by remember(id) { mutableStateOf(UiState<MomentDetail>()) }
    val scope = rememberCoroutineScope()
    val uriHandler = LocalUriHandler.current
    suspend fun load() {
        state = UiState(LoadStatus.LOADING)
        state = try { UiState(LoadStatus.READY, model.api.moment(id)) } catch (e: Exception) { UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load the moment.") }
    }
    LaunchedEffect(id) { load() }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 16.dp), verticalArrangement = Arrangement.spacedBy(18.dp)) {
        LoadState(state, onRetry = { scope.launch { load() } }) { m ->
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                if (m.dateDisplay.isNotEmpty()) Overline(m.dateDisplay)
                Text(m.title, style = MaterialTheme.typography.headlineMedium, color = Museum.text)
                if (m.hasNarration || m.narration != null) Text("Audio guide available on the tour", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
            }
            if (m.media.isNotEmpty()) MediaGallery(m.media)
            Card {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    (m.body.ifEmpty { m.excerpt }).split(Regex("\\n\\s*\\n")).map { plainText(it) }.filter { it.isNotEmpty() }.forEach { para ->
                        Text(para, style = MaterialTheme.typography.bodyLarge, color = Museum.text.copy(alpha = 0.9f))
                    }
                }
            }
            if (m.topics.isNotEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) { m.topics.forEach { t -> AssistChip(onClick = {}, label = { Text(t.name) }) } }
            }
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                SectionHeader("Sources", eyebrow = "Where this comes from")
                if (m.sources.isEmpty()) EmptyCard("No sources listed", "This moment was written without a cited source.")
                m.sources.forEach { s ->
                    Card(onClick = if (s.url.isNotEmpty()) ({ uriHandler.openUri(s.url) }) else null) {
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Text(s.title.ifEmpty { s.url }, style = MaterialTheme.typography.titleMedium, color = if (s.url.isNotEmpty()) Museum.action else Museum.text)
                            Text(listOf(s.publisher, s.license, s.retrievedAt?.let { "retrieved $it" } ?: "").filter { it.isNotEmpty() }.joinToString(" · "), style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                            if (s.note.isNotEmpty()) Text(s.note, style = MaterialTheme.typography.bodySmall, color = Museum.faint)
                        }
                    }
                }
            }
        }
    }
}
