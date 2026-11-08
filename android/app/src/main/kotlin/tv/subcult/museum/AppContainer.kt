package tv.subcult.museum

import android.content.Context
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import tv.subcult.museum.audio.NarrationPlayer
import tv.subcult.museum.location.LocationSource
import tv.subcult.museum.session.EncryptedTokenStorage
import tv.subcult.museum.storage.FileKeyValueStore
import tv.subcult.museum.core.ApiClient
import tv.subcult.museum.core.Endpoints
import tv.subcult.museum.core.KeyValueStore
import tv.subcult.museum.core.SessionStore

/** Wires the :core pieces to their Android implementations; one per process. */
class AppContainer(context: Context) {
    private val app = context.applicationContext
    private val prefs = app.getSharedPreferences("museum", Context.MODE_PRIVATE)

    val session = SessionStore(EncryptedTokenStorage(app))
    val store: KeyValueStore = FileKeyValueStore(app.filesDir.resolve("museum-cache"))
    val reportStore: KeyValueStore = FileKeyValueStore(app.noBackupFilesDir.resolve("route-reports"))
    val client = ApiClient(resolveBaseUrl()) { session.token }
    val api = Endpoints(client)
    val location = LocationSource(app)
    val narration = NarrationPlayer(app)
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    var audioGuideEnabled: Boolean
        get() = prefs.getBoolean(KEY_AUDIO, false)
        set(value) = prefs.edit().putBoolean(KEY_AUDIO, value).apply()

    var citySlug: String?
        get() = prefs.getString(KEY_CITY, null)
        set(value) = prefs.edit().putString(KEY_CITY, value).apply()

    /** UserDefaults-style override of the API base URL (Profile screen); empty restores the default. */
    fun setBaseUrlOverride(value: String) {
        val trimmed = value.trim()
        if (trimmed.isEmpty()) prefs.edit().remove(KEY_BASE_URL).apply() else prefs.edit().putString(KEY_BASE_URL, trimmed).apply()
        client.baseUrl = resolveBaseUrl()
    }

    fun resolveBaseUrl(): String = if (BuildConfig.DEBUG) prefs.getString(KEY_BASE_URL, null)?.takeIf { it.isNotBlank() } ?: BuildConfig.API_BASE_URL else BuildConfig.API_BASE_URL

    companion object {
        private const val KEY_AUDIO = "audioGuide"
        private const val KEY_CITY = "citySlug"
        private const val KEY_BASE_URL = "apiBaseUrl"
    }
}
