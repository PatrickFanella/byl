package tv.subcult.museum.core

object Fixtures {
    fun text(name: String): String =
        checkNotNull(javaClass.getResourceAsStream("/fixtures/$name.json")) { "missing fixture $name" }
            .bufferedReader().readText()

    inline fun <reified T> decode(name: String): T = MuseumJson.decodeFromString(text(name))
}
