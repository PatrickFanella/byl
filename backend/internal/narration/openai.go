package narration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// OpenAI calls an OpenAI-compatible speech endpoint.
type OpenAI struct {
	baseURL string
	apiKey  string
	model   string
	voice   string
	client  *http.Client
}

// NewOpenAI builds a narrator for {baseURL}/v1/audio/speech.
func NewOpenAI(baseURL, apiKey, model, voice string, client *http.Client) Narrator {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &OpenAI{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model, voice: voice, client: client}
}

// maxSpeechInput is the speech endpoint's documented input limit, in characters.
const maxSpeechInput = 4096

// Synthesize speaks text in requests within the endpoint's input limit and joins the MP3 parts,
// so long moments are narrated instead of failing every attempt.
func (o *OpenAI) Synthesize(ctx context.Context, text string) (*Audio, error) {
	var out *Audio
	for _, part := range speechChunks(text, maxSpeechInput) {
		a, err := o.synthesize(ctx, part)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = a
			continue
		}
		out.Data = append(out.Data, a.Data...)
	}
	return out, nil
}

// speechChunks splits text into pieces of at most limit runes, preferring paragraph, then
// sentence, then word boundaries.
func speechChunks(text string, limit int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if n := len(out); n > 0 && utf8.RuneCountInString(out[n-1])+2+utf8.RuneCountInString(para) <= limit {
			out[n-1] += "\n\n" + para
			continue
		}
		for utf8.RuneCountInString(para) > limit {
			cut := splitPoint([]rune(para), limit)
			out = append(out, strings.TrimSpace(string([]rune(para)[:cut])))
			para = strings.TrimSpace(string([]rune(para)[cut:]))
		}
		if para != "" {
			out = append(out, para)
		}
	}
	return out
}

// splitPoint returns where to cut runes, at most limit: after the last sentence end, else the last space.
func splitPoint(runes []rune, limit int) int {
	for _, boundary := range []string{". ", "! ", "? ", " "} {
		if i := strings.LastIndex(string(runes[:limit]), boundary); i > 0 {
			return utf8.RuneCountInString(string(runes[:limit])[:i+len(boundary)])
		}
	}
	return limit
}

func (o *OpenAI) synthesize(ctx context.Context, text string) (*Audio, error) {
	body, _ := json.Marshal(map[string]any{"model": o.model, "voice": o.voice, "input": text, "response_format": "mp3"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("narration: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("narration: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("narration: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "audio/mpeg"
	}
	return &Audio{Data: data, MimeType: mime}, nil
}
