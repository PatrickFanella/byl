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
data class SuggestionIntake(val enabled: Boolean = false)

@Serializable
data class RouteSuggestionInput(
    val topic: String,
    val area: String,
    val notes: String = "",
    @SerialName("place_leads") val placeLeads: List<String> = emptyList(),
    @SerialName("event_leads") val eventLeads: List<String> = emptyList(),
    @SerialName("source_links") val sourceLinks: List<String> = emptyList(),
    val modes: List<String> = listOf("walk", "bike"),
    @SerialName("trip_shape") val tripShape: String = "flexible",
    @SerialName("duration_minutes") val durationMinutes: Int? = null,
    @SerialName("start_preference") val startPreference: String = "",
    @SerialName("end_preference") val endPreference: String = "",
    @SerialName("pilot_id") val pilotId: String = ""
)

@Serializable
data class RouteSuggestion(
    val id: String,
    @SerialName("owner_account_id") val ownerAccountId: String,
    val submitted: RouteSuggestionInput,
    val status: String,
    val reason: String = "",
    val version: Int,
    @SerialName("created_at") val createdAt: String,
    @SerialName("updated_at") val updatedAt: String
)

data class SuggestionStatus(val label: String, val detail: String)

fun suggestionStatus(status: String): SuggestionStatus = when (status) {
    "draft" -> SuggestionStatus("Received", "Saved privately for curator triage. Research has not started yet.")
    "processing" -> SuggestionStatus("Research in progress", "Research is being prepared. Check back for an update.")
    "needs_research" -> SuggestionStatus("More research needed", "Some questions or sources still need investigation.")
    "review" -> SuggestionStatus("Curator review", "The proposal is waiting for a curator decision.")
    "accepted" -> SuggestionStatus("Accepted", "Only content approved through curator publication appears as a public tour.")
    "declined" -> SuggestionStatus("Declined", "This suggestion is not being taken forward. Read the decision below.")
    else -> SuggestionStatus("Status unavailable", "Refresh to check the current submission status.")
}

@Serializable
data class SuggestionFields(
    val topic: String = "", val area: String = "", val notes: String = "",
    val places: String = "", val events: String = "", val sources: String = ""
) {
    fun input() = RouteSuggestionInput(topic.trim(), area.trim(), notes, lines(places), lines(events), lines(sources))
    val hasOptional: Boolean get() = listOf(notes, places, events, sources).any { it.isNotEmpty() }
    private fun lines(value: String) = value.lines().map { it.trim() }.filter { it.isNotEmpty() }
}

@Serializable
data class SuggestionAttempt(val key: String, val input: RouteSuggestionInput)

@Serializable
private data class StoredSuggestionDraft(val fields: SuggestionFields, val pending: SuggestionAttempt? = null)

data class RouteSuggestionsState(
    val fields: SuggestionFields = SuggestionFields(),
    val pending: SuggestionAttempt? = null,
    val busy: Boolean = false,
    val message: String? = null,
    val fieldErrors: Map<String, String> = emptyMap(),
    val needsSignIn: Boolean = false,
    val intake: Boolean? = null,
    val intakeLoading: Boolean = false,
    val intakeError: String? = null,
    val items: List<RouteSuggestion> = emptyList(),
    val listLoading: Boolean = false,
    val listError: String? = null,
    val offset: Int = 0,
    val hasMore: Boolean = false,
    val selectedId: String? = null,
    val selected: RouteSuggestion? = null,
    val detailLoading: Boolean = false,
    val detailError: String? = null
) {
    val canEdit: Boolean get() = !busy && pending == null
    val canSubmit: Boolean get() = intake == true && !busy && !needsSignIn &&
        (pending != null || (fields.topic.isNotBlank() && fields.area.isNotBlank()))
}

/** One account and API origin, with a credential-bound Endpoints supplied by the app.
 * Only unfinished input and its exact retry identity go to private app storage.
 * Server status is reloaded; old account responses never populate a new screen.
 */
class RouteSuggestions(
    private val api: Endpoints,
    private val store: KeyValueStore,
    private val accountId: String,
    private val isCurrent: () -> Boolean
) {
    private val origin = api.client.baseUrl.trimEnd('/')
    private val originHash = MessageDigest.getInstance("SHA-256").digest(origin.toByteArray()).joinToString("") { "%02x".format(it) }
    private val key = "suggestion.v1.$originHash.$accountId"
    private val _state = MutableStateFlow(RouteSuggestionsState())
    val state: StateFlow<RouteSuggestionsState> = _state
    private val submission = Mutex()
    @Volatile private var active = true
    private val pageRequest = AtomicInteger()
    private val detailRequest = AtomicInteger()
    private val intakeRequest = AtomicInteger()
    private fun current() = active && isCurrent()

    init {
        require(accountId.isNotBlank())
        if (isCurrent()) runCatching { store.getJson<StoredSuggestionDraft>(key) }.getOrNull()?.let { draft ->
            _state.value = RouteSuggestionsState(fields = draft.fields, pending = draft.pending)
        }
    }

    fun edit(fields: SuggestionFields) {
        if (!current() || !_state.value.canEdit) return
        _state.update { if (!current() || !it.canEdit) it else it.copy(fields = fields, message = null, fieldErrors = emptyMap()) }
        if (!persist()) _state.update { if (!current()) it else it.copy(message = "Could not save this draft on the device. Keep this screen open and try again.") }
    }

    fun discardLocalDraft() {
        if (!current() || _state.value.busy) return
        try { store.remove(key) }
        catch (_: Exception) {
            _state.update { if (!current()) it else it.copy(message = "Could not remove the saved draft. Try again.") }
            return
        }
        _state.update { if (!current()) it else it.copy(fields = SuggestionFields(), pending = null, fieldErrors = emptyMap(), message = null) }
    }

    private fun persist(draft: StoredSuggestionDraft = _state.value.let { StoredSuggestionDraft(it.fields, it.pending) }): Boolean = runCatching {
        store.putJson(key, draft)
    }.isSuccess

    suspend fun loadIntake() {
        if (!current()) return
        val request = intakeRequest.incrementAndGet()
        _state.update { if (!current() || request != intakeRequest.get()) it else it.copy(intake = null, intakeLoading = true, intakeError = null) }
        try {
            val result = api.suggestionIntake()
            _state.update { if (!current() || request != intakeRequest.get()) it else it.copy(intake = result.enabled, intakeLoading = false) }
        } catch (e: CancellationException) { throw e }
        catch (e: Exception) {
            _state.update { if (!current() || request != intakeRequest.get()) it else it.copy(intake = null, intakeLoading = false, intakeError = "Could not check whether new suggestions are open.") }
        }
    }

    suspend fun loadPage(offset: Int = _state.value.offset) {
        if (!current()) return
        val request = pageRequest.incrementAndGet()
        val pageOffset = offset.coerceAtLeast(0)
        _state.update { if (!current() || request != pageRequest.get()) it else it.copy(items = emptyList(), offset = pageOffset, hasMore = false, listLoading = true, listError = null) }
        try {
            val result = api.suggestions(pageOffset)
            if (result.any { it.ownerAccountId != accountId }) throw ApiException(403, "owner_mismatch", "Private submissions are unavailable.")
            _state.update { if (!current() || request != pageRequest.get()) it else it.copy(items = result.take(20), hasMore = result.size > 20, listLoading = false) }
        } catch (e: CancellationException) { throw e }
        catch (e: Exception) {
            _state.update { if (!current() || request != pageRequest.get()) it else it.copy(listLoading = false, listError = "Could not load your private submissions. Try again when connected.") }
        }
    }

    suspend fun loadDetail(id: String) {
        if (!current()) return
        val request = detailRequest.incrementAndGet()
        _state.update { if (!current() || request != detailRequest.get()) it else it.copy(selectedId = id, selected = null, detailLoading = true, detailError = null, message = null) }
        try {
            val result = api.suggestion(id)
            if (result.ownerAccountId != accountId || result.id != id) throw ApiException(404, "not_found", "Suggestion unavailable.")
            _state.update { if (!current() || request != detailRequest.get()) it else it.copy(selected = result, detailLoading = false) }
        } catch (e: CancellationException) { throw e }
        catch (e: Exception) {
            _state.update { if (!current() || request != detailRequest.get()) it else it.copy(detailLoading = false, detailError = if (e is ApiException && (e.status == 404 || e.status == 403)) "This suggestion is not available for this account." else "Could not load this suggestion. Try again when connected.") }
        }
    }

    fun showList() {
        if (!current()) return
        detailRequest.incrementAndGet()
        _state.update { if (!current()) it else it.copy(selectedId = null, selected = null, detailError = null, detailLoading = false, message = null) }
    }

    /** A second tap is ignored. A restarted flow reuses the persisted body/key. */
    suspend fun submit(): RouteSuggestion? {
        if (!current() || !submission.tryLock()) return null
        try {
            val state = _state.value
            if (!state.canSubmit) return null
            val attempt = state.pending ?: SuggestionAttempt(UUID.randomUUID().toString(), state.fields.input())
            _state.update { if (!current()) it else it.copy(pending = attempt, busy = true, message = null, fieldErrors = emptyMap()) }
            if (!persist(StoredSuggestionDraft(state.fields, attempt))) {
                _state.update { if (!current()) it else it.copy(busy = false, message = "Could not save the draft on this device. No submission was sent.") }
                return null
            }
            if (!current()) return null
            return try {
                val result = api.submitSuggestion(attempt.input, attempt.key)
                if (!current()) return null
                if (result.ownerAccountId != accountId) throw ApiException(0, "unconfirmed", "Submission not confirmed.")
                val cleanupMessage = try {
                    store.remove(key)
                    null
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    "Saved on the server. The local draft could not be cleared; retrying uses the same suggestion."
                }
                _state.update {
                    if (!current()) it else it.copy(
                        fields = if (cleanupMessage == null) SuggestionFields() else it.fields,
                        pending = if (cleanupMessage == null) null else it.pending,
                        busy = false,
                        selectedId = result.id,
                        selected = result,
                        detailLoading = false,
                        detailError = null,
                        message = cleanupMessage ?: "Suggestion saved privately. Curators review it before any publication."
                    )
                }
                result
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                if (!current()) return null
                val error = e as? ApiException
                when {
                    error?.code == "intake_disabled" -> _state.update { if (!current()) it else it.copy(intake = false, message = "New suggestions are closed. Your draft is kept on this device for retrying later.") }
                    error?.status == 401 -> _state.update { if (!current()) it else it.copy(needsSignIn = true, message = "Sign in again to continue. Your original suggestion is kept on this device.") }
                    error != null && error.status in listOf(400, 404, 422, 429) -> _state.update { if (!current()) it else it.copy(pending = null, fieldErrors = error.fields, message = if (error.fields.isNotEmpty()) "Check the marked fields and submit again." else error.message) }
                    else -> _state.update { if (!current()) it else it.copy(message = "Could not confirm the submission. Retry sends the same suggestion; check your submissions before starting another.") }
                }
                persist()
                null
            } finally {
                if (current()) _state.update { if (!current()) it else it.copy(busy = false) }
            }
        } finally { submission.unlock() }
    }

    /** Cancel UI ownership without deleting the resumable local draft. */
    fun close() { active = false; _state.value = RouteSuggestionsState() }
}
