package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicDraftUsesStructuredOutput(t *testing.T) {
	var got map[string]any
	var apiKey, version string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			w.WriteHeader(404)
			return
		}
		apiKey = r.Header.Get("x-api-key")
		if apiKey == "" {
			apiKey = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		version = r.Header.Get("anthropic-version")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"end_turn","content":[{"type":"text","text":` + jsonString(goodDraft) + `}],"usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer srv.Close()
	d := NewAnthropic("sk-ant-test", srv.URL, "claude-opus-5-5")
	out, err := d.DraftMoment(context.Background(), draftReq())
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Built for the Talkies" || out.DateStart != "1928-12-25" {
		t.Fatalf("draft %+v", out)
	}
	if apiKey != "sk-ant-test" || version == "" {
		t.Fatalf("headers key=%q version=%q", apiKey, version)
	}
	if got["model"] != "claude-opus-5-5" {
		t.Fatalf("model %v", got["model"])
	}
	oc, _ := got["output_config"].(map[string]any)
	format, _ := oc["format"].(map[string]any)
	if format["type"] != "json_schema" || format["schema"].(map[string]any)["type"] != "object" {
		t.Fatalf("output_config %v", got["output_config"])
	}
	if _, hasThinking := got["thinking"]; hasThinking {
		t.Fatalf("thinking must be left at the model default: %v", got["thinking"])
	}
	if got["system"] == nil {
		t.Fatal("system prompt missing")
	}
}

func TestAnthropicRefusalIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_2","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"refusal","stop_details":{"type":"refusal","category":"general_harms"},"content":[],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	if _, err := NewAnthropic("k", srv.URL, "claude-opus-5-5").DraftMoment(context.Background(), draftReq()); err == nil {
		t.Fatal("refusal must surface as an error")
	}
}

func TestAnthropicRetriesSchemaInvalidDraftOnce(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		draft := `{}`
		if hits > 1 {
			draft = goodDraft
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_retry","type":"message","role":"assistant","model":"test","stop_reason":"end_turn","content":[{"type":"text","text":` + jsonString(draft) + `}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	result, err := NewAnthropic("test-key", srv.URL, "test").DraftMoment(context.Background(), draftReq())
	if err != nil || hits != 2 || result.Title != "Built for the Talkies" {
		t.Fatalf("hits=%d draft=%+v err=%v", hits, result, err)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
