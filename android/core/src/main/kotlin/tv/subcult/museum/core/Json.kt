package tv.subcult.museum.core

import kotlinx.serialization.json.Json

/** The one JSON configuration: unknown fields ignored, absent optionals omitted. */
val MuseumJson: Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = true
}
