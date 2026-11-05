package tv.subcult.museum.core

import kotlin.math.max
import kotlin.math.roundToInt

/** walk → "walk", bike → "ride". */
fun modeVerb(mode: String): String = if (mode == "bike") "ride" else "walk"

fun modeLabel(mode: String): String = if (mode == "bike") "Biking" else "Walking"

/** "24 min walk" / "7 min ride" for a ready route, null otherwise. */
fun estimateText(route: RouteSummary?): String? {
    if (route == null || route.status != "ready") return null
    val minutes = route.estimatedMinutes ?: (route.durationS / 60.0).roundToInt()
    return "${max(minutes, 1)} min ${modeVerb(route.mode)}"
}

/** "850 m" under a kilometre, "1.8 km" above. */
fun distanceText(meters: Double): String =
    if (meters < 1000) "${meters.roundToInt()} m" else String.format(java.util.Locale.US, "%.1f km", meters / 1000)
