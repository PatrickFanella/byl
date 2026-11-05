package tv.subcult.museum.core

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.encodeToString

/** Where the token and account live; the app provides an encrypted implementation. */
interface TokenStorage {
    fun read(key: String): String?
    fun write(key: String, value: String)
    fun remove(key: String)
}

class MemoryTokenStorage : TokenStorage {
    private val map = HashMap<String, String>()
    override fun read(key: String): String? = map[key]
    override fun write(key: String, value: String) { map[key] = value }
    override fun remove(key: String) { map.remove(key) }
}

/** The signed-in session: token plus cached account, observable through `state`. */
class SessionStore(private val storage: TokenStorage) {
    private val _state = MutableStateFlow<Session?>(null)
    val state: StateFlow<Session?> = _state

    val token: String? get() = _state.value?.token
    val isSignedIn: Boolean get() = _state.value != null

    /** Loads a saved session; a token without an account restores as signed out. */
    @Synchronized
    fun restore(): Boolean {
        val token = storage.read(TOKEN_KEY)
        val accountJson = storage.read(ACCOUNT_KEY)
        if (token == null || accountJson == null) {
            if (token == null) storage.remove(ACCOUNT_KEY)
            _state.value = null
            return false
        }
        val account = runCatching { MuseumJson.decodeFromString<Account>(accountJson) }.getOrNull()
        if (account == null) {
            clear()
            return false
        }
        _state.value = Session(token = token, account = account)
        return true
    }

    @Synchronized
    fun set(session: Session) {
        storage.write(TOKEN_KEY, session.token)
        storage.write(ACCOUNT_KEY, MuseumJson.encodeToString(session.account))
        _state.value = session
    }

    @Synchronized
    fun update(account: Account) {
        val current = _state.value ?: return
        storage.write(ACCOUNT_KEY, MuseumJson.encodeToString(account))
        _state.value = current.copy(account = account)
    }

    @Synchronized
    fun clear() {
        storage.remove(TOKEN_KEY)
        storage.remove(ACCOUNT_KEY)
        _state.value = null
    }

    /**
     * Signs out only when `rejected` is the token still in use: a 401 for a stale token that
     * lands after a fresh sign-in, or for a request that carried no token, changes nothing.
     * Atomic with respect to set(), so the check and the clear cannot straddle a sign-in.
     */
    @Synchronized
    fun clearIfToken(rejected: String?): Boolean {
        val current = _state.value?.token ?: return false
        if (rejected == null || rejected != current) return false
        clear()
        return true
    }

    companion object {
        const val TOKEN_KEY = "session.token"
        const val ACCOUNT_KEY = "session.account"
    }
}
