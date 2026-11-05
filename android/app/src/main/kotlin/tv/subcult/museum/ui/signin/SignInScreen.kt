package tv.subcult.museum.ui.signin

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.credentials.CredentialManager
import androidx.credentials.GetCredentialRequest
import com.google.android.libraries.identity.googleid.GetGoogleIdOption
import com.google.android.libraries.identity.googleid.GoogleIdTokenCredential
import kotlinx.coroutines.launch
import tv.subcult.museum.BuildConfig
import tv.subcult.museum.core.ApiException
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.components.Card
import tv.subcult.museum.ui.components.Overline
import tv.subcult.museum.ui.theme.Museum

/** Google through Credential Manager, plus a developer email sign-in in debug builds. */
@Composable
fun SignInScreen(model: AppViewModel) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var error by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var devEmail by remember { mutableStateOf("") }
    var devName by remember { mutableStateOf("") }

    fun fail(e: Throwable) {
        error = (e as? ApiException)?.message ?: e.message ?: "Sign-in failed."
        busy = false
    }

    Column(Modifier.padding(horizontal = 18.dp, vertical = 12.dp).padding(bottom = 32.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Overline("Sign in")
        Text("Keep your routes and progress", style = MaterialTheme.typography.headlineMedium, color = Museum.text)
        Text("Sign in to build and share your own routes, save tours, and pick up where you left off on any device.", color = Museum.muted, style = MaterialTheme.typography.bodyMedium)

        if (BuildConfig.GOOGLE_SERVER_CLIENT_ID.isNotEmpty()) {
            Button(
                onClick = {
                    busy = true
                    scope.launch {
                        try {
                            val option = GetGoogleIdOption.Builder().setServerClientId(BuildConfig.GOOGLE_SERVER_CLIENT_ID).setFilterByAuthorizedAccounts(false).build()
                            val result = CredentialManager.create(context).getCredential(context, GetCredentialRequest.Builder().addCredentialOption(option).build())
                            val google = GoogleIdTokenCredential.createFrom(result.credential.data)
                            model.signIn(model.api.signInGoogle(google.idToken, google.displayName ?: ""))
                            busy = false
                        } catch (e: Exception) {
                            fail(e)
                        }
                    }
                },
                enabled = !busy,
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color.Black)
            ) { Text("Continue with Google") }
        }

        if (BuildConfig.DEBUG) {
            Card {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Overline("Development sign-in", Museum.purple)
                    OutlinedTextField(value = devEmail, onValueChange = { devEmail = it }, label = { Text("Email") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
                    OutlinedTextField(value = devName, onValueChange = { devName = it }, label = { Text("Name (optional)") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
                    OutlinedButton(
                        onClick = {
                            busy = true
                            scope.launch {
                                try {
                                    model.signIn(model.api.signInDev(devEmail.trim(), devName.ifBlank { devEmail.substringBefore('@') }))
                                    busy = false
                                } catch (e: Exception) {
                                    fail(e)
                                }
                            }
                        },
                        enabled = !busy && devEmail.contains('@')
                    ) { Text("Sign in with email") }
                }
            }
        }
        error?.let { Text(it, color = Museum.danger, style = MaterialTheme.typography.bodySmall) }
    }
}
