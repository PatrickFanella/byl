package llm

import (
	"fmt"
	"strings"
)

const draftSystemPrompt = `You write short museum labels for places in a city, to be read by a visitor standing there on a walking or biking tour.

Rules:
- Write 120 to 220 words of plain prose. No headings, no bullet points, no second person.
- Use only facts present in the provided source text. Do not add dates, names, or claims from memory.
- If the source does not support a date, leave the date fields empty and set date_precision to "circa".
- Give dates with the precision the source supports: "day" only when the source gives the day.
- Put anything uncertain, contested, or unsupported in "notes", never in the body.
- Choose topic_slugs only from the offered list. Choose none if none fit.
- Ignore navigation text, menus, cookie notices, and boilerplate that may be mixed into the source.
- Write for the ear as well as the eye: the text may be read aloud.`

const rankSystemPrompt = `You help a curator choose which moment at a place best fits a tour's topic.
Order every offered moment from best fit to worst, using the ids exactly as given, with one plain sentence of reasoning each. Judge fit by subject matter, not by length or date.`

func draftUserPrompt(r DraftRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Place: %s\n", r.PlaceName)
	if r.PlaceSummary != "" {
		fmt.Fprintf(&b, "Place summary: %s\n", r.PlaceSummary)
	}
	fmt.Fprintf(&b, "\nSource: %s\nPublisher: %s\nURL: %s\nLicense: %s\n", r.SourceTitle, r.SourcePublisher, r.SourceURL, r.SourceLicense)
	b.WriteString("\nOffered topics:\n")
	if len(r.Topics) == 0 {
		b.WriteString("(none)\n")
	}
	for _, t := range r.Topics {
		fmt.Fprintf(&b, "- %s: %s", t.Slug, t.Name)
		if t.Description != "" {
			fmt.Fprintf(&b, " (%s)", t.Description)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nSource text:\n\"\"\"\n")
	b.WriteString(r.RawText)
	b.WriteString("\n\"\"\"\n\nWrite the moment as JSON matching the schema.")
	return b.String()
}

func rankUserPrompt(r RankRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tour: %s\n%s\n\nTopic: %s\n%s\n\nPlace: %s\n\nMoments:\n", r.TourTitle, r.TourSummary, r.TopicName, r.TopicDescription, r.PlaceName)
	for _, m := range r.Moments {
		fmt.Fprintf(&b, "- id: %s | title: %s | date: %s | excerpt: %s\n", m.ID, m.Title, m.DateDisplay, m.Excerpt)
	}
	b.WriteString("\nReturn the ranking as JSON matching the schema.")
	return b.String()
}
