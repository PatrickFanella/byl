package tv.subcult.museum.ui.routes

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
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
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.semantics.error
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import tv.subcult.museum.core.RouteSuggestion
import tv.subcult.museum.core.Session
import tv.subcult.museum.core.suggestionStatus
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.EmptyCard
import tv.subcult.museum.ui.components.ErrorCard
import tv.subcult.museum.ui.components.LoadingRow
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.components.SignInPrompt
import tv.subcult.museum.ui.theme.Museum

@Composable
fun RouteSuggestionsScreen(model: AppViewModel, onBack: () -> Unit) {
    val app by model.state.collectAsStateWithLifecycle()
    val owner = app.session
    val scroll = rememberScrollState()
    Column(
        Modifier.fillMaxSize().verticalScroll(scroll).imePadding().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        TextButton(shape = RoundedCornerShape(4.dp), onClick = onBack) { Text("← Your routes", color = Museum.muted) }
        Overline("Private research suggestions")
        Text("Tour suggestions", style = MaterialTheme.typography.headlineLarge, color = Museum.text)
        Text("Public tour ideas go through research and curator review. Personal route planning stays separate.", color = Museum.muted)
        if (owner == null) {
            SignInPrompt("Sign in to suggest a tour and read your private submission status.") { model.showSignIn(true) }
        } else {
            // A new account, credential or API origin gets a separate request owner.
            key(owner.account.id, owner.token, model.container.client.baseUrl) {
                SuggestionContent(model, owner, scroll)
            }
        }
    }
}

@Composable
private fun SuggestionContent(model: AppViewModel, owner: Session, scroll: ScrollState) {
    val flow = remember { model.routeSuggestions(owner) }
    val state by flow.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var optional by rememberSaveable { mutableStateOf(state.fields.hasOptional) }
    DisposableEffect(flow) { onDispose { flow.close() } }
    LaunchedEffect(flow) {
        launch { flow.loadIntake() }
        launch { flow.loadPage() }
    }
    LaunchedEffect(state.fieldErrors, state.pending) {
        if (state.pending != null || state.fieldErrors.keys.any { it !in listOf("topic", "area") }) optional = true
    }
    LaunchedEffect(state.selectedId) { scroll.scrollTo(0) }
    BackHandler(enabled = state.selectedId != null) {
        flow.showList()
        scope.launch { flow.loadPage() }
    }

    if (state.selectedId != null) {
        TextButton(shape = RoundedCornerShape(4.dp), onClick = { flow.showList(); scope.launch { flow.loadPage(0) } }) { Text("← Your submissions", color = Museum.action) }
        state.message?.let { Text(it, color = Museum.muted) }
        when {
            state.detailLoading -> LoadingRow("Loading private status…")
            state.detailError != null -> ErrorCard(state.detailError!!) { scope.launch { flow.loadDetail(state.selectedId!!) } }
            state.selected != null -> {
                SuggestionDetail(state.selected!!)
                OutlinedButton(shape = RoundedCornerShape(4.dp), onClick = { scope.launch { flow.loadDetail(state.selectedId!!) } }) { Text("Refresh status") }
            }
        }
        return
    }

    when {
        state.intakeLoading -> LoadingRow("Checking suggestion availability…")
        state.intakeError != null -> ErrorCard(state.intakeError!!) { scope.launch { flow.loadIntake() } }
        state.intake != true -> Card {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("New suggestions are closed. Earlier submissions remain readable.", color = Museum.text)
                state.message?.let { Text(it, color = Museum.muted) }
                TextButton(shape = RoundedCornerShape(4.dp), onClick = { scope.launch { flow.loadIntake() } }) { Text("Check availability") }
            }
        }
        else -> {
            Text("Suggest a public tour", style = MaterialTheme.typography.titleLarge, color = Museum.text)
            Text("Only topic and area are required. Notes and links stay private as research leads. Submitting does not publish a tour.", color = Museum.muted)
            state.message?.let { Text(it, color = Museum.danger) }
            if (state.needsSignIn) SignInPrompt("Sign in again to continue with your saved draft.") { model.showSignIn(true) }
            val fields = state.fields
            SuggestionField("Topic (required)", fields.topic, "A theme, people, event or question.", state.fieldErrors["topic"], state.canEdit) { flow.edit(flow.state.value.fields.copy(topic = it)) }
            SuggestionField("Area (required)", fields.area, "A neighborhood, town or region; no exact location required.", state.fieldErrors["area"], state.canEdit) { flow.edit(flow.state.value.fields.copy(area = it)) }
            TextButton(shape = RoundedCornerShape(4.dp), onClick = { optional = !optional }) { Text(if (optional) "Hide optional research leads" else "Optional research leads") }
            if (optional) {
                SuggestionField("Research notes", fields.notes, "Unverified questions or context. These notes stay private.", state.fieldErrors["notes"], state.canEdit, true) { flow.edit(flow.state.value.fields.copy(notes = it)) }
                SuggestionField("Place leads", fields.places, "One name or address per line, up to 20. Unknown locations are welcome.", state.fieldErrors["place_leads"], state.canEdit, true) { flow.edit(flow.state.value.fields.copy(places = it)) }
                SuggestionField("Event leads", fields.events, "One event or question per line, up to 20.", state.fieldErrors["event_leads"], state.canEdit, true) { flow.edit(flow.state.value.fields.copy(events = it)) }
                SuggestionField("Source links", fields.sources, "One HTTP or HTTPS link per line, up to 20. Links are leads, not verified citations.", state.fieldErrors["source_links"], state.canEdit, true) { flow.edit(flow.state.value.fields.copy(sources = it)) }
            }
            if (state.pending != null) Text("A submission may already be saved. Retry sends the same suggestion.", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
            Button(
                modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(4.dp),
                enabled = state.canSubmit, onClick = { scope.launch { flow.submit() } },
                colors = ButtonDefaults.buttonColors(containerColor = Museum.action, contentColor = Museum.actionText)
            ) { Text(if (state.busy) "Submitting…" else if (state.pending != null) "Retry same suggestion" else "Submit suggestion") }
            if (state.pending != null && !state.busy) {
                OutlinedButton(modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(4.dp), onClick = flow::discardLocalDraft) { Text("Discard local draft") }
                Text("Discarding the local draft does not remove a suggestion already saved to your account.", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
            } else if (fields.topic.isBlank() || fields.area.isBlank()) {
                Text("Add a topic and area to submit.", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
            }
        }
    }

    Text("Your submissions", style = MaterialTheme.typography.titleLarge, color = Museum.text)
    OutlinedButton(shape = RoundedCornerShape(4.dp), onClick = { scope.launch { flow.loadPage() } }, enabled = !state.listLoading) { Text("Refresh submissions") }
    when {
        state.listLoading -> LoadingRow("Loading private submissions…")
        state.listError != null -> ErrorCard(state.listError!!) { scope.launch { flow.loadPage() } }
        state.items.isEmpty() -> EmptyCard("No suggestions on this page", "Suggestions you send appear here with their private status.")
        else -> state.items.forEach { suggestion ->
            Card(onClick = { scope.launch { flow.loadDetail(suggestion.id) } }) {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(suggestion.submitted.topic, style = MaterialTheme.typography.titleMedium, color = Museum.text)
                    Text(suggestion.submitted.area, color = Museum.muted)
                    Text(suggestionStatus(suggestion.status).label, color = Museum.action)
                }
            }
        }
    }
    if (state.offset > 0 || state.hasMore) {
        Text("Page " + (state.offset / 20 + 1), color = Museum.muted)
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            OutlinedButton(shape = RoundedCornerShape(4.dp), enabled = state.offset > 0 && !state.listLoading, onClick = { scope.launch { flow.loadPage(state.offset - 20) } }) { Text("Previous") }
            OutlinedButton(shape = RoundedCornerShape(4.dp), enabled = state.hasMore && !state.listLoading, onClick = { scope.launch { flow.loadPage(state.offset + 20) } }) { Text("Next") }
        }
    }
}

@Composable
private fun SuggestionField(label: String, value: String, hint: String, fieldError: String?, enabled: Boolean, multiline: Boolean = false, onChange: (String) -> Unit) {
    OutlinedTextField(
        value = value, onValueChange = onChange, enabled = enabled,
        label = { Text(label) }, supportingText = { Text(fieldError ?: hint) },
        isError = fieldError != null, singleLine = !multiline, minLines = if (multiline) 3 else 1,
        modifier = Modifier.fillMaxWidth().semantics { if (fieldError != null) error(fieldError) },
        shape = RoundedCornerShape(4.dp)
    )
}

@Composable
private fun SuggestionDetail(suggestion: RouteSuggestion) {
    val status = suggestionStatus(suggestion.status)
    val input = suggestion.submitted
    val uri = LocalUriHandler.current
    var linkError by remember { mutableStateOf(false) }
    Text(input.topic, style = MaterialTheme.typography.headlineMedium, color = Museum.text)
    Text(input.area, color = Museum.muted)
    Card {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Status: " + status.label, style = MaterialTheme.typography.titleLarge, color = Museum.text)
            Text(status.detail, color = Museum.text)
            if (suggestion.reason.isNotBlank()) {
                Text("Curator update", style = MaterialTheme.typography.titleMedium, color = Museum.text)
                SelectionContainer { Text(suggestion.reason, color = Museum.text) }
            }
            Text("Updated " + suggestion.updatedAt, style = MaterialTheme.typography.bodySmall, color = Museum.muted)
        }
    }
    Text("Private research leads", style = MaterialTheme.typography.titleLarge, color = Museum.text)
    Text("These are leads, not verified history. Only you and authorized curators can read this suggestion. Curators may refine the saved brief.", color = Museum.muted)
    if (input.notes.isNotBlank()) SelectionContainer { Text(input.notes, color = Museum.text) }
    listOf("Place leads" to input.placeLeads, "Event leads" to input.eventLeads, "Source links" to input.sourceLinks).forEach { (label, entries) ->
        if (entries.isNotEmpty()) {
            Text(label, style = MaterialTheme.typography.titleMedium, color = Museum.text)
            entries.forEach { entry ->
                SelectionContainer { Text(entry, color = Museum.text) }
                if (label == "Source links" && (entry.startsWith("https://") || entry.startsWith("http://"))) {
                    TextButton(shape = RoundedCornerShape(4.dp), onClick = { linkError = runCatching { uri.openUri(entry) }.isFailure }) { Text("Open source", color = Museum.action) }
                }
            }
        }
    }
    if (linkError) Text("Could not open the source link. You can copy its address above.", color = Museum.danger)
}
