package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Anthropic drafts through the Claude API with structured outputs.
type Anthropic struct {
	client anthropic.Client
	model  string
}

// NewAnthropic builds a drafter. baseURL may be empty for the default endpoint.
func NewAnthropic(apiKey, baseURL, model string) Drafter {
	opts := []option.RequestOption{option.WithAPIKey(apiKey), option.WithMaxRetries(2)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if model == "" {
		model = "claude-opus-5-5"
	}
	return &Anthropic{client: anthropic.NewClient(opts...), model: model}
}

func (a *Anthropic) DraftMoment(ctx context.Context, req DraftRequest) (MomentDraft, error) {
	var out MomentDraft
	err := withOneRetry(func() error {
		raw, err := a.complete(ctx, MomentDraftSchema(), draftSystemPrompt, draftUserPrompt(req))
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

func (a *Anthropic) RankMoments(ctx context.Context, req RankRequest) (Ranking, error) {
	var out Ranking
	err := withOneRetry(func() error {
		raw, err := a.complete(ctx, RankingSchema(), rankSystemPrompt, rankUserPrompt(req))
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

// complete sends one request constrained to schema and returns the text block.
// Thinking is left at the model default; depth is the model's choice.
func (a *Anthropic) complete(ctx context.Context, schema map[string]any, system, user string) (string, error) {
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user))},
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
	})
	if err != nil {
		return "", fmt.Errorf("llm: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("llm: model refused (%s)", resp.StopDetails.Category)
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	if text.Len() == 0 {
		return "", fmt.Errorf("llm: response had no text (stop_reason %s)", resp.StopReason)
	}
	return text.String(), nil
}
