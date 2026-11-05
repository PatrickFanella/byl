package tv.subcult.museum.ui.profile

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
    val app by model.state.collectAsStateWithLifecycle()
    var baseUrl by remember { mutableStateOf(model.container.resolveBaseUrl()) }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Overline("Profile")
        Text(app.session?.account?.displayName?.ifEmpty { null } ?: "Visitor", style = MaterialTheme.typography.headlineLarge, color = Museum.text)

        Card {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Overline("Account")
                val account = app.session?.account
                if (account != null) {
                    Text(account.email.ifEmpty { "Signed in with ${account.provider}" }, color = Museum.text, style = MaterialTheme.typography.bodyLarge)
                    Text("${account.provider} · ${account.role}", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
                    OutlinedButton(onClick = { model.signOut() }) { Text("Sign out") }
                } else {
                    Text("Sign in to save tours, keep progress, and build routes. Browsing and walking tours work without an account.", color = Museum.muted, style = MaterialTheme.typography.bodyMedium)
                    Button(onClick = { model.showSignIn(true) }, colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color.Black)) { Text("Sign in") }
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

        if (BuildConfig.DEBUG) Card {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Overline("Server", Museum.purple)
                OutlinedTextField(value = baseUrl, onValueChange = { baseUrl = it }, label = { Text("API base URL") }, modifier = Modifier.fillMaxWidth(), singleLine = true, supportingText = { Text("Leave empty for ${BuildConfig.API_BASE_URL}. Changing it signs you out.") })
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = { model.signOut(); model.container.setBaseUrlOverride(baseUrl); baseUrl = model.container.resolveBaseUrl(); model.loadCities() }) { Text("Apply") }
                    TextButton(onClick = { model.signOut(); model.container.setBaseUrlOverride(""); baseUrl = model.container.resolveBaseUrl(); model.loadCities() }) { Text("Reset") }
                }
            }
        }

        Card {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Overline("About")
                Text("Invisible Museum ${BuildConfig.VERSION_NAME}", style = MaterialTheme.typography.titleMedium, color = Museum.text)
                Text("Stories are drawn from open archives and reviewed by curators; each moment lists its sources and media credits.", color = Museum.muted, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}
