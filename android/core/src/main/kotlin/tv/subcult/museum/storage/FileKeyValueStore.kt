package tv.subcult.museum.storage

import tv.subcult.museum.core.KeyValueStore
import java.io.File
import java.io.IOException

/** One JSON file per key under the app's files directory. */
class FileKeyValueStore(private val dir: File) : KeyValueStore {
    init { dir.mkdirs() }

    private fun file(key: String) = File(dir, key.map { if (it.isLetterOrDigit() || it == '-' || it == '_' || it == '.') it else '_' }.joinToString("") + ".json")

    @Synchronized override fun get(key: String): String? = file(key).takeIf { it.exists() }?.readText()

    @Synchronized override fun put(key: String, value: String) {
        val tmp = File(dir, file(key).name + ".tmp")
        try {
            tmp.writeText(value)
            if (!tmp.renameTo(file(key))) throw IOException("Could not replace the saved file")
        } finally {
            tmp.delete()
        }
    }

    @Synchronized override fun remove(key: String) {
        val target = file(key)
        if (target.exists() && !target.delete()) throw IOException("Could not remove the saved file")
    }
}
