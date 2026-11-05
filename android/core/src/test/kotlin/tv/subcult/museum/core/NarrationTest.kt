package tv.subcult.museum.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class NarrationTest {
    @Test
    fun `underscores inside words are preserved`() = assertEquals("museum_archive_story and history", plainText("museum_archive_story and _history_"))
    @Test
    fun `prefers recorded narration`() {
        assertEquals(NarrationPlan.Stream("https://media.test/n.mp3"), planNarration("https://media.test/n.mp3", "Some body"))
    }

    @Test
    fun `falls back to speech from the plain body`() {
        assertEquals(NarrationPlan.Speech("The Depot It opened in 1864."), planNarration(null, "## The Depot\n\nIt **opened** in [1864](https://x)."))
    }

    @Test
    fun `is none without anything to say`() {
        assertEquals(NarrationPlan.None, planNarration(null, "   "))
    }

    @Test
    fun `strips Markdown`() {
        val md = """
            # Title

            Some *emphasis* and __strong__ words, `code`, an ![image](https://i/x.png) and a [link](https://example.org).

            - a list item
            1. numbered

            > quoted

            ```
            fenced code
            ```
            <br>Trailing   spaces
        """.trimIndent()
        assertEquals("Title Some emphasis and strong words, code, an and a link. a list item numbered quoted Trailing spaces", plainText(md))
    }
}
