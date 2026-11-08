package tv.subcult.museum.ui.routes

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.error
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch
import tv.subcult.museum.core.ReportFields
import tv.subcult.museum.core.RouteReportContext
import tv.subcult.museum.core.RouteReports
import tv.subcult.museum.core.Session
import tv.subcult.museum.core.TourRevision
import tv.subcult.museum.core.suggestionStatus
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.ErrorCard
import tv.subcult.museum.ui.components.LoadingRow
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SignInPrompt
import tv.subcult.museum.ui.theme.Museum

@Composable
fun RouteReportsScreen(
    model: AppViewModel, onBack: () -> Unit, onOpenTour: (String) -> Unit,
    tourId: String? = null, revisionId: String? = null, mode: String = "walk",
    cachedRevision: TourRevision? = null, initialTarget: String? = null, onReviewUpdate: (() -> Unit)? = null
) {
    val app by model.state.collectAsStateWithLifecycle()
    val owner = app.session
    val origin = model.container.client.baseUrl
    var showInbox by remember(revisionId, mode) { mutableStateOf(revisionId == null) }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).imePadding().padding(16.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        val back = { if (showInbox && revisionId != null) showInbox = false else onBack() }
        TextButton(onClick = back) { Text("← Back", color = Museum.action) }
        Overline("Private route observations")
        Text(if (showInbox) "Your route reports" else "Suggest a route change", style = MaterialTheme.typography.headlineLarge, color = Museum.text)
        if (owner == null) {
            SignInPrompt("Sign in to report this retained route and read your private decisions. Input returns for the same account on this device.") { model.showSignIn(true) }
        } else key(owner.account.id, owner.token, origin, revisionId, mode, showInbox) {
            if (showInbox) ReportContent(model, owner, null, null, onReports = {}, onOpenTour = onOpenTour, onReviewUpdate = onReviewUpdate)
            else {
                var retained by remember { mutableStateOf(cachedRevision?.takeIf { it.id == revisionId && it.tourId == tourId }) }
                var failure by remember { mutableStateOf<String?>(null) }
                val scope = rememberCoroutineScope()
                val api = remember { model.routeReportApi(owner) }
                suspend fun load() {
                    failure = null
                    try {
                        val revision = cachedRevision ?: api.revision(tourId.orEmpty(), revisionId.orEmpty())
                        if (model.container.session.state.value != owner || model.container.client.baseUrl != origin) return
                        if (revision.id != revisionId || revision.tourId != tourId) { failure = "The selected revision could not be confirmed."; return }
                        retained = revision
                    } catch (e: CancellationException) { throw e }
                    catch (_: Exception) { failure = "Could not read this retained revision. Connect and retry; it will not be replaced with today's route." }
                }
                LaunchedEffect(Unit) { load() }
                val revision = retained
                when {
                    failure != null -> ErrorCard(failure!!) { scope.launch { load() } }
                    revision == null -> LoadingRow("Opening the reported revision…")
                    else -> {
                        val context = RouteReportContext(revision, mode)
                        val unavailable = context.unavailableReason
                        if (unavailable != null) Text(unavailable, color = Museum.danger)
                        else ReportContent(model, owner, context, initialTarget, onReports = { showInbox = true }, onOpenTour = onOpenTour, onReviewUpdate = onReviewUpdate)
                    }
                }
            }
        }
    }
}

@Composable
private fun ReportContent(model: AppViewModel, owner: Session, context: RouteReportContext?, initialTarget: String?, onReports: () -> Unit, onOpenTour: (String) -> Unit, onReviewUpdate: (() -> Unit)?) {
    val flow = remember { model.routeReports(owner, context, initialTarget) }
    val state by flow.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var discard by remember { mutableStateOf(false) }
    DisposableEffect(flow) { onDispose { flow.close() } }
    LaunchedEffect(flow) { if (context == null) flow.loadPage() }
    BackHandler(enabled = context == null && state.selectedId != null) { flow.showList(); scope.launch { flow.loadPage() } }
    state.message?.let { Text(it, color = Museum.muted) }
    if (state.needsSignIn) SignInPrompt("Sign in again. The original input and retry key are kept.") { model.showSignIn(true) }
    if (state.selectedId != null) {
        if (context == null) TextButton(onClick = { flow.showList(); scope.launch { flow.loadPage() } }) { Text("← Your reports") }
        when {
            state.detailLoading -> LoadingRow("Reading private report…")
            state.detailError != null -> ErrorCard(state.detailError!!) { scope.launch { flow.loadDetail(state.selectedId!!) } }
            state.selected != null -> {
                val report = state.selected!!
                Text(suggestionStatus(report.status).label, color = Museum.text, style = MaterialTheme.typography.titleLarge)
                SelectionContainer { Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text(report.submitted.reason.replaceFirstChar { it.uppercase() }, color = Museum.text)
                    Text(report.submitted.details.ifEmpty { "No additional description." }, color = Museum.text)
                    if (report.submitted.better.isNotEmpty()) Text("Suggested improvement: ${report.submitted.better}", color = Museum.text)
                    Text("${report.submitted.mode} · ${report.submitted.affected.kind} · Direction: ${report.submitted.direction} · Duration: ${report.submitted.duration}", color = Museum.muted)
                    Text("Curator reason: ${report.reason.ifEmpty { "No decision recorded yet." }}", color = Museum.text)
                    if (report.submitted.sourceUrl.isNotEmpty()) Text("Submitted source lead: ${report.submitted.sourceUrl}", color = Museum.muted)
                    report.submitted.annotation?.let { Text("Reported location: ${it.lat}, ${it.lng}", color = Museum.muted) }
                    report.submitted.alternative?.let { Text("Unverified alternative: ${it.coordinates.size} entered points.", color = Museum.muted) }
                } }
                state.outcome?.let { outcome ->
                    Text("An approved update is available. A started trip stays on its original revision until you review and choose an update.", color = Museum.muted)
                    OutlinedButton(onClick = { if (onReviewUpdate != null) onReviewUpdate() else onOpenTour(report.submitted.tourId) }) { Text(if (onReviewUpdate != null) "Review route update" else "Open tour to review the update") }
                    Text("In a started trip, open Trip and check for updates before explicitly switching revision.", color = Museum.muted)
                    if (outcome.temporary) Text("Temporary detour · review due ${outcome.reviewOn ?: "date unavailable"}.${if (outcome.reassessmentRequired) " Reassessment required; reopening is not inferred." else ""}", color = Museum.muted)
                }
                state.outcomeError?.let { Text(it, color = Museum.danger) }
                OutlinedButton(onClick = { scope.launch { flow.loadDetail(report.id) } }) { Text("Refresh report status") }
            }
        }
        return
    }
    if (context != null) {
        Text("${context.revision.snapshot?.tour?.title.orEmpty()} · Revision ${context.revision.number} · ${context.mode}. Reporting needs no replacement path. Curators review observations before a route changes.", color = Museum.muted)
        ReportForm(flow, context)
        OutlinedButton(onClick = { discard = true }, enabled = !state.busy) { Text("Discard local draft") }
        TextButton(onClick = onReports) { Text("Your route reports") }
    } else {
        OutlinedButton(onClick = { scope.launch { flow.loadPage() } }, enabled = !state.listLoading) { Text("Refresh reports") }
        when {
            state.listLoading -> LoadingRow("Reading private reports…")
            state.listError != null -> ErrorCard(state.listError!!) { scope.launch { flow.loadPage() } }
            state.items.isEmpty() -> Text("No reports on this page.", color = Museum.muted)
            else -> state.items.forEach { report -> Card(onClick = { scope.launch { flow.loadDetail(report.id) } }) {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(report.submitted.details.ifBlank { report.submitted.reason }, color = Museum.text, maxLines = 3)
                    Text("${suggestionStatus(report.status).label} · ${report.submitted.mode}", color = Museum.muted)
                }
            } }
        }
        if (state.offset > 0) OutlinedButton(onClick = { scope.launch { flow.loadPage(state.offset - 20) } }, enabled = !state.listLoading) { Text("Previous reports") }
        if (state.hasMore) OutlinedButton(onClick = { scope.launch { flow.loadPage(state.offset + 20) } }, enabled = !state.listLoading) { Text("More reports") }
    }
    if (discard) AlertDialog(onDismissRequest = { discard = false }, title = { Text("Discard this local draft?") }, text = { Text("This removes unfinished input and its retry key on this device. It does not cancel a report already saved on the server. Check Your reports before starting another.") }, confirmButton = {
        TextButton(onClick = { flow.discardLocalDraft(); discard = false }) { Text("Discard local draft") }
    }, dismissButton = { TextButton(onClick = { discard = false }) { Text("Cancel") } })
}

@Composable
private fun ReportForm(flow: RouteReports, context: RouteReportContext) {
    val state by flow.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val fields = state.fields
    var optional by remember { mutableStateOf(false) }
    fun edit(change: (ReportFields) -> ReportFields) = flow.edit(change(flow.state.value.fields))
    ReportChoice("Affected stop, entrance or path", fields.target, context.targets.map { it.id to it.label }, state.canEdit) { value -> edit { it.copy(target = value) } }
    ReportChoice("What is the problem?", fields.reason, listOf("blocked" to "Cannot pass", "closed" to "Closure", "unsafe" to "Unsafe crossing or path", "access" to "Entrance or access problem", "surface" to "Surface problem", "improvement" to "A better option", "other" to "Something else"), state.canEdit) { value -> edit { it.copy(reason = value) } }
    ReportField("What did you observe?", fields.details, state.canEdit, true) { value -> edit { it.copy(details = value) } }
    if (fields.reason == "improvement") ReportField("What would be better?", fields.better, state.canEdit, true) { value -> edit { it.copy(better = value) } }
    ReportField("Source link, if available", fields.sourceUrl, state.canEdit) { value -> edit { it.copy(sourceUrl = value) } }
    ReportChoice("Travel direction", fields.direction, listOf("unknown" to "Not sure", "forward" to "Following displayed itinerary", "reverse" to "Opposite displayed itinerary"), state.canEdit) { value -> edit { it.copy(direction = value) } }
    ReportChoice("How long might it last?", fields.duration, listOf("unknown" to "Not sure", "temporary" to "Temporary", "permanent" to "Permanent"), state.canEdit) { value -> edit { it.copy(duration = value) } }
    ReportField("Date observed, if known (YYYY-MM-DD)", fields.observedOn, state.canEdit) { value -> edit { it.copy(observedOn = value) } }
    TextButton(onClick = { optional = !optional }) { Text(if (optional) "Hide optional location/alternative" else "Optional location or local alternative") }
    if (optional) {
        Text("Enter only the problem area. No current location or background journey is attached. Leave empty for a report only.", color = Museum.muted)
        ReportField("Problem latitude", fields.latitude, state.canEdit) { value -> edit { it.copy(latitude = value) } }
        ReportField("Problem longitude", fields.longitude, state.canEdit) { value -> edit { it.copy(longitude = value) } }
        ReportField("Alternative: longitude, latitude pairs, one per line", fields.alternativePoints, state.canEdit, true) { value -> edit { it.copy(alternativePoints = value) } }
        Text("Use 2 to 64 points in travel order, close to the selected area. Path alternatives must connect to the selected segment's endpoints. These remain unverified suggestions.", color = Museum.muted)
    }
    state.fieldErrors.forEach { (field, message) -> Text("$field: $message", color = Museum.danger, modifier = Modifier.semantics { error(message) }) }
    if (state.pending != null) Text("Input is locked while retrying the same observation, revision and key.", color = Museum.muted)
    Button(onClick = { scope.launch { flow.submit()?.let { flow.loadDetail(it.id) } } }, enabled = state.canSubmit, modifier = Modifier.fillMaxWidth(), colors = ButtonDefaults.buttonColors(containerColor = Museum.action, contentColor = Museum.actionText)) { Text(if (state.busy) "Sending…" else if (state.pending != null) "Retry exact report" else "Send private report") }
}

@Composable
private fun ReportField(label: String, value: String, enabled: Boolean, multiline: Boolean = false, onChange: (String) -> Unit) {
    OutlinedTextField(value, onChange, label = { Text(label) }, enabled = enabled, singleLine = !multiline, minLines = if (multiline) 3 else 1, modifier = Modifier.fillMaxWidth())
}

@Composable
private fun ReportChoice(label: String, value: String, options: List<Pair<String, String>>, enabled: Boolean, onChange: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    Column {
        Text(label, color = Museum.text)
        OutlinedButton(onClick = { expanded = true }, enabled = enabled, modifier = Modifier.fillMaxWidth()) { Text(options.firstOrNull { it.first == value }?.second ?: "Choose an option") }
        DropdownMenu(expanded = expanded && enabled, onDismissRequest = { expanded = false }) {
            options.forEach { (key, text) -> DropdownMenuItem(text = { Text(text) }, onClick = { onChange(key); expanded = false }) }
        }
    }
}
