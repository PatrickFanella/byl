package tv.subcult.museum.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import okhttp3.OkHttpClient

@Serializable
data class ContributionGate(val enabled: Boolean, val calibrated: Boolean, @SerialName("detector_version") val detectorVersion: String)
interface ContributionApi {
    suspend fun gate(): ContributionGate
    suspend fun grant(choice: ContributionChoice, key: String): ContributionConsent
    suspend fun submit(consent: ContributionConsent, summary: DeviationSummary, key: String)
    suspend fun withdraw(consent: ContributionConsent, origin: String)
}
/** Captured credentials; optional failures never invoke navigation/session auth hooks. */
class ContributionHttpApi(origin: String, token: String?, private val http: OkHttpClient = OkHttpClient()) : ContributionApi {
    private val client = ApiClient(origin, http) { token }
    private val publicClient = ApiClient(origin, http) { null }
    override suspend fun gate(): ContributionGate = publicClient.get("/api/v1/route-contribution-intake")
    override suspend fun grant(choice: ContributionChoice, key: String): ContributionConsent =
        client.post<ContributionConsent, ContributionChoice>("/api/v1/me/route-contributions/consent", choice, mapOf("Idempotency-Key" to key)).also { require(ContributionStore.validConsent(it, choice)) }
    @Serializable private data class Submission(@SerialName("consent_id") val consentId: String, val summary: DeviationSummary)
    @Serializable private data class Receipt(val id: String, @SerialName("expires_on") val expiresOn: String)
    override suspend fun submit(consent: ContributionConsent, summary: DeviationSummary, key: String) {
        require(ContributionStore.validSummary(summary))
        val receipt: Receipt = client.post("/api/v1/me/route-contributions", Submission(consent.id, summary), mapOf("Idempotency-Key" to key))
        require(ContributionStore.uuid(receipt.id) && day(receipt.expiresOn) != null && receipt.expiresOn <= consent.expiresOn)
    }
    @Serializable private data class Withdrawal(@SerialName("consent_id") val consentId: String, @SerialName("deletion_handle") val deletionHandle: String)
    override suspend fun withdraw(consent: ContributionConsent, origin: String) {
        ApiClient(origin, http) { null }.send("POST", "/api/v1/route-contributions/withdraw", MuseumJson.encodeToString(Withdrawal(consent.id, consent.deletionHandle)))
    }
}
