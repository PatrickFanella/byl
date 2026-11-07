package llm

import "slices"

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

// MomentDraftSchema is the closed JSON schema both providers constrain drafts to.
func MomentDraftSchema() map[string]any {
	closed := func(props map[string]any) map[string]any {
		required := []string{}
		for k := range props {
			required = append(required, k)
		}
		slices.Sort(required)
		return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	}
	citation := closed(map[string]any{"evidence_id": str("An offered immutable evidence ID."), "passage": str("An offered passage reference."), "quote": str("Exact substring of that passage, at most 300 characters.")})
	citations := map[string]any{"type": "array", "maxItems": 6, "items": citation}
	requiredCitations := map[string]any{"type": "array", "minItems": 1, "maxItems": 6, "items": citation}
	claim := closed(map[string]any{"text": str("One evidence-supported paragraph, without newlines."), "kind": map[string]any{"type": "string", "enum": []string{"fact", "testimony"}}, "attribution": str("Name the testimony's attribution and include it in text; empty for ordinary facts."), "citations": requiredCitations})
	uncertainty := closed(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"conflicting_dates", "research_gap"}}, "text": str("Visible unresolved disagreement or research limitation."), "citations": citations})
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":           str("Short title for the moment, under 60 characters."),
			"body":            str("A sourced historical story, aiming for 300 to 500 words in at least three paragraphs. Only source-supported facts; shorter if evidence is insufficient, with the research gap explained in notes."),
			"date_start":      str("YYYY-MM-DD when known, else empty string."),
			"date_end":        str("YYYY-MM-DD when the moment spans a range, else empty string."),
			"date_precision":  map[string]any{"type": "string", "enum": []string{"day", "month", "year", "circa"}},
			"date_display":    str("Human date as a visitor should read it, e.g. 'December 25, 1928' or 'Summer 1923'."),
			"topic_slugs":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Only slugs from the offered topic list."},
			"claims":          map[string]any{"type": "array", "minItems": 1, "maxItems": 20, "items": claim},
			"title_citations": requiredCitations,
			"date_citations":  citations,
			"uncertainties":   map[string]any{"type": "array", "maxItems": 8, "items": uncertainty},
			"notes":           str("Anything the source does not support, uncertainties, and what a curator should verify. Empty if none."),
		},
		"required":             []string{"title", "body", "date_start", "date_end", "date_precision", "date_display", "topic_slugs", "notes", "claims", "title_citations", "date_citations", "uncertainties"},
		"additionalProperties": false,
	}
}

// RankingSchema is the closed JSON schema for moment rankings.
func RankingSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ordered": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":     str("Moment id exactly as given."),
						"reason": str("One sentence on why it fits the topic, or does not."),
					},
					"required":             []string{"id", "reason"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"ordered"},
		"additionalProperties": false,
	}
}
