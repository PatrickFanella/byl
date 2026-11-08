package tv.subcult.museum.core

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.security.MessageDigest
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger

@Serializable
data class ReportLocation(
    val kind: String,
    @SerialName("stop_id") val stopId: String? = null,
    @SerialName("arrival_id") val arrivalId: String? = null,
    @SerialName("from_stop_id") val fromStopId: String? = null,
    @SerialName("to_stop_id") val toStopId: String? = null
) {
    val key: String get() = if (kind == "segment") "segment:$fromStopId:$toStopId" else "$kind:$stopId"
}
@Serializable data class ReportPoint(val lat: Double, val lng: Double) {
    val valid: Boolean get() = lat.isFinite() && lng.isFinite() && lat in -90.0..90.0 && lng in -180.0..180.0
}
@Serializable data class ReportAlternative(val type: String = "LineString", val coordinates: List<List<Double>>)
@Serializable
data class RouteReportInput(
    @SerialName("tour_id") val tourId: String,
    @SerialName("revision_id") val revisionId: String,
    val mode: String, val affected: ReportLocation,
    val reason: String = "blocked", val details: String = "", val better: String = "",
    val direction: String = "unknown", val duration: String = "unknown",
    @SerialName("observed_on") val observedOn: String? = null,
    @SerialName("source_url") val sourceUrl: String = "",
    val annotation: ReportPoint? = null, val alternative: ReportAlternative? = null
)
@Serializable
data class RouteReport(
    val id: String, @SerialName("owner_account_id") val ownerAccountId: String,
    val submitted: RouteReportInput, val status: String, val reason: String = "", val version: Int,
    @SerialName("created_at") val createdAt: String, @SerialName("updated_at") val updatedAt: String
)
@Serializable
data class RouteCorrectionOutcome(
    @SerialName("revision_id") val revisionId: String,
    @SerialName("previous_revision_id") val previousRevisionId: String,
    val temporary: Boolean,
    @SerialName("review_on") val reviewOn: String? = null,
    @SerialName("reassessment_required") val reassessmentRequired: Boolean = false
)
data class ReportTarget(val label: String, val affected: ReportLocation) { val id get() = affected.key }

/** Select only frozen references, not current draft geometry or unrelated progress. */
data class RouteReportContext(val revision: TourRevision, val mode: String) {
    val unavailableReason: String? get() {
        val snapshot = revision.snapshot
        val route = snapshot?.routes?.get(mode)
        if (mode !in listOf("walk", "bike") || revision.id.isBlank() || snapshot == null || snapshot.tour.id != revision.tourId || snapshot.tour.revisionId != revision.id || snapshot.tour.revisionNumber != revision.number || mode !in snapshot.tour.modes || route?.status != "ready" || route.mode != mode || route.tourId != revision.tourId)
            return "The selected revision has no retained ready path for this mode."
        val stops = snapshot.tour.stops
        if (stops.size !in 2..30 || stops.any { runCatching { UUID.fromString(it.id) }.isFailure } || stops.map { it.id }.distinct().size != stops.size || stops.map { it.position }.toSet() != (1..stops.size).toSet())
            return "The retained stop references could not be read."
        return null
    }
    val targets: List<ReportTarget> get() {
        if (unavailableReason != null) return emptyList()
        val snapshot = revision.snapshot ?: return emptyList()
        val stops = snapshot.tour.stops.sortedBy { it.position }
        val targets = stops.flatMap { stop -> listOf(
            ReportTarget("Stop ${stop.position}: ${stop.place.name}", ReportLocation("stop", stopId = stop.id)),
            ReportTarget("Entrance / access at ${stop.place.name}", ReportLocation("entrance", stopId = stop.id, arrivalId = stop.place.arrivalPoint?.id))
        ) }.toMutableList()
        for (leg in snapshot.routes[mode]?.legs.orEmpty()) {
            val from = stops.firstOrNull { it.position == leg.fromPosition } ?: continue
            val to = stops.firstOrNull { it.position == leg.toPosition } ?: continue
            if (from.id == to.id || leg.fromStopId != null && leg.fromStopId != from.id || leg.toStopId != null && leg.toStopId != to.id) continue
            val affected = ReportLocation("segment", fromStopId = from.id, toStopId = to.id)
            if (targets.none { it.id == affected.key }) targets += ReportTarget("Path from ${from.place.name} to ${to.place.name}${if (leg.isReturn) " (return)" else ""}", affected)
        }
        return targets
    }
}

@Serializable
data class ReportFields(
    val target: String = "", val reason: String = "blocked", val details: String = "", val better: String = "",
    val sourceUrl: String = "", val direction: String = "unknown", val duration: String = "unknown",
    val observedOn: String = "", val latitude: String = "", val longitude: String = "", val alternativePoints: String = ""
) {
    fun input(context: RouteReportContext): RouteReportInput {
        require(context.unavailableReason == null) { context.unavailableReason.orEmpty() }
        val affected = requireNotNull(context.targets.firstOrNull { it.id == target }) { "Choose a retained stop, entrance or path." }.affected
        val annotation = if (latitude.isNotBlank() || longitude.isNotBlank()) {
            val lat = latitude.trim().toDoubleOrNull(); val lng = longitude.trim().toDoubleOrNull()
            require(lat != null && lng != null && ReportPoint(lat, lng).valid) { "Enter both a valid latitude and longitude, or leave both empty." }
            ReportPoint(lat, lng)
        } else null
        val lines = alternativePoints.lines().map { it.trim() }.filter { it.isNotEmpty() }
        val alternative = if (lines.isNotEmpty()) {
            require(lines.size in 2..64) { "An alternative needs 2 to 64 longitude, latitude pairs." }
            ReportAlternative(coordinates = lines.map { line ->
                val values = line.split(',').map { it.trim().toDoubleOrNull() }
                require(values.size == 2 && values[0] != null && values[1] != null && ReportPoint(values[1]!!, values[0]!!).valid) { "Use one valid longitude, latitude pair per line for the alternative." }
                listOf(values[0]!!, values[1]!!)
            })
        } else null
        return RouteReportInput(context.revision.tourId, context.revision.id, context.mode, affected, reason, details.trim(), better.trim(), direction, duration, observedOn.ifBlank { null }, sourceUrl.trim(), annotation, alternative)
    }
}
@Serializable data class ReportAttempt(val key: String, val input: RouteReportInput)
@Serializable private data class StoredReportDraft(val fields: ReportFields, val pending: ReportAttempt? = null)

data class RouteReportsState(
    val fields: ReportFields = ReportFields(), val pending: ReportAttempt? = null,
    val contextReady: Boolean = false, val unreadableDraft: Boolean = false,
    val busy: Boolean = false, val message: String? = null, val fieldErrors: Map<String, String> = emptyMap(),
    val needsSignIn: Boolean = false, val items: List<RouteReport> = emptyList(), val listLoading: Boolean = false,
    val listError: String? = null, val offset: Int = 0, val hasMore: Boolean = false,
    val selectedId: String? = null, val selected: RouteReport? = null,
    val outcome: RouteCorrectionOutcome? = null, val outcomeError: String? = null,
    val detailLoading: Boolean = false, val detailError: String? = null
) {
    val canEdit get() = contextReady && !busy && !unreadableDraft && pending == null && selectedId == null
    val canSubmit get() = contextReady && !busy && !unreadableDraft && !needsSignIn && selectedId == null
}

/** Bound to opening credentials by the app. All unfinished writes are checked before POST. */
class RouteReports(
    private val api: Endpoints, private val store: KeyValueStore, private val accountId: String,
    val context: RouteReportContext? = null, initialTarget: String? = null,
    private val isCurrent: () -> Boolean
) {
    private val originHash = MessageDigest.getInstance("SHA-256").digest(api.client.baseUrl.trimEnd('/').toByteArray()).joinToString("") { "%02x".format(it) }
    private val key = "route-report.v1.$originHash.$accountId.${context?.revision?.id ?: "inbox"}.${context?.mode.orEmpty()}"
    private val initialFields = ReportFields(target = context?.targets?.firstOrNull { it.id == initialTarget }?.id ?: context?.targets?.firstOrNull()?.id.orEmpty())
    private val _state = MutableStateFlow(RouteReportsState(fields = initialFields, contextReady = context != null && context.unavailableReason == null))
    val state: StateFlow<RouteReportsState> = _state
    private val submission = Mutex()
    private val pageRequest = AtomicInteger()
    private val detailRequest = AtomicInteger()
    @Volatile private var active = true
    private fun current() = active && isCurrent()
    init {
        require(accountId.isNotBlank())
        if (context != null && current()) try {
            store.get(key)?.let { raw ->
                val draft = MuseumJson.decodeFromString<StoredReportDraft>(raw)
                draft.pending?.let { require(it.input.tourId == context.revision.tourId && it.input.revisionId == context.revision.id && it.input.mode == context.mode && runCatching { UUID.fromString(it.key) }.isSuccess) }
                _state.value = _state.value.copy(fields = draft.fields, pending = draft.pending)
            }
        } catch (_: Exception) { _state.value = _state.value.copy(unreadableDraft = true, message = "The saved report could not be read. Check Your reports before discarding the local draft.") }
    }
    private fun persist(fields: ReportFields, pending: ReportAttempt?) = store.putJson(key, StoredReportDraft(fields, pending))
    fun edit(fields: ReportFields) {
        if (!current() || !_state.value.canEdit) return
        _state.update { if (!current() || !it.canEdit) it else it.copy(fields = fields, message = null, fieldErrors = emptyMap()) }
        try { persist(fields, null) } catch (_: Exception) { _state.update { if (!current()) it else it.copy(message = "Could not save this draft. Keep this screen open and try again.") } }
    }
    fun discardLocalDraft() {
        if (!current() || _state.value.busy || context == null) return
        try { store.remove(key) }
        catch (_: Exception) { _state.update { if (!current()) it else it.copy(message = "Could not clear the saved report. Input and retry identity are kept.") }; return }
        _state.update { if (!current()) it else it.copy(fields = initialFields, pending = null, unreadableDraft = false, fieldErrors = emptyMap(), message = null) }
    }
    suspend fun submit(): RouteReport? {
        if (!current() || !submission.tryLock()) return null
        try {
            val state = _state.value
            if (!state.canSubmit || context == null) return null
            val attempt = try { state.pending ?: ReportAttempt(UUID.randomUUID().toString(), state.fields.input(context)) }
            catch (e: IllegalArgumentException) { _state.update { if (!current()) it else it.copy(message = e.message) }; return null }
            _state.update { if (!current()) it else it.copy(pending = attempt, busy = true, message = null, fieldErrors = emptyMap()) }
            try { persist(state.fields, attempt) }
            catch (_: Exception) { _state.update { if (!current()) it else it.copy(message = "Could not save the report retry. No report was sent.") }; return null }
            if (!current()) return null
            return try {
                val saved = api.submitReport(attempt.input, attempt.key)
                if (!current()) return null
                if (saved.ownerAccountId != accountId || saved.submitted != attempt.input) throw ApiException(0, "unconfirmed", "Report identity was not confirmed.")
                val cleared = try { store.remove(key); true }
                catch (e: CancellationException) { throw e }
                catch (_: Exception) { false }
                _state.update { if (!current()) it else it.copy(pending = if (cleared) null else attempt, selectedId = saved.id, selected = saved, message = if (cleared) "Report saved privately. Curators review it before a route changes." else "Report saved privately. Its local retry could not be cleared; it still identifies this report.") }
                saved
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                if (!current()) return null
                if (e is ApiException && e.status == 401) _state.update { it.copy(needsSignIn = true, message = "Sign in again. The original report and retry identity are kept.") }
                else if (e is ApiException && e.status in listOf(400, 403, 404, 422, 429)) {
                    try {
                        persist(state.fields, null)
                        _state.update { if (!current()) it else it.copy(pending = null, fieldErrors = e.fields, message = if (e.fields.isEmpty()) e.message else "Check the report fields and submit again.") }
                    } catch (_: Exception) { _state.update { if (!current()) it else it.copy(message = "The report was rejected, but its saved retry could not be updated. Input and retry identity are kept.") } }
                } else _state.update { if (!current()) it else it.copy(message = "Could not confirm the report. Retry sends the same observation; check Your reports before discarding it.") }
                null
            }
        } finally {
            _state.update { if (!current()) it else it.copy(busy = false) }
            submission.unlock()
        }
    }
    suspend fun loadPage(offset: Int = _state.value.offset) {
        if (!current()) return
        val request = pageRequest.incrementAndGet(); val pageOffset = offset.coerceAtLeast(0)
        _state.update { if (!current()) it else it.copy(items = emptyList(), offset = pageOffset, hasMore = false, listLoading = true, listError = null) }
        try {
            val reports = api.reports(pageOffset)
            require(reports.all { it.ownerAccountId == accountId })
            _state.update { if (!current() || pageRequest.get() != request) it else it.copy(items = reports.take(20), hasMore = reports.size > 20, listLoading = false) }
        } catch (e: CancellationException) { throw e }
        catch (e: Exception) { _state.update { if (!current() || pageRequest.get() != request) it else it.copy(listLoading = false, listError = "Could not read your private reports. Try again when connected.", needsSignIn = it.needsSignIn || e is ApiException && e.status == 401) } }
    }
    suspend fun loadDetail(id: String) {
        if (!current()) return
        val request = detailRequest.incrementAndGet()
        _state.update { if (!current()) it else it.copy(selectedId = id, selected = null, outcome = null, outcomeError = null, detailLoading = true, detailError = null) }
        try {
            val report = api.report(id)
            if (report.id != id || report.ownerAccountId != accountId) throw ApiException(404, "not_found", "Report unavailable.")
            if (!current() || detailRequest.get() != request) return
            _state.update { if (!current() || detailRequest.get() != request) it else it.copy(selected = report) }
            try {
                val outcome = api.reportCorrection(id)
                require(outcome.previousRevisionId == report.submitted.revisionId && outcome.revisionId.isNotBlank())
                _state.update { if (!current() || detailRequest.get() != request) it else it.copy(outcome = outcome) }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) { if (!(e is ApiException && e.status == 404)) _state.update { if (!current() || detailRequest.get() != request) it else it.copy(outcomeError = "The report is available, but its approved-update status could not be read.") } }
        } catch (e: CancellationException) { throw e }
        catch (e: Exception) { _state.update { if (!current() || detailRequest.get() != request) it else it.copy(detailError = if (e is ApiException && e.status in listOf(403, 404)) "This report is not available for this account." else "Could not read this report. Try again when connected.", needsSignIn = it.needsSignIn || e is ApiException && e.status == 401) } }
        finally { _state.update { if (!current() || detailRequest.get() != request) it else it.copy(detailLoading = false) } }
    }
    fun showList() {
        if (!current()) return
        detailRequest.incrementAndGet()
        _state.update { if (!current()) it else it.copy(selectedId = null, selected = null, outcome = null, outcomeError = null, detailLoading = false, detailError = null, message = null) }
    }
    fun close() { active = false; _state.value = RouteReportsState() }
}
