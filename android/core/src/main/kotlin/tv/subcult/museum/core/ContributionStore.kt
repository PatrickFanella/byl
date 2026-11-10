package tv.subcult.museum.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import java.io.File
import java.net.URI
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.util.UUID

@Serializable
data class ContributionChoice(
    val version: Int = 1, val accepted: Boolean = true,
    @SerialName("tour_id") val tourId: String,
    @SerialName("revision_id") val revisionId: String,
    val mode: String, @SerialName("requested_on") val requestedOn: String
)
@Serializable
data class ContributionConsent(val id: String, @SerialName("deletion_handle") val deletionHandle: String, @SerialName("expires_on") val expiresOn: String)
@Serializable
internal data class PendingDeviation(val key: String, val summary: DeviationSummary)
@Serializable
internal data class ContributionRecord(
    val origin: String, val owner: String, val choice: ContributionChoice, val key: String,
    val enabled: Boolean, val withdrawing: Boolean, val consent: ContributionConsent? = null, val queue: List<PendingDeviation> = emptyList()
) {
    val id get() = listOf(origin, owner, choice.tourId, choice.revisionId, choice.mode).joinToString("|")
}
internal class ContributionStorageException : Exception("Private contribution storage is unavailable.")
internal fun contributionDay(time: Long) = Instant.ofEpochMilli(time).atZone(ZoneOffset.UTC).toLocalDate().toString()
internal fun day(value: String): LocalDate? = runCatching { LocalDate.parse(value).takeIf { it.toString() == value && it.year in 1970..9999 } }.getOrNull()

/** App supplies a private noBackupFilesDir leaf. No navigation-cache fallback is allowed. */
class ContributionStore(private val directory: File, private val now: () -> Long = System::currentTimeMillis) {
    private var erasureWritePending = false
    private val file = directory.resolve("contributions-v1.json")
    private val json = Json { encodeDefaults = true; explicitNulls = false; ignoreUnknownKeys = false }
    var choiceGeneration = 0L
        private set
    init { if (!directory.isDirectory && !directory.mkdirs()) throw ContributionStorageException() }

    @Synchronized internal fun read(): List<ContributionRecord> = checked {
        if (!file.exists()) return@checked emptyList()
        val bytes = file.inputStream().use { stream ->
            val data = java.io.ByteArrayOutputStream()
            val buffer = ByteArray(8192)
            while (true) { val count = stream.read(buffer); if (count < 0) break; data.write(buffer, 0, count); require(data.size() <= 131072) }
            data.toByteArray()
        }
        json.decodeFromString<List<ContributionRecord>>(bytes.toString(Charsets.UTF_8)).also(::validate)
    }
    @Synchronized internal fun write(records: List<ContributionRecord>) = checked {
        validate(records)
        val bytes = json.encodeToString(records).toByteArray(Charsets.UTF_8)
        require(bytes.size <= 131072 && directory.isDirectory)
        val temporary = directory.resolve("contributions-v1.pending")
        try {
            temporary.outputStream().use { it.write(bytes); it.fd.sync() }
            Files.move(temporary.toPath(), file.toPath(), StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
            require(read() == records)
        } finally { temporary.delete() }
    }
    @Synchronized internal fun update(id: String, key: String, change: (ContributionRecord) -> ContributionRecord) {
        val records = read().toMutableList(); val index = records.indexOfFirst { it.id == id && it.key == key }
        if (index < 0) throw ContributionStorageException()
        records[index] = change(records[index]); write(records)
    }
    @Synchronized internal fun prune() {
        if (erasureWritePending) stopAll()
        val today = contributionDay(now())
        val current = read()
        val records = current.filter { r -> (r.consent?.expiresOn ?: day(r.choice.requestedOn)!!.plusDays(31).toString()) > today }
            .map { r -> r.copy(queue = r.queue.filter { it.summary.observationPeriod <= today && day(it.summary.observationPeriod)!!.plusDays(7).toString() > today }) }
        // Pruning runs on every refresh; rewrite and fsync only when something actually expired.
        if (records != current) write(records)
    }
    @Synchronized internal fun stopAll() {
        choiceGeneration++
        erasureWritePending = true
        write(read().map { it.copy(enabled = false, withdrawing = true, queue = emptyList()) })
        erasureWritePending = false
    }
    private inline fun <T> checked(work: () -> T): T = try { work() } catch (_: Exception) { throw ContributionStorageException() }
    private fun validate(records: List<ContributionRecord>) {
        require(records.size <= 16 && records.map { it.id }.toSet().size == records.size)
        require(records.all { r ->
            val url = URI(r.origin)
            url.scheme in listOf("http", "https") && !url.host.isNullOrBlank() && url.rawUserInfo == null && url.rawQuery == null && url.rawFragment == null && r.origin.toByteArray().size <= 2048 &&
                uuid(r.owner) && uuid(r.choice.tourId) && uuid(r.choice.revisionId) && r.choice.version == 1 && r.choice.accepted && r.choice.mode in listOf("walk", "bike") && day(r.choice.requestedOn) != null && validKey(r.key) &&
                r.queue.size <= 32 && r.queue.map { it.key }.toSet().size == r.queue.size && (r.consent == null || validConsent(r.consent, r.choice)) &&
                (r.consent != null || r.queue.isEmpty()) && (!r.withdrawing || (!r.enabled && r.queue.isEmpty())) &&
                r.queue.all { validKey(it.key) && validSummary(it.summary) && it.summary.revisionId == r.choice.revisionId && it.summary.mode == r.choice.mode }
        })
    }
    companion object {
        internal fun uuid(s: String) = runCatching { UUID.fromString(s).toString().equals(s, ignoreCase = true) }.getOrDefault(false)
        private fun validKey(s: String) = s.length in 16..128 && s.all { it.code in 33..126 }
        internal fun validConsent(consent: ContributionConsent, choice: ContributionChoice): Boolean {
            val requested = day(choice.requestedOn) ?: return false
            val expiry = day(consent.expiresOn) ?: return false
            return uuid(consent.id) && consent.deletionHandle.length == 64 && consent.deletionHandle.all { it in "0123456789abcdefABCDEF" } && expiry >= requested.plusDays(30) && expiry <= requested.plusDays(31)
        }
        internal fun validSummary(s: DeviationSummary) = s.schemaVersion == 1 && s.detectorVersion == DeviationDetector.VERSION && !s.calibrated && uuid(s.revisionId) && s.segmentId.isNotBlank() && s.segmentId.toByteArray().size <= 120 &&
            s.mode in listOf("walk", "bike") && s.direction in listOf("forward", "reverse") && day(s.observationPeriod) != null && s.qualifiedPassages == 1 && s.sustainedDepartureRejoins in 0..1
    }
}
