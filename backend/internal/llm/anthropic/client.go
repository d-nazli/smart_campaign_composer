// Package anthropic is the llm.Client backed by the real Claude API.
//
// Request shape (chosen for the claude-sonnet-5-5 family):
//   - JSON is requested with structured outputs (output_config.format with a
//     JSON Schema), NOT with a forced tool_choice: this model family rejects
//     tool_choice "any"/"tool" with a 400.
//   - No temperature/top_p/top_k (non-default values are rejected) and no
//     explicit thinking config (adaptive thinking is the default); the amount of
//     thinking is controlled with output_config.effort.
//   - Prefill is not used; a retry is a new user turn that shows the rejected
//     answer and the reasons.
//   - The SDK retries 429/5xx/connection errors itself (default 2 retries).
package anthropic

import (
	"context"
	"errors"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/segment"
)

// maxTokens includes adaptive-thinking tokens, so it is generous even though
// the visible JSON is small.
const maxTokens = 8000

// Client implements llm.Client.
type Client struct {
	api   sdk.Client
	model string
}

// New creates a client. Extra options are for tests (e.g. option.WithBaseURL).
func New(apiKey, model string, opts ...option.RequestOption) *Client {
	all := append([]option.RequestOption{option.WithAPIKey(apiKey)}, opts...)
	return &Client{api: sdk.NewClient(all...), model: model}
}

func (c *Client) Mode() string  { return "claude" }
func (c *Client) Model() string { return c.model }

func (c *Client) Segment(ctx context.Context, req llm.SegmentRequest) (llm.Output, error) {
	params := c.params(
		llm.SegmentSystemPrompt(),
		segment.JSONSchema(),
		sdk.OutputConfigEffortLow, // extraction: little reasoning needed
		conversation(llm.SegmentUserPrompt(req.Brief), req.Previous),
	)
	msg, err := c.api.Messages.New(ctx, params)
	if err != nil {
		return llm.Output{}, mapError(err)
	}
	return toOutput(msg)
}

func (c *Client) Messages(ctx context.Context, req llm.MessageRequest, onDelta func(string)) (llm.Output, error) {
	params := c.params(
		llm.MessagesSystemPromptFor(req.Policy, req.Locale),
		content.JSONSchema(),
		sdk.OutputConfigEffortMedium, // copywriting: some reasoning pays off
		conversation(llm.MessagesUserPromptFor(req), req.Previous),
	)
	return c.stream(ctx, params, onDelta)
}

// Translate writes the source text in one target locale (structured output,
// streamed like Messages).
func (c *Client) Translate(ctx context.Context, req llm.TranslateRequest, onDelta func(string)) (llm.Output, error) {
	params := c.params(
		llm.TranslateSystemPrompt(req.Policy, req.SourceLocale, req.Target),
		content.JSONSchema(),
		sdk.OutputConfigEffortLow, // translation with fixed facts: little reasoning needed
		conversation(llm.TranslateUserPrompt(req), req.Previous),
	)
	return c.stream(ctx, params, onDelta)
}

func (c *Client) stream(ctx context.Context, params sdk.MessageNewParams, onDelta func(string)) (llm.Output, error) {
	stream := c.api.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	var msg sdk.Message
	for stream.Next() {
		event := stream.Current()
		if err := msg.Accumulate(event); err != nil {
			return llm.Output{}, err
		}
		if d, ok := event.AsAny().(sdk.ContentBlockDeltaEvent); ok && onDelta != nil {
			if t, ok := d.Delta.AsAny().(sdk.TextDelta); ok {
				onDelta(t.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return llm.Output{}, mapError(err)
	}
	return toOutput(&msg)
}

func (c *Client) params(system string, schema map[string]any, effort sdk.OutputConfigEffort, msgs []sdk.MessageParam) sdk.MessageNewParams {
	return sdk.MessageNewParams{
		Model:     sdk.Model(c.model),
		MaxTokens: maxTokens,
		System:    []sdk.TextBlockParam{{Text: system}},
		Messages:  msgs,
		OutputConfig: sdk.OutputConfigParam{
			Effort: effort,
			Format: sdk.JSONOutputFormatParam{Schema: schema},
		},
	}
}

// conversation is the user turn, followed (on retries) by the rejected answer
// and a user turn explaining what to fix.
func conversation(first string, previous []llm.Attempt) []sdk.MessageParam {
	msgs := []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock(first))}
	for _, p := range previous {
		raw := p.Raw
		if strings.TrimSpace(raw) == "" {
			raw = "{}" // an empty assistant turn is rejected by the API
		}
		msgs = append(msgs,
			sdk.NewAssistantMessage(sdk.NewTextBlock(raw)),
			sdk.NewUserMessage(sdk.NewTextBlock(llm.RepairPrompt(p.Problems))),
		)
	}
	return msgs
}

func toOutput(msg *sdk.Message) (llm.Output, error) {
	if msg.StopReason == sdk.StopReasonRefusal {
		return llm.Output{}, &llm.RefusalError{Category: string(msg.StopDetails.Category)}
	}
	var text strings.Builder
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(sdk.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	out := llm.Output{
		Raw:        text.String(),
		StopReason: llm.StopEnd,
		Usage:      llm.Usage{InputTokens: int(msg.Usage.InputTokens), OutputTokens: int(msg.Usage.OutputTokens)},
	}
	if msg.StopReason == sdk.StopReasonMaxTokens {
		out.StopReason = llm.StopMaxTokens
	}
	return out, nil
}

func mapError(err error) error {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return &llm.AuthError{Err: err}
	}
	return err
}
