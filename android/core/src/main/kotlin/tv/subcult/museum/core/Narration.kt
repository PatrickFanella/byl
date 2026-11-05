package tv.subcult.museum.core

/** What the audio guide should do at a stop. */
sealed class NarrationPlan {
    data class Stream(val url: String) : NarrationPlan()
    data class Speech(val text: String) : NarrationPlan()
    data object None : NarrationPlan()
}

/** Recorded narration first, otherwise the body read aloud. */
fun planNarration(narrationUrl: String?, body: String): NarrationPlan {
    if (!narrationUrl.isNullOrBlank()) return NarrationPlan.Stream(narrationUrl)
    val text = plainText(body)
    return if (text.isEmpty()) NarrationPlan.None else NarrationPlan.Speech(text)
}

private val markdownRules: List<Pair<Regex, String>> = listOf(
    Regex("```[\\s\\S]*?```") to " ",
    Regex("!\\[[^\\]]*]\\([^)]*\\)") to " ",
    Regex("\\[([^\\]]*)]\\([^)]*\\)") to "$1",
    Regex("<[^>]+>") to " ",
    Regex("(?m)^\\s{0,3}#{1,6}\\s*") to "",
    Regex("(?m)^\\s*(?:[-*+]|\\d+\\.)\\s+") to "",
    Regex("(?m)^\\s*>\\s?") to "",
    Regex("`([^`]*)`") to "$1",
    Regex("(\\*\\*|__)(.*?)\\1") to "$2",
    Regex("(\\*|_)(.*?)\\1") to "$2"
)

/** Strips the Markdown a narrator should not read aloud and collapses whitespace. */
fun plainText(markdown: String): String {
    var text = markdown
    for ((re, replacement) in markdownRules) text = re.replace(text, replacement)
    return text.split(Regex("\\s+")).filter { it.isNotEmpty() }.joinToString(" ")
}
