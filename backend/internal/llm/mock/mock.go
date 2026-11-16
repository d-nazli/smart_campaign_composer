// Package mock is the LLM used when no API key is configured and in every
// automated test. By default it behaves like a small, deterministic "model":
// it reads the brief with keyword rules and answers in the same JSON the real
// model must produce. For tests it can instead follow a script that returns
// broken output, refusals or API errors so the retry/fallback paths can be
// exercised without a network or a key.
package mock

import (
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/offer"
)

// Kind selects what a scripted step does.
type Kind int

const (
	// Smart answers like a working model would (keyword rules on the brief).
	Smart Kind = iota
	// Raw returns Step.Raw verbatim (valid or deliberately broken).
	Raw
	// MaxTokens returns Step.Raw and reports that the output was cut off.
	MaxTokens
	// Refusal makes the model decline.
	Refusal
	// Error returns Step.Err (a transport/API failure).
	Error
	// Drop streams the first half of Step.Raw (or the smart answer) and then
	// fails, like a connection that breaks mid-stream.
	Drop
)

// Step is one scripted response.
type Step struct {
	Kind Kind
	Raw  string
	Err  error
}

// Call records what the client was asked, so tests can assert on feedback.
type Call struct {
	Kind     string // "segment" or "messages"
	Brief    string
	Previous []llm.Attempt
	Offer    offer.Offer
	// Language is the language asked for (messages only).
	Language string
}

// Client implements llm.Client.
type Client struct {
	delay time.Duration

	mu       sync.Mutex
	segment  []Step
	messages []Step
	segIdx   int
	msgIdx   int
	calls    []Call

	// langMessages scripts the messages of one non-default language.
	langMessages map[string][]Step
	langIdx      map[string]int
}

// New returns the smart mock. delay is the pause between streamed fragments
// (use 0 in tests).
func New(delay time.Duration) *Client { return &Client{delay: delay} }

// NewScripted returns a mock that plays the given steps in order and repeats
// the last one when it runs out. An empty script behaves like Smart.
func NewScripted(segment []Step, messages []Step) *Client {
	return &Client{segment: segment, messages: messages}
}

// WithMessages scripts the answers for one language other than the default
// (whose script is NewScripted's messages), and returns the client.
func (c *Client) WithMessages(lang string, steps ...Step) *Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.langMessages == nil {
		c.langMessages, c.langIdx = map[string][]Step{}, map[string]int{}
	}
	c.langMessages[lang] = steps
	return c
}

func (c *Client) Mode() string  { return "mock" }
func (c *Client) Model() string { return "mock" }

// Calls returns a copy of the recorded calls.
func (c *Client) Calls() []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Call(nil), c.calls...)
}

func next(steps []Step, idx *int) Step {
	if len(steps) == 0 {
		return Step{Kind: Smart}
	}
	s := steps[min(*idx, len(steps)-1)]
	*idx++
	return s
}

func (c *Client) Segment(ctx context.Context, req llm.SegmentRequest) (llm.Output, error) {
	c.mu.Lock()
	c.calls = append(c.calls, Call{Kind: "segment", Brief: req.Brief, Previous: req.Previous})
	step := next(c.segment, &c.segIdx)
	c.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return llm.Output{}, err
	}
	return resolve(step, func() string { return smartSegment(req.Brief) })
}

func (c *Client) Messages(ctx context.Context, req llm.MessageRequest, onDelta func(string)) (llm.Output, error) {
	lang := content.NormalizeLanguage(req.Language)
	c.mu.Lock()
	c.calls = append(c.calls, Call{Kind: "messages", Brief: req.Brief, Previous: req.Previous, Offer: req.Offer, Language: lang})
	// A script describes the reference (default) language, the one a test is
	// about; the other languages are answered like a working model would unless
	// WithMessages scripts them.
	step := Step{Kind: Smart}
	if lang == content.DefaultLanguage {
		step = next(c.messages, &c.msgIdx)
	} else if steps, ok := c.langMessages[lang]; ok {
		idx := c.langIdx[lang]
		step = next(steps, &idx)
		c.langIdx[lang] = idx
	}
	c.mu.Unlock()

	out, err := resolve(step, func() string { return smartMessagesFor(lang, req.Brief, req.Offer) })
	if err != nil {
		return out, err
	}
	raw := out.Raw
	if step.Kind == Drop {
		raw = firstHalf(raw)
	}
	if err := c.stream(ctx, raw, onDelta); err != nil {
		return llm.Output{Raw: raw}, err
	}
	if step.Kind == Drop {
		return llm.Output{Raw: raw}, errors.New("mock: connection dropped mid-stream")
	}
	return out, nil
}

func resolve(step Step, smart func() string) (llm.Output, error) {
	switch step.Kind {
	case Refusal:
		return llm.Output{}, &llm.RefusalError{Category: "mock"}
	case Error:
		if step.Err != nil {
			return llm.Output{}, step.Err
		}
		return llm.Output{}, errors.New("mock: API error")
	case Raw:
		return llm.Output{Raw: step.Raw, StopReason: llm.StopEnd}, nil
	case MaxTokens:
		return llm.Output{Raw: step.Raw, StopReason: llm.StopMaxTokens}, nil
	case Drop:
		raw := step.Raw
		if raw == "" {
			raw = smart()
		}
		return llm.Output{Raw: raw, StopReason: llm.StopEnd}, nil
	default:
		return llm.Output{Raw: smart(), StopReason: llm.StopEnd}, nil
	}
}

// stream emits raw in small fragments, pausing between them.
func (c *Client) stream(ctx context.Context, raw string, onDelta func(string)) error {
	if onDelta == nil {
		return ctx.Err()
	}
	const fragment = 6 // runes
	runes := []rune(raw)
	for i := 0; i < len(runes); i += fragment {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(i+fragment, len(runes))
		onDelta(string(runes[i:end]))
		if c.delay > 0 {
			t := time.NewTimer(c.delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
	}
	return ctx.Err()
}

func firstHalf(s string) string {
	runes := utf8.RuneCountInString(s)
	r := []rune(s)
	return string(r[:runes/2])
}
