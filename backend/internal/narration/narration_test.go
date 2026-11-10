package narration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNoneReturnsNothing(t *testing.T) {
	a, err := None{}.Synthesize(context.Background(), "hello")
	if err != nil || a != nil {
		t.Fatalf("none: %v %v", err, a)
	}
}

func TestOpenAIPostsSpeechRequest(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			w.WriteHeader(404)
			return
		}
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3fake-mp3"))
	}))
	defer srv.Close()
	n := NewOpenAI(srv.URL, "sk-test", "gpt-4o-mini-tts", "alloy", srv.Client())
	a, err := n.Synthesize(context.Background(), "Welcome to the station.")
	if err != nil {
		t.Fatal(err)
	}
	if string(a.Data) != "ID3fake-mp3" || a.MimeType != "audio/mpeg" {
		t.Fatalf("audio %+v", a)
	}
	if got["model"] != "gpt-4o-mini-tts" || got["voice"] != "alloy" || got["input"] != "Welcome to the station." || got["response_format"] != "mp3" {
		t.Fatalf("request %v", got)
	}
	if auth != "Bearer sk-test" {
		t.Fatalf("auth %q", auth)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	if _, err := NewOpenAI(bad.URL, "k", "m", "v", bad.Client()).Synthesize(context.Background(), "x"); err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestOpenAISplitsLongTextWithinTheInputLimit(t *testing.T) {
	var inputs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		input, _ := got["input"].(string)
		if utf8.RuneCountInString(input) > maxSpeechInput {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		inputs = append(inputs, input)
		_, _ = w.Write([]byte("part;"))
	}))
	defer srv.Close()
	sentence := "The depot opened beside the new line in 1864. "
	text := "Title\n\n" + strings.Repeat(sentence, 120) + "\n\n" + strings.Repeat("ž", 5000)
	a, err := NewOpenAI(srv.URL, "", "m", "v", srv.Client()).Synthesize(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) < 3 || string(a.Data) != strings.Repeat("part;", len(inputs)) {
		t.Fatalf("expected joined parts, got %d inputs and %q", len(inputs), a.Data)
	}
	if strings.Join(strings.Fields(strings.Join(inputs, " ")), "") != strings.Join(strings.Fields(text), "") {
		t.Fatal("splitting lost or reordered text")
	}
	for _, in := range inputs[:len(inputs)-2] {
		if !strings.HasSuffix(in, ".") && in != "Title" {
			t.Fatalf("a sentence was cut mid-way: %q", in[len(in)-20:])
		}
	}
}
