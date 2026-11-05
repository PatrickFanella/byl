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

func (o *OpenAI) Synthesize(ctx context.Context, text string) (*Audio, error) {
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
