package narration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
