package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func draftReq() DraftRequest {
	return DraftRequest{PlaceName: "Tivoli Theatre", SourceTitle: "Tivoli Theatre (Downers Grove, Illinois)", SourceURL: "https://en.wikipedia.org/wiki/Tivoli", SourcePublisher: "Wikipedia", SourceLicense: "CC-BY-SA-4.0",
		RawText: "The Tivoli opened on December 25, 1928.", Topics: []TopicOption{{Slug: "downtown-after-dark", Name: "Downtown After Dark"}}}
}

const goodDraft = `{"title":"Built for the Talkies","body":"The Tivoli opened on Christmas Day 1928.","date_start":"1928-12-25","date_end":"1928-12-25","date_precision":"day","date_display":"December 25, 1928","topic_slugs":["downtown-after-dark"],"notes":""}`

func openAIServer(t *testing.T, bodies []string) (*httptest.Server, *[]map[string]any, *atomic.Int32) {
	t.Helper()
	var got []map[string]any
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(404)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		req["_auth"] = r.Header.Get("Authorization")
		got = append(got, req)
		i := int(n.Add(1)) - 1
		if i >= len(bodies) {
			i = len(bodies) - 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": bodies[i]}}}})
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &n
}

func TestOpenAIDraftSendsSchemaAndDecodes(t *testing.T) {
	srv, got, _ := openAIServer(t, []string{goodDraft})
	d := NewOpenAI(srv.URL, "sk-test", "gpt-test", srv.Client())
	out, err := d.DraftMoment(context.Background(), draftReq())
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Built for the Talkies" || out.DatePrecision != "day" || len(out.TopicSlugs) != 1 {
		t.Fatalf("draft %+v", out)
	}
	req := (*got)[0]
	if req["model"] != "gpt-test" || req["_auth"] != "Bearer sk-test" {
		t.Fatalf("request %v", req)
	}
	rf := req["response_format"].(map[string]any)
	js := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || js["strict"] != true || js["schema"].(map[string]any)["type"] != "object" {
		t.Fatalf("response_format %v", rf)
	}
	msgs := req["messages"].([]any)
	if msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["role"] != "user" {
		t.Fatalf("messages %v", msgs)
	}
}

func TestOpenAIRetriesOnceOnInvalidJSON(t *testing.T) {
	srv, _, n := openAIServer(t, []string{"not json", goodDraft})
	d := NewOpenAI(srv.URL, "k", "m", srv.Client())
	if _, err := d.DraftMoment(context.Background(), draftReq()); err != nil {
		t.Fatalf("second response was valid: %v", err)
	}
	if n.Load() != 2 {
		t.Fatalf("expected 2 requests, got %d", n.Load())
	}
	srv2, _, n2 := openAIServer(t, []string{"{}", "{}"})
	if _, err := NewOpenAI(srv2.URL, "k", "m", srv2.Client()).DraftMoment(context.Background(), draftReq()); err == nil {
		t.Fatal("two invalid drafts must fail")
	}
	if n2.Load() != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", n2.Load())
	}
}

func TestOpenAIRank(t *testing.T) {
	srv, got, _ := openAIServer(t, []string{`{"ordered":[{"id":"m2","reason":"matches the rail topic"},{"id":"m1","reason":"weaker"}]}`})
	d := NewOpenAI(srv.URL, "k", "m", srv.Client())
	r, err := d.RankMoments(context.Background(), RankRequest{TopicName: "Rail", PlaceName: "Station", Moments: []MomentOption{{ID: "m1", Title: "A"}, {ID: "m2", Title: "B"}}})
	if err != nil || len(r.Ordered) != 2 || r.Ordered[0].ID != "m2" {
		t.Fatalf("%v %+v", err, r)
	}
	if (*got)[0]["response_format"].(map[string]any)["json_schema"].(map[string]any)["name"] != "moment_ranking" {
		t.Fatalf("schema name %v", (*got)[0]["response_format"])
	}
}

func TestNoneIsUnavailable(t *testing.T) {
	if _, err := (None{}).DraftMoment(context.Background(), draftReq()); err != ErrUnavailable {
		t.Fatalf("got %v", err)
	}
}
