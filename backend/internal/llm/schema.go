package llm

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

// MomentDraftSchema is the closed JSON schema both providers constrain drafts to.
func MomentDraftSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":          str("Short title for the moment, under 60 characters."),
			"body":           str("120 to 220 words of plain prose for a visitor standing at the place. Only facts supported by the source."),
			"date_start":     str("YYYY-MM-DD when known, else empty string."),
			"date_end":       str("YYYY-MM-DD when the moment spans a range, else empty string."),
			"date_precision": map[string]any{"type": "string", "enum": []string{"day", "month", "year", "circa"}},
			"date_display":   str("Human date as a visitor should read it, e.g. 'December 25, 1928' or 'Summer 1923'."),
			"topic_slugs":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Only slugs from the offered topic list."},
			"notes":          str("Anything the source does not support, uncertainties, and what a curator should verify. Empty if none."),
		},
		"required":             []string{"title", "body", "date_start", "date_end", "date_precision", "date_display", "topic_slugs", "notes"},
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
