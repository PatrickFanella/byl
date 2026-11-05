package llm

import (
	"encoding/json"
	"testing"
)

func TestSchemasAreClosedObjects(t *testing.T) {
	for name, s := range map[string]map[string]any{"draft": MomentDraftSchema(), "ranking": RankingSchema()} {
		if s["type"] != "object" || s["additionalProperties"] != false {
			t.Fatalf("%s schema must be a closed object: %v", name, s)
		}
		if _, err := json.Marshal(s); err != nil {
			t.Fatalf("%s schema not serialisable: %v", name, err)
		}
	}
	props := MomentDraftSchema()["properties"].(map[string]any)
	for _, k := range []string{"title", "body", "date_start", "date_end", "date_precision", "date_display", "topic_slugs", "notes"} {
		if _, ok := props[k]; !ok {
			t.Fatalf("draft schema missing %s", k)
		}
	}
}

func TestValidateDraftDropsUnknownSlugsAndBadPrecision(t *testing.T) {
	d := MomentDraft{Title: "T", Body: "B", DatePrecision: "fortnight", TopicSlugs: []string{"rail", "ghosts"}}
	out, dropped := ValidateDraft(d, []string{"rail", "parks"})
	if len(out.TopicSlugs) != 1 || out.TopicSlugs[0] != "rail" {
		t.Fatalf("slugs %v", out.TopicSlugs)
	}
	if len(dropped) != 1 || dropped[0] != "ghosts" {
		t.Fatalf("dropped %v", dropped)
	}
	if out.DatePrecision != "circa" {
		t.Fatalf("bad precision should fall back to circa, got %q", out.DatePrecision)
	}
	if _, err := out.Check(); err != nil {
		t.Fatalf("valid draft should check: %v", err)
	}
	if _, err := (MomentDraft{Body: "x"}).Check(); err == nil {
		t.Fatal("missing title must fail Check")
	}
	if _, err := (MomentDraft{Title: "t", Body: "x", DateStart: "1923-13-01"}).Check(); err == nil {
		t.Fatal("bad date must fail Check")
	}
}
