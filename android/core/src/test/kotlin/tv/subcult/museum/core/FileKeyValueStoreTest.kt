package tv.subcult.museum.core

import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import tv.subcult.museum.storage.FileKeyValueStore
import java.io.File
import java.io.IOException
import java.nio.file.Path

class FileKeyValueStoreTest {
    @TempDir lateinit var directory: Path

    @Test
    fun replacingAndRemovingADraftUsesTheActualFiles() {
        val store = FileKeyValueStore(directory.toFile())
        store.put("draft", "first")
        store.put("draft", "replacement")
        assertEquals("replacement", store.get("draft"))
        assertFalse(directory.resolve("draft.json.tmp").toFile().exists())
        store.remove("draft")
        store.remove("absent")
        assertNull(store.get("draft"))
    }

    private fun blockReplacement(target: File) {
        if (target.isFile) assertTrue(target.delete())
        assertTrue(target.mkdir())
        target.resolve("retained").writeText("Do not remove this existing entry")
    }

    @Test
    fun failedRenameThrowsAndCleansItsTemporaryFile() {
        val store = FileKeyValueStore(directory.toFile())
        val target = directory.resolve("draft.json").toFile()
        blockReplacement(target)
        assertThrows(IOException::class.java) { store.put("draft", "must not report this as saved") }
        assertFalse(directory.resolve("draft.json.tmp").toFile().exists())
        assertEquals("Do not remove this existing entry", target.resolve("retained").readText())
    }

    @Test
    fun failedRemovalThrowsInsteadOfReportingADiscard() {
        val store = FileKeyValueStore(directory.toFile())
        blockReplacement(directory.resolve("draft.json").toFile())
        assertThrows(IOException::class.java) { store.remove("draft") }
    }

    @Test
    fun failedTripRecordRemovalKeepsTheTripOpenWithAnError() = runTest {
        val store = FileKeyValueStore(directory.toFile())
        val tour: TourDetail = Fixtures.decode("tour")
        val active = ActiveTour(Endpoints(ApiClient("https://unused.test") { null }), tour, "walk", store, { false }, this)
        blockReplacement(directory.resolve("active.json").toFile())
        assertFalse(active.end())
        assertNotNull(active.state.value.nextStop)
        assertTrue(active.state.value.syncError?.contains("could not be closed") == true)
    }

    @Test
    fun suggestionDoesNotPostWhenItsRealFileCannotBeReplaced() = runTest {
        MockWebServer().use { server ->
            server.start()
            val fixture: RouteSuggestion = Fixtures.decode("route_suggestion")
            val store = FileKeyValueStore(directory.toFile())
            val flow = RouteSuggestions(Endpoints(ApiClient(server.url("/").toString()) { "fixture-token" }), store, fixture.ownerAccountId) { true }
            flow.edit(SuggestionFields("A question", "A region"))
            server.enqueue(MockResponse().setBody("""{"enabled":true}"""))
            flow.loadIntake()
            blockReplacement(directory.toFile().listFiles()!!.single { it.name.endsWith(".json") })
            server.enqueue(MockResponse().setResponseCode(201).setBody(Fixtures.text("route_suggestion")))
            assertNull(flow.submit())
            assertEquals(1, server.requestCount, "Only the intake GET may reach the server")
            assertTrue(flow.state.value.message?.contains("No submission was sent") == true)
            flow.discardLocalDraft()
            assertEquals("A question", flow.state.value.fields.topic)
            assertTrue(flow.state.value.message?.contains("Could not remove") == true)
        }
    }
}
