package tv.subcult.museum.ui.places

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
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
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.PlaceDetail
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.EmptyCard
import tv.subcult.museum.ui.components.LoadState
import tv.subcult.museum.ui.components.MomentCard
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SectionHeader
import tv.subcult.museum.ui.components.UiState
import tv.subcult.museum.ui.theme.Museum

/** A place and every moment that happened there; the tour's featured moment comes first. */
@Composable
fun PlaceScreen(model: AppViewModel, ref: String, featuredId: String?, onOpenMoment: (id: String) -> Unit) {
    var state by remember(ref) { mutableStateOf(UiState<PlaceDetail>()) }
    val scope = rememberCoroutineScope()
    suspend fun load() {
        state = UiState(LoadStatus.LOADING)
        state = try { UiState(LoadStatus.READY, model.api.place(ref)) } catch (e: Exception) { UiState(LoadStatus.ERROR, message = (e as? ApiException)?.message ?: "Could not load the place.") }
    }
    LaunchedEffect(ref) { load() }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 16.dp), verticalArrangement = Arrangement.spacedBy(18.dp)) {
        LoadState(state, onRetry = { scope.launch { load() } }) { p ->
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Overline(p.kind.replace('_', ' '))
                Text(p.name, style = MaterialTheme.typography.headlineMedium, color = Museum.text)
                if (p.address.isNotEmpty()) Text(p.address, color = Museum.muted, style = MaterialTheme.typography.bodyMedium)
                if (p.summary.isNotEmpty()) Text(p.summary, color = Museum.text.copy(alpha = 0.85f), style = MaterialTheme.typography.bodyLarge)
            }
            val featured = p.moments.firstOrNull { it.id == featuredId } ?: p.primaryMoment
            val others = p.moments.filter { it.id != featured?.id }
            if (featured != null) {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    SectionHeader(featured.title, eyebrow = if (featuredId != null) "Featured on this tour" else "Primary moment")
                    MomentCard(featured, prominent = true) { onOpenMoment(featured.id) }
                }
            }
            if (others.isNotEmpty()) {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    SectionHeader(if (featured == null) "Moments here" else "Also at this place", eyebrow = "${others.size} more")
                    others.forEach { m -> MomentCard(m) { onOpenMoment(m.id) } }
                }
            }
            if (featured == null && others.isEmpty()) EmptyCard("Nothing published here yet", "Curators are still writing this place's moments.")
        }
    }
}
