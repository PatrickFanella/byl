package llm

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

// OpenAI talks to any OpenAI-compatible chat completions endpoint.
type OpenAI struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewOpenAI builds a drafter for {baseURL}/v1/chat/completions.
func NewOpenAI(baseURL, apiKey, model string, client *http.Client) Drafter {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAI{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model, client: client}
}

func (o *OpenAI) DraftMoment(ctx context.Context, req DraftRequest) (MomentDraft, error) {
	var out MomentDraft
	err := withOneRetry(func() error {
		raw, err := o.complete(ctx, "moment_draft", MomentDraftSchema(), draftSystemPrompt, draftUserPrompt(req))
		if err != nil {
			return err
		}
		var d MomentDraft
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			return fmt.Errorf("llm: decode draft: %w", err)
		}
		if _, err := d.Check(); err != nil {
			return fmt.Errorf("llm: %w", err)
		}
		out = d
		return nil
	})
	return out, err
}

func (o *OpenAI) RankMoments(ctx context.Context, req RankRequest) (Ranking, error) {
	var out Ranking
	err := withOneRetry(func() error {
		raw, err := o.complete(ctx, "moment_ranking", RankingSchema(), rankSystemPrompt, rankUserPrompt(req))
		if err != nil {
			return err
		}
		var r Ranking
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return fmt.Errorf("llm: decode ranking: %w", err)
		}
		out = r
		return nil
	})
	return out, err
}

func (o *OpenAI) complete(ctx context.Context, schemaName string, schema map[string]any, system, user string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": schemaName, "strict": true, "schema": schema},
		},
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("llm: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("llm: unreadable completion: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm: completion had no choices")
	}
	if parsed.Choices[0].Message.Refusal != "" {
		return "", fmt.Errorf("llm: model refused: %s", parsed.Choices[0].Message.Refusal)
	}
	return parsed.Choices[0].Message.Content, nil
}

// withOneRetry runs fn and retries exactly once on error.
func withOneRetry(fn func() error) error {
	if err := fn(); err != nil {
		return fn()
	}
	return nil
}
