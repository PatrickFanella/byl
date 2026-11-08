package tv.subcult.museum.ui.tours

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.dp
import tv.subcult.museum.core.RouteJourney
import tv.subcult.museum.core.TourDetail
import tv.subcult.museum.core.distanceText
import tv.subcult.museum.core.modeLabel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.theme.Museum

/** The retained public itinerary projection, with estimates separated from field measurements. */
@Composable
fun TripJourney(tour: TourDetail, mode: String, journey: RouteJourney?) {
    Card {
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Overline("${modeLabel(mode)} itinerary")
            tour.corridor?.notices(mode)?.forEach { Text(it, color = Museum.text, style = MaterialTheme.typography.bodyMedium) }
            if (journey == null) {
                Text("Reviewed path details are unavailable. A computed distance alone is not a reviewed trip.", color = Museum.muted)
            } else {
                Text(if (journey.usable) "Reviewed path ready" else "Path ${journey.state.replace('_', ' ')}", color = Museum.text, style = MaterialTheme.typography.titleMedium)
                Text("Computation: ${journey.computation} · Quality: ${journey.quality}", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                Text("Travel ${minutes(journey.travelDurationS)} · Stops ${minutes(journey.dwellDurationS)} · Story allowance ${minutes(journey.narrationDurationS)}", color = Museum.text)
                Text("Itinerary total: ${minutes(journey.itineraryDurationS)}", color = Museum.text)
                journey.travelDistanceM?.let { Text("Travel distance: ${distanceText(it)}", color = Museum.text) }
                Text(if (journey.returnPlan.status == "included") "Return included · Round trip ${minutes(journey.roundTripDurationS)}" else "Return ${journey.returnPlan.status.replace('_', ' ')} · No round-trip total", color = Museum.muted)
                journey.segments.forEach { segment ->
                    Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                        Text("${segment.kind.replaceFirstChar { it.uppercase() }}${if (segment.reverse) " · Reverse traversal" else ""}", color = Museum.text)
                        Text("Surface: ${segment.surface ?: "unknown"} · Modes: ${segment.modes?.joinToString() ?: "unknown"} · Access: ${segment.access}", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                        Text("Retained reference: ${segment.source.title} · ${segment.source.publisher}", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                        if (segment.source.url.isNotBlank()) Text(segment.source.url, color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                    }
                }
                journey.assumptions.forEach { Text(it, color = Museum.muted, style = MaterialTheme.typography.bodySmall) }
            }
            tour.stops.sortedBy { it.position }.forEach { stop ->
                Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                    Text("${stop.position}. ${stop.place.name}", color = Museum.text, style = MaterialTheme.typography.titleSmall)
                    val arrival = stop.place.arrivalPoint
                    if (arrival == null) Text("Reviewed arrival access is unknown for this mode.", color = Museum.muted)
                    else {
                        Text("${arrival.point.kind.replaceFirstChar { it.uppercase() }}: ${arrival.point.name}", color = Museum.text)
                        if (arrival.point.notes.isNotBlank()) Text(arrival.point.notes, color = Museum.muted)
                        arrival.point.access.filter { it.mode == mode }.forEach { access ->
                            Text("$mode: ${access.status} · Surface: ${access.surface}", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                            if (access.notes.isNotBlank()) Text(access.notes, color = Museum.muted)
                            if (access.sourceTitle.isNotBlank()) Text("${access.sourceTitle} · Retrieved ${access.retrievedAt}", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                    if (stop.note.isNotBlank()) Text(stop.note, color = Museum.muted)
                }
            }
        }
    }
}

private fun minutes(seconds: Int?): String = seconds?.let { "${((it.toLong() + 59) / 60).coerceAtLeast(0)} min" } ?: "unavailable"
