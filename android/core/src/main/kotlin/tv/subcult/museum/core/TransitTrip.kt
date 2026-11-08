package tv.subcult.museum.core

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import java.net.URLEncoder
import java.security.MessageDigest
import java.time.Duration
import java.time.Instant

data class TransitTripState(
    val selected: ScheduledTransitPlan? = null, val proposed: ScheduledTransitPlan? = null,
    val guidanceBlocked: Boolean = false, val enabled: Boolean? = null,
    val busy: Boolean = false, val message: String? = null, val requiresRemoval: Boolean = false
)

/** Opening or refreshing a trip never chooses another train. */
class TransitTrip(
    private val api: Endpoints, val revision: TourRevision, private val store: KeyValueStore,
    accountId: String?, private val isCurrent: () -> Boolean = { true }, private val now: () -> Instant = Instant::now
) {
    @Serializable private data class Retained(val version: Int, val plan: ScheduledTransitPlan, val guidanceBlocked: Boolean)
    private val origin = api.client.baseUrl
    private val key = "transit-plan.v1." + MessageDigest.getInstance("SHA-256").digest(listOf(origin, accountId ?: "anonymous", revision.tourId, revision.id).joinToString("\n").toByteArray()).joinToString("") { "%02x".format(it) }
    private val lock = Any()
    private val _state = MutableStateFlow(TransitTripState())
    val state: StateFlow<TransitTripState> = _state
    private var generation = 0
    private var gateGeneration = 0
    private var closed = false
    private var storageBlocked = false
    private var admittedAt: Instant? = null
    private fun current() = isCurrent() && api.client.baseUrl == origin
    init {
        try {
            val text = store.get(key)
            val value = text?.let { MuseumJson.decodeFromString<Retained>(it) }
            if (value != null && value.version == 1 && value.plan.valid(revision)) _state.value = TransitTripState(selected = value.plan, guidanceBlocked = value.guidanceBlocked)
            else if (text != null) throw IllegalStateException()
        } catch (_: Exception) {
            storageBlocked = true
            _state.value = TransitTripState(message = "The saved transit choice could not be read. Remove it explicitly before choosing another plan.", requiresRemoval = true)
        }
    }
    suspend fun refreshGate() {
        val request = synchronized(lock) { if (closed || !current()) { invalidate(); return }; ++gateGeneration }
        try {
            val gate = api.itineraryCapabilities()
            synchronized(lock) {
                if (closed || !current() || request != gateGeneration) return
                admittedAt = if (gate.scheduledTransitV1) now() else null
                _state.value = _state.value.copy(enabled = gate.scheduledTransitV1, message = if (gate.scheduledTransitV1) _state.value.message?.takeUnless { it == "Scheduled transit is disabled. Directions are suspended." || it == "Could not confirm scheduled transit availability. Directions are suspended." } else "Scheduled transit is disabled. Directions are suspended.")
            }
        } catch (e: CancellationException) { throw e }
        catch (_: Exception) {
            synchronized(lock) {
                if (closed || !current() || request != gateGeneration) return
                admittedAt = null; _state.value = _state.value.copy(enabled = null, message = "Could not confirm scheduled transit availability. Directions are suspended.")
            }
        }
    }
    suspend fun request(departure: Instant) {
        val request = synchronized(lock) {
            if (closed || !current() || _state.value.busy || storageBlocked) return
            if (departure <= now() || departure > now().plusSeconds(7 * 86400)) {
                _state.value = _state.value.copy(message = "Choose a future departure from the first connector story stop within seven days."); return
            }
            _state.value = _state.value.copy(busy = true, proposed = null, message = null); ++generation
        }
        try {
            refreshGate()
            synchronized(lock) { if (closed || !current() || request != generation || _state.value.enabled != true) return }
            val result = api.transitPlan(revision.tourId, revision.id, departure.toString())
            synchronized(lock) {
                if (closed || !current() || request != generation) return
                if (!result.valid(revision) || transitInstant(result.requestedDepartureAt) != departure) {
                    _state.value = _state.value.copy(message = "The timetable response did not match this revision and departure. Your retained choice is unchanged."); return
                }
                _state.value = _state.value.copy(proposed = result)
                if (result.state in listOf("needs_review", "unsupported") || (result.state != "scheduled" && _state.value.selected?.requestedDepartureAt?.let(::transitInstant) == departure)) suspendGuidance()
            }
        } catch (e: CancellationException) { throw e }
        catch (e: Exception) {
            synchronized(lock) {
                if (closed || !current() || request != generation) return
                val refused = e is ApiException && e.status in listOf(403, 404)
                if (refused) admittedAt = null
                _state.value = _state.value.copy(enabled = if (refused) false else _state.value.enabled, message = "Could not plan this departure. Your retained choice is unchanged.")
            }
        } finally {
            synchronized(lock) { if (request == generation) _state.value = _state.value.copy(busy = false) }
        }
    }
    fun accept() = synchronized(lock) {
        val proposal = _state.value.proposed
        if (closed || !current() || storageBlocked || _state.value.busy || proposal == null || !proposal.valid(revision) || proposal.unavailableReason(now()) != null || !gateCurrent())
            throw ApiException(0, "transit_unavailable", "Refresh availability and review a usable future plan before accepting it.")
        persist(Retained(1, proposal, false))
        _state.value = _state.value.copy(selected = proposal, proposed = null, guidanceBlocked = false, message = null)
    }
    fun remove() {
        synchronized(lock) {
            if (closed || !current()) return
            store.remove(key); generation++; storageBlocked = false
            _state.value = _state.value.copy(selected = null, proposed = null, guidanceBlocked = false, busy = false, message = null, requiresRemoval = false)
        }
    }
    fun invalidate() = synchronized(lock) {
        closed = true; generation++; gateGeneration++; admittedAt = null
        _state.value = _state.value.copy(enabled = null, proposed = null, busy = false)
    }
    private fun gateCurrent() = _state.value.enabled == true && admittedAt?.let { Duration.between(it, now()).toMillis() in 0..60000 } == true
    fun canGuide(at: Instant): Boolean = synchronized(lock) {
        !closed && current() && !_state.value.guidanceBlocked && gateCurrent() && _state.value.selected?.unavailableReason(at) == null && _state.value.selected != null
    }
    private fun suspendGuidance() {
        val chosen = _state.value.selected ?: return
        _state.value = _state.value.copy(guidanceBlocked = true)
        try { persist(Retained(1, chosen, true)) }
        catch (_: Exception) {
            runCatching { store.remove(key) }
            _state.value = _state.value.copy(message = "Guidance is suspended. The suspension could not be saved; check device storage before reopening.")
        }
    }
    private fun persist(value: Retained) {
        val text = MuseumJson.encodeToString(value)
        if (text.toByteArray().size > 256 * 1024) throw ApiException(0, "transit_storage", "The timetable choice is too large to retain.")
        val previous = store.get(key)
        try {
            store.put(key, text)
            if (store.get(key) != text) throw ApiException(0, "transit_storage", "The choice could not be read back from device storage.")
        } catch (e: Exception) {
            runCatching { if (previous != null) store.put(key, previous) else store.remove(key) }; throw e
        }
    }
    /** Both endpoints are public references; no device fix or account enters this URL. */
    fun walkingUrl(boarding: Boolean, at: Instant): String? {
      return synchronized(lock) {
        if (!canGuide(at)) return null
        val tour = revision.snapshot?.tour ?: return null; val choice = tour.approvedTransit ?: return null
        val story = tour.stops.firstOrNull { it.id == if (boarding) choice.intent.fromStopId else choice.intent.toStopId }?.place?.arrivalPoint?.navigationPoint("walk") ?: return null
        val platform = if (boarding) choice.board else choice.alight
        val platformPoint = "${platform.latitude},${platform.longitude}"; val storyPoint = "${story.lat},${story.lng}"
        fun encode(s: String) = URLEncoder.encode(s, "UTF-8")
        "https://www.google.com/maps/dir/?api=1&travelmode=walking&origin=${encode(if (boarding) storyPoint else platformPoint)}&destination=${encode(if (boarding) platformPoint else storyPoint)}"
    }
    }
}
