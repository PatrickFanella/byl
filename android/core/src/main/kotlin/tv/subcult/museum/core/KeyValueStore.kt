package tv.subcult.museum.core

/** Small JSON persistence (cached tours, routes, progress); the app backs it with files. */
interface KeyValueStore {
    fun get(key: String): String?
    fun put(key: String, value: String)
    fun remove(key: String)
}

class InMemoryKeyValueStore : KeyValueStore {
    private val map = HashMap<String, String>()
    override fun get(key: String): String? = map[key]
    override fun put(key: String, value: String) { map[key] = value }
    override fun remove(key: String) { map.remove(key) }
}

/** Typed helpers over a KeyValueStore. */
inline fun <reified T> KeyValueStore.getJson(key: String): T? =
    get(key)?.let { runCatching { MuseumJson.decodeFromString<T>(it) }.getOrNull() }

inline fun <reified T> KeyValueStore.putJson(key: String, value: T) =
    put(key, MuseumJson.encodeToString(kotlinx.serialization.serializer(), value))
