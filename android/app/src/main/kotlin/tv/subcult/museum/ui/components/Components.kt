package tv.subcult.museum.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import tv.subcult.museum.core.LoadStatus
import tv.subcult.museum.core.MediaAsset
import tv.subcult.museum.core.MomentSummary
import tv.subcult.museum.core.TourStop
import tv.subcult.museum.core.TourSummary
import tv.subcult.museum.core.estimateText
import tv.subcult.museum.ui.theme.Museum

/** A screen's async state: one value, four states, a retry. */
data class UiState<T>(
    val status: LoadStatus = LoadStatus.LOADING,
    val data: T? = null,
    val message: String = "",
    val empty: Boolean = false
)

@Composable
fun Overline(text: String, color: Color = Museum.action) {
    Text(text, style = MaterialTheme.typography.labelSmall, color = color)
}

@Composable
fun SectionHeader(title: String, eyebrow: String? = null) {
    Column {
        if (eyebrow != null) Overline(eyebrow)
        Text(title, style = MaterialTheme.typography.titleLarge, color = Museum.text)
    }
}

@Composable
fun Card(modifier: Modifier = Modifier, onClick: (() -> Unit)? = null, content: @Composable () -> Unit) {
    val shape = RoundedCornerShape(8.dp)
    Box(
        modifier
            .fillMaxWidth()
            .clip(shape)
            .background(Museum.surface)
            .border(1.dp, Museum.border, shape)
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .padding(16.dp)
    ) { content() }
}

@Composable
fun LoadingRow(text: String = "Loading…") {
    Row(Modifier.padding(vertical = 16.dp), verticalAlignment = Alignment.CenterVertically) {
        CircularProgressIndicator(Modifier.size(18.dp), color = Museum.text, strokeWidth = 2.dp)
        Spacer(Modifier.width(12.dp))
        Text(text, color = Museum.muted, style = MaterialTheme.typography.bodyMedium)
    }
}

@Composable
fun EmptyCard(title: String, message: String? = null) {
    Card {
        Column {
            Text(title, style = MaterialTheme.typography.titleMedium, color = Museum.text)
            if (message != null) Text(message, color = Museum.muted, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

@Composable
fun ErrorCard(message: String, onRetry: (() -> Unit)? = null) {
    Card {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Overline("Something went wrong", Museum.danger)
            Text(message, color = Museum.text, style = MaterialTheme.typography.bodyMedium)
            if (onRetry != null) OutlinedButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = onRetry) { Text("Try again") }
        }
    }
}

/** Renders loading / error / empty states, or the content when ready. */
@Composable
fun <T> LoadState(
    state: UiState<T>,
    emptyTitle: String = "Nothing here yet",
    emptyMessage: String? = null,
    loadingText: String = "Loading…",
    onRetry: (() -> Unit)? = null,
    content: @Composable (T) -> Unit
) {
    when {
        state.status == LoadStatus.LOADING || state.status == LoadStatus.IDLE -> LoadingRow(loadingText)
        state.status == LoadStatus.ERROR -> ErrorCard(state.message, onRetry)
        state.empty || state.data == null -> EmptyCard(emptyTitle, emptyMessage)
        else -> content(state.data)
    }
}

@Composable
fun SignInPrompt(reason: String, onSignIn: () -> Unit) {
    Card {
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Overline("Sign in")
            Text(reason, color = Museum.text, style = MaterialTheme.typography.bodyMedium)
            Button(shape = RoundedCornerShape(4.dp), onClick = onSignIn, colors = ButtonDefaults.buttonColors(containerColor = Museum.action, contentColor = Museum.actionText)) { Text("Sign in") }
        }
    }
}

@Composable
fun HeroCard(imageUrl: String?, height: Int = 180, content: @Composable () -> Unit) {
    Column(Modifier.fillMaxWidth().background(Museum.background).border(1.dp, Museum.border)) {
        if (imageUrl != null) {
            AsyncImage(model = imageUrl, contentDescription = null, contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxWidth().height(height.dp))
        }
        Box(Modifier.fillMaxWidth().padding(16.dp)) { content() }
    }
}

@Composable
fun TourCard(tour: TourSummary, onClick: () -> Unit) {
    Box(Modifier.clickable(onClick = onClick)) {
        HeroCard(tour.heroMedia?.publicUrl) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                tour.topic?.let { Overline(it.name) }
                Text(tour.title, style = MaterialTheme.typography.titleLarge, color = Museum.text, maxLines = Int.MAX_VALUE, overflow = TextOverflow.Clip)
                val parts = buildList {
                    add("${tour.stopCount} stops")
                    tour.modes.forEach { m -> estimateText(tour.routes[m])?.let { add(it) } }
                    if (tour.isUserRoute) add("visitor route")
                }
                Text(parts.joinToString(" · "), style = MaterialTheme.typography.bodySmall, color = Museum.muted)
            }
        }
    }
}

@Composable
fun StopRow(stop: TourStop, visited: Boolean = false, onClick: () -> Unit) {
    Card(onClick = onClick) {
        Row(horizontalArrangement = Arrangement.spacedBy(14.dp), verticalAlignment = Alignment.Top) {
            Box(
                Modifier.size(30.dp).clip(CircleShape).background(if (visited) Museum.successSurface else Museum.action),
                contentAlignment = Alignment.Center
            ) {
                if (visited) Icon(Icons.Default.Check, contentDescription = "Visited", tint = Museum.success, modifier = Modifier.size(16.dp))
                else Text("${stop.position}", color = Museum.actionText, style = MaterialTheme.typography.labelSmall)
            }
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(stop.place.name, style = MaterialTheme.typography.titleMedium, color = Museum.text)
                val m = stop.featuredMoment
                if (m != null) {
                    Text(m.title, style = MaterialTheme.typography.bodyMedium, color = Museum.text)
                    if (m.dateDisplay.isNotEmpty()) Text(m.dateDisplay, style = MaterialTheme.typography.labelSmall, color = Museum.action)
                } else {
                    Text("No moment published here yet", style = MaterialTheme.typography.bodySmall, color = Museum.faint)
                }
                if (stop.place.momentCount > 1) Text("+${stop.place.momentCount - 1} more at this place", style = MaterialTheme.typography.bodySmall, color = Museum.muted)
                if (stop.note.isNotEmpty()) Text(stop.note, style = MaterialTheme.typography.bodySmall, color = Museum.muted)
            }
        }
    }
}

@Composable
fun MomentCard(moment: MomentSummary, prominent: Boolean = false, onClick: () -> Unit) {
    if (prominent) {
        Box(Modifier.clickable(onClick = onClick)) {
            HeroCard(moment.heroMedia?.publicUrl, height = 200) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    if (moment.dateDisplay.isNotEmpty()) Overline(moment.dateDisplay)
                    Text(moment.title, style = MaterialTheme.typography.titleLarge, color = Museum.text)
                    Text(moment.excerpt, style = MaterialTheme.typography.bodySmall, color = Museum.text, maxLines = 3, overflow = TextOverflow.Ellipsis)
                    if (moment.hasNarration) Overline("Audio guide", Museum.text.copy(alpha = 0.75f))
                }
            }
        }
    } else {
        Card(onClick = onClick) {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                if (moment.dateDisplay.isNotEmpty()) Text(moment.dateDisplay, style = MaterialTheme.typography.labelSmall, color = Museum.action)
                Text(moment.title, style = MaterialTheme.typography.titleMedium, color = Museum.text)
                Text(moment.excerpt, style = MaterialTheme.typography.bodyMedium, color = Museum.muted, maxLines = Int.MAX_VALUE, overflow = TextOverflow.Clip)
            }
        }
    }
}

@Composable
fun MediaGallery(media: List<MediaAsset>) {
    LazyRow(horizontalArrangement = Arrangement.spacedBy(14.dp)) {
        items(media, key = { it.id }) { asset ->
            Column(Modifier.width(300.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                val shape = RoundedCornerShape(8.dp)
                Box(Modifier.fillMaxWidth().height(210.dp).clip(shape).background(Museum.surfaceElevated).border(1.dp, Museum.border, shape), contentAlignment = Alignment.Center) {
                    if (asset.kind == "image") {
                        AsyncImage(model = asset.publicUrl, contentDescription = asset.caption, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize())
                    } else {
                        Text(if (asset.kind == "video") "Video" else "Audio", color = Museum.muted)
                    }
                }
                if (asset.caption.isNotEmpty()) Text(asset.caption, style = MaterialTheme.typography.bodyMedium, color = Museum.text, maxLines = 3, overflow = TextOverflow.Ellipsis)
                val credit = listOf(asset.credit, asset.license).filter { it.isNotEmpty() }.joinToString(" · ").ifEmpty { "Credit unavailable" }
                Text(credit, style = MaterialTheme.typography.bodySmall, color = Museum.faint)
            }
        }
    }
}
