package tv.subcult.museum.core

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.encodeToString
import kotlinx.serialization.serializer
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException

/** Error for any non-2xx response, carrying the API's error envelope. */
class ApiException(
    val status: Int,
    val code: String,
    override val message: String,
    val fields: Map<String, String> = emptyMap()
) : Exception(message) {
    val isUnauthorized: Boolean get() = status == 401
    val isNotFound: Boolean get() = status == 404
}

/**
 * Minimal OkHttp wrapper: base URL, bearer token, JSON encode and decode, error envelope.
 * `onUnauthorized` receives the token the rejected request carried so the app can ignore a
 * stale 401 that lands after a fresh sign-in.
 */
class ApiClient(
    @Volatile var baseUrl: String,
    private val http: OkHttpClient = OkHttpClient(),
    private val tokenProvider: () -> String?
) {
    @Volatile
    var onUnauthorized: ((String?) -> Unit)? = null

    suspend inline fun <reified T> get(path: String, query: Map<String, String> = emptyMap()): T =
        MuseumJson.decodeFromString(serializer(), request("GET", path, query, null))

    suspend inline fun <reified T, reified B> post(path: String, body: B?): T =
        MuseumJson.decodeFromString(serializer(), request("POST", path, emptyMap(), body?.let { MuseumJson.encodeToString(it) }))

    suspend inline fun <reified T, reified B> put(path: String, body: B?): T =
        MuseumJson.decodeFromString(serializer(), request("PUT", path, emptyMap(), body?.let { MuseumJson.encodeToString(it) }))

    suspend inline fun <reified T, reified B> patch(path: String, body: B?): T =
        MuseumJson.decodeFromString(serializer(), request("PATCH", path, emptyMap(), body?.let { MuseumJson.encodeToString(it) }))

    /** A request whose success response has no body (204). */
    suspend fun send(method: String, path: String, json: String? = null) {
        request(method, path, emptyMap(), json)
    }

    suspend fun delete(path: String) = send("DELETE", path)

    /** Performs the request and returns the body text, or "{}" for an empty success body. */
    suspend fun request(method: String, path: String, query: Map<String, String>, json: String?): String =
        withContext(Dispatchers.IO) {
            val url = (baseUrl.trimEnd('/') + path).toHttpUrl().newBuilder().apply {
                query.forEach { (k, v) -> addQueryParameter(k, v) }
            }.build()
            val token = tokenProvider()
            val builder = Request.Builder().url(url).header("Accept", "application/json")
            if (token != null) builder.header("Authorization", "Bearer $token")
            val body = json?.toRequestBody(JSON_TYPE)
            builder.method(method, body ?: if (method == "POST" || method == "PUT" || method == "PATCH") "".toRequestBody(JSON_TYPE) else null)
            val response = try {
                http.newCall(builder.build()).execute()
            } catch (e: IOException) {
                throw ApiException(0, "network", e.message ?: "The server could not be reached.")
            }
            response.use { res ->
                val text = res.body?.string() ?: ""
                if (res.code == 401) onUnauthorized?.invoke(token)
                if (res.code !in 200..299) throw parseError(res.code, text)
                if (text.isBlank()) "{}" else text
            }
        }

    companion object {
        val JSON_TYPE = "application/json; charset=utf-8".toMediaType()

        /** Maps the error envelope ({"error": {code, message, fields}}) to an ApiException. */
        fun parseError(status: Int, text: String): ApiException {
            val body = runCatching { MuseumJson.decodeFromString<ApiErrorBody>(text) }.getOrNull()
            return if (body != null) {
                ApiException(status, body.error.code, body.error.message, body.error.fields ?: emptyMap())
            } else {
                ApiException(status, "http_error", "The request failed with status $status.")
            }
        }
    }
}
