package tv.subcult.museum.core

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import java.util.concurrent.TimeUnit

class ContributionApiTest {
    private val http = OkHttpClient.Builder().callTimeout(3, TimeUnit.SECONDS).build()
    private val choice = ContributionChoice(tourId = "40000000-0000-4000-8000-000000000001", revisionId = "40000000-0000-4000-8000-000000000002", mode = "walk", requestedOn = "2026-11-07")
    private val consent = ContributionConsent("60000000-0000-4000-8000-000000000001", "a".repeat(64), "2026-12-07")
    @Test fun `gate and original server withdrawal never carry bearer or capability in URL`() = runTest {
        MockWebServer().use { server -> MockWebServer().use { original ->
            server.start(); original.start()
            server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(ContributionGate(false, false, DeviationDetector.VERSION))))
            original.enqueue(MockResponse().setResponseCode(204))
            val api = ContributionHttpApi(server.url("/").toString(), "synthetic-opening-token", http)
            assertFalse(api.gate().enabled)
            api.withdraw(consent, original.url("/").toString())
            val gate = server.takeRequest(3, TimeUnit.SECONDS)!!
            val withdrawal = original.takeRequest(3, TimeUnit.SECONDS)!!
            assertNull(gate.getHeader("Authorization")); assertNull(withdrawal.getHeader("Authorization"))
            assertEquals("/api/v1/route-contributions/withdraw", withdrawal.path)
            assertEquals(setOf("consent_id", "deletion_handle"), MuseumJson.parseToJsonElement(withdrawal.body.readUtf8()).jsonObject.keys)
        } }
    }
    @Test fun `captured token exact retry keys and minimal payload match ingestion contract`() = runTest {
        MockWebServer().use { server ->
            server.start(); server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(consent)))
            server.enqueue(MockResponse().setBody("{\"id\":\"70000000-0000-4000-8000-000000000001\",\"expires_on\":\"2026-12-07\"}"))
            val api = ContributionHttpApi(server.url("/").toString(), "synthetic-opening-token", http)
            val receipt = api.grant(choice, "original-consent-key")
            val summary = DeviationSummary(revisionId = choice.revisionId, segmentId = "segment", mode = "walk", direction = "forward", observationPeriod = "2026-11-07", sustainedDepartureRejoins = 0)
            api.submit(receipt, summary, "original-summary-key")
            val grant = server.takeRequest(3, TimeUnit.SECONDS)!!; val sent = server.takeRequest(3, TimeUnit.SECONDS)!!
            assertEquals("Bearer synthetic-opening-token", grant.getHeader("Authorization")); assertEquals(grant.getHeader("Authorization"), sent.getHeader("Authorization"))
            assertEquals("original-consent-key", grant.getHeader("Idempotency-Key")); assertEquals("original-summary-key", sent.getHeader("Idempotency-Key"))
            assertEquals(MuseumJson.encodeToString(choice), grant.body.readUtf8())
            val body = MuseumJson.parseToJsonElement(sent.body.readUtf8()).jsonObject
            assertEquals(setOf("consent_id", "summary"), body.keys); assertEquals(10, body.getValue("summary").jsonObject.size)
        }
    }
    @Test fun `invalid capability receipt is never accepted`() = runTest {
        MockWebServer().use { server ->
            server.start(); server.enqueue(MockResponse().setBody(MuseumJson.encodeToString(consent.copy(deletionHandle = "invalid"))))
            val api = ContributionHttpApi(server.url("/").toString(), "synthetic-opening-token", http)
            var failed = false
            try { api.grant(choice, "original-consent-key") } catch (_: IllegalArgumentException) { failed = true }
            assertTrue(failed)
        }
    }
}
