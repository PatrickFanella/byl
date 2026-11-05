package tv.subcult.museum.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class SessionStoreTest {
    private fun session(role: String = "visitor"): Session =
        MuseumJson.decodeFromString("""{"token":"tok-1","expires_at":"2026-12-01T00:00:00Z","account":{"id":"a1","provider":"dev","email":"a@b.c","display_name":"A","role":"$role","created_at":"x"}}""")

    @Test
    fun `set persists and restore reloads`() {
        val storage = MemoryTokenStorage()
        val first = SessionStore(storage)
        first.set(session())
        assertTrue(first.isSignedIn)
        val second = SessionStore(storage)
        assertTrue(second.restore())
        assertEquals("tok-1", second.token)
        assertEquals("a@b.c", second.state.value?.account?.email)
    }

    @Test
    fun `clear removes everything`() {
        val storage = MemoryTokenStorage()
        val store = SessionStore(storage)
        store.set(session())
        store.clear()
        assertNull(store.token)
        assertNull(store.state.value)
        assertNull(storage.read(SessionStore.TOKEN_KEY))
        assertFalse(SessionStore(storage).restore())
    }

    @Test
    fun `update account keeps the token`() {
        val store = SessionStore(MemoryTokenStorage())
        store.set(session())
        store.update(Account(id = "a1", provider = "dev", email = "a@b.c", displayName = "A", role = "curator"))
        assertEquals("tok-1", store.token)
        assertEquals("curator", store.state.value?.account?.role)
    }

    @Test
    fun `restore without a token drops a stale account`() {
        val storage = MemoryTokenStorage()
        storage.write(SessionStore.ACCOUNT_KEY, """{"id":"x","provider":"dev","email":"","display_name":"","role":"visitor"}""")
        val store = SessionStore(storage)
        assertFalse(store.restore())
        assertNull(store.state.value)
    }

    @Test
    fun `in memory key value store round trips`() {
        val kv = InMemoryKeyValueStore()
        kv.put("a", "1")
        assertEquals("1", kv.get("a"))
        kv.remove("a")
        assertNull(kv.get("a"))
    }
}
