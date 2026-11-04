package sources

import (
	"html"
	"regexp"
	"strings"
)

var (
	titleRe  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	blockEnd = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|br|tr|section|article|blockquote)\s*>|<br\s*/?>`)
	// Go's regexp has no backreferences, so each dropped element gets its own pattern.
	dropTags  = compilePairs([]string{"script", "style", "nav", "header", "footer", "aside", "noscript", "form", "svg"})
	dropNoise = regexp.MustCompile(`(?is)<(?:div|section)\b[^>]*(?:cookie|banner|menu|sidebar|advert)[^>]*>.*?</(?:div|section)>`)
	mainRe    = regexp.MustCompile(`(?is)<main\b[^>]*>(.*?)</main>`)
	articleRe = regexp.MustCompile(`(?is)<article\b[^>]*>(.*?)</article>`)
)

func compilePairs(tags []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(tags))
	for _, t := range tags {
		out = append(out, regexp.MustCompile(`(?is)<`+t+`\b[^>]*>.*?</`+t+`>`))
	}
	return out
}

// ExtractArticle returns the page title and the readable body text of an HTML page.
// It prefers <main> or <article>, drops navigation and scripts, and collapses whitespace.
func ExtractArticle(page string) (title, text string) {
	if m := titleRe.FindStringSubmatch(page); m != nil {
		title = strings.TrimSpace(stripHTML(m[1]))
	}
	body := page
	for _, re := range dropTags {
		body = re.ReplaceAllString(body, " ")
	}
	body = dropNoise.ReplaceAllString(body, " ")
	if m := mainRe.FindStringSubmatch(body); m != nil {
		body = m[1]
	} else if m := articleRe.FindStringSubmatch(body); m != nil {
		body = m[1]
	}
	body = blockEnd.ReplaceAllString(body, "\n")
	body = tagRe.ReplaceAllString(body, " ")
	body = html.UnescapeString(body)
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(spaceRe.ReplaceAllString(line, " "))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return title, strings.Join(lines, "\n")
}
