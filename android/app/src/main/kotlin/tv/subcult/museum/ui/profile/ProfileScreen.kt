package tv.subcult.museum.ui.profile

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.RadioButton
import tv.subcult.museum.ui.theme.Appearance
import tv.subcult.museum.ui.theme.LocalAppearance
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.rememberCoroutineScope
import kotlinx.coroutines.launch
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tv.subcult.museum.BuildConfig
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.theme.Museum

/** Account, audio guide preference, API base URL override, sign out. */
@Composable
fun ProfileScreen(model: AppViewModel) {
    val appearance = LocalAppearance.current
    val app by model.state.collectAsStateWithLifecycle()
    val contributions = remember(app.session, model.container.client.baseUrl) { model.contributionModel() }
    val lifecycle = androidx.lifecycle.compose.LocalLifecycleOwner.current.lifecycle
    val scope = rememberCoroutineScope()
    DisposableEffect(contributions, lifecycle) {
        val observer = androidx.lifecycle.LifecycleEventObserver { _, event ->
            if (event == androidx.lifecycle.Lifecycle.Event.ON_RESUME) { contributions?.setForeground(true); scope.launch { contributions?.refresh() } }
            if (event == androidx.lifecycle.Lifecycle.Event.ON_PAUSE) contributions?.setForeground(false)
        }
        lifecycle.addObserver(observer)
        if (lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.RESUMED)) contributions?.setForeground(true)
        onDispose { lifecycle.removeObserver(observer); contributions?.setForeground(false) }
    }
    LaunchedEffect(contributions) { contributions?.refresh() }
    var baseUrl by remember { mutableStateOf(model.container.resolveBaseUrl()) }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 16.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Overline("Profile")
        Text(app.session?.account?.displayName?.ifEmpty { null } ?: "Visitor", style = MaterialTheme.typography.headlineLarge, color = Museum.text)

        Card {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Overline("Account")
                val account = app.session?.account
                if (account != null) {
                    Text(account.email.ifEmpty { "Signed in with ${account.provider}" }, color = Museum.text, style = MaterialTheme.typography.bodyLarge)
                    Text("${account.provider} · ${account.role}", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                    OutlinedButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { model.signOut() }) { Text("Sign out") }
                } else {
                    Text("Sign in to save tours, keep progress, and build routes. Browsing and walking tours work without an account.", color = Museum.muted, style = MaterialTheme.typography.bodyMedium)
                    Button(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { model.showSignIn(true) }, colors = ButtonDefaults.buttonColors(containerColor = Museum.action, contentColor = Museum.actionText)) { Text("Sign in") }
                }
            }
        }

        Card {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Overline("On the tour")
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Column(Modifier.weight(1f)) {
                        Text("Audio guide", style = MaterialTheme.typography.titleMedium, color = Museum.text)
                        Text("Play each stop's story when you arrive. Recorded narration when there is one, otherwise read aloud.", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                    }
                    Switch(checked = app.audioEnabled, onCheckedChange = { model.setAudioGuide(it) })
                }
                Text(if (model.container.location.hasPermission()) "Location: allowed" else "Location: not allowed (the walk still works with I'm here)", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
            }
        }

        Card {
            Column {
                Text("Appearance", style = MaterialTheme.typography.titleMedium, color = Museum.text)
                Appearance.entries.forEach { mode ->
                    Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                        RadioButton(selected = appearance.value == mode, onClick = { appearance.set(mode) })
                        TextButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { appearance.set(mode) }) { Text(mode.name) }
                    }
                }
            }
        }

        contributions?.let { controller -> Card { ContributionControls(controller, allowOptIn = false) } }
        app.contributionError?.let { Text(it, color = Museum.danger) }

        if (BuildConfig.DEBUG) Card {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Overline("Server", Museum.info)
                OutlinedTextField(value = baseUrl, onValueChange = { baseUrl = it }, label = { Text("API base URL") }, modifier = Modifier.fillMaxWidth(), singleLine = true, supportingText = { Text("Leave empty for ${BuildConfig.API_BASE_URL}. Changing it signs you out.") })
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { model.signOut(); model.container.setBaseUrlOverride(baseUrl); baseUrl = model.container.resolveBaseUrl(); model.loadCities() }) { Text("Apply") }
                    TextButton(shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp), onClick = { model.signOut(); model.container.setBaseUrlOverride(""); baseUrl = model.container.resolveBaseUrl(); model.loadCities() }) { Text("Reset") }
                }
            }
        }

        Card {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Overline("About")
                Text("Byl ${BuildConfig.VERSION_NAME}", style = MaterialTheme.typography.titleMedium, color = Museum.text)
                Text("Stories are drawn from open archives and reviewed by curators; each moment lists its sources and media credits.", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}
