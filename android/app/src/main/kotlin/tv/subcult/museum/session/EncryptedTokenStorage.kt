package tv.subcult.museum.session

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import tv.subcult.museum.core.TokenStorage

/** Token and account in EncryptedSharedPreferences (the Keychain equivalent). */
class EncryptedTokenStorage(context: Context) : TokenStorage {
    private val prefs = EncryptedSharedPreferences.create(
        context,
        "museum-session",
        MasterKey.Builder(context).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
    )

    override fun read(key: String): String? = prefs.getString(key, null)
    override fun write(key: String, value: String) = prefs.edit().putString(key, value).apply()
    override fun remove(key: String) = prefs.edit().remove(key).apply()
}
