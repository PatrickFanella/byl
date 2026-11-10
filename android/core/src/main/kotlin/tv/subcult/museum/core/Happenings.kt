package tv.subcult.museum.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.net.URI
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Locale

@Serializable
data class Happening(
    val id: String,
    @SerialName("revision_id") val revisionId: String,
    @SerialName("place_id") val placeId: String,
    val title: String, val summary: String, val organizer: String,
    @SerialName("starts_on") val startsOn: String,
    @SerialName("ends_on") val endsOn: String,
    val timezone: String,
    @SerialName("declared_state") val declaredState: String,
    val state: String,
    @SerialName("review_on") val reviewOn: String,
    @SerialName("expires_on") val expiresOn: String,
    @SerialName("last_checked_at") val lastCheckedAt: String,
    @SerialName("source_health") val sourceHealth: String,
    val sources: List<HappeningSource>, val issues: List<String>
) {
    fun presentationStatus(now: Instant): HappeningStatus = try {
        require(state in setOf("upcoming", "active", "ended", "expired", "needs_review", "cancelled", "date_conflict", "source_unavailable"))
        require(declaredState in setOf("scheduled", "cancelled", "date_conflict", "source_unavailable"))
        listOf(startsOn, endsOn, reviewOn, expiresOn).forEach { require(it.matches(Regex("\\d{4}-\\d{2}-\\d{2}"))); LocalDate.parse(it) }
        require(endsOn >= startsOn)
        val today = now.atZone(ZoneId.of(timezone)).toLocalDate().toString()
        val checked = parseInstant(lastCheckedAt)
        when {
            declaredState == "cancelled" || state == "cancelled" -> HappeningStatus.CANCELLED
            declaredState == "date_conflict" || state == "date_conflict" -> HappeningStatus.DATE_CONFLICT
            state == "expired" || today >= expiresOn -> HappeningStatus.EXPIRED
            sourceHealth == "unavailable" || declaredState == "source_unavailable" || state == "source_unavailable" -> HappeningStatus.SOURCE_UNAVAILABLE
            sourceHealth != "unchanged" || issues.isNotEmpty() || state == "needs_review" || checked > now || now.epochSecond - checked.epochSecond >= 7 * 86400 || today >= reviewOn || sources.none { sourceURL(it.url) != null } -> HappeningStatus.NEEDS_REVIEW
            state == "ended" || today > endsOn -> HappeningStatus.ENDED
            today < startsOn -> HappeningStatus.UPCOMING
            else -> HappeningStatus.ACTIVE
        }
    } catch (_: Exception) { HappeningStatus.UNAVAILABLE }

    val occurrenceText: String get() = try {
        val f = DateTimeFormatter.ofPattern("MMM d, uuuu", Locale.US)
        "${LocalDate.parse(startsOn).format(f)}${if (startsOn == endsOn) "" else " – ${LocalDate.parse(endsOn).format(f)}"} · $timezone · dates only"
    } catch (_: Exception) { "Occurrence dates unavailable" }
    val checkedText: String get() = try {
        "Last source check: ${parseInstant(lastCheckedAt).atOffset(ZoneOffset.UTC).format(DateTimeFormatter.ofPattern("uuuu-MM-dd HH:mm 'UTC'", Locale.US))}"
    } catch (_: Exception) { "Source check unavailable" }

    companion object {
        fun sourceURL(value: String): String? = try {
            val uri = URI(value)
            if (uri.scheme?.lowercase() in setOf("http", "https") && uri.host != null && uri.rawUserInfo == null) value else null
        } catch (_: Exception) { null }
    }
}

@Serializable
data class HappeningSource(
    val url: String, val title: String, val publisher: String, val version: String,
    @SerialName("retrieved_on") val retrievedOn: String,
    val license: String,
    @SerialName("license_url") val licenseUrl: String
)

enum class HappeningStatus(val wire: String, val label: String) {
    ACTIVE("active", "Scheduled for these dates"), UPCOMING("upcoming", "Upcoming"), ENDED("ended", "Dates passed"),
    EXPIRED("expired", "Expired notice"), CANCELLED("cancelled", "Cancelled"), DATE_CONFLICT("date_conflict", "Dates conflict"),
    NEEDS_REVIEW("needs_review", "Needs review"), SOURCE_UNAVAILABLE("source_unavailable", "Source unavailable"), UNAVAILABLE("unavailable", "Notice unavailable")
}
