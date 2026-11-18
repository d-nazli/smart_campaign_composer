// Package llm defines how the application talks to a language model.
//
// A Client performs ONE attempt and returns raw text. Everything that makes the
// output trustworthy (parsing, validation, retrying with feedback) lives
// outside the client, in Repair and the domain validators, so it is identical
// for the real Claude client and the mock used in tests.
package llm

import (
	"context"
	"errors"
	"fmt"

	"smartcampaign/internal/content"
	"smartcampaign/internal/offer"
)

// Stop reasons, normalized across clients.
const (
	StopEnd       = "end_turn"
	StopMaxTokens = "max_tokens"
)

type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Output is the result of one attempt.
type Output struct {
	Raw        string
	StopReason string
	Usage      Usage
}

// Attempt is a previous, rejected output plus why it was rejected. It is sent
// back to the model so it can correct itself.
type Attempt struct {
	Raw      string
	Problems []string
}

type SegmentRequest struct {
	Brief string
	// Previous holds the last rejected attempt (empty on the first try).
	Previous []Attempt
}

type MessageRequest struct {
	Brief       string
	Explanation string      // what the audience is, from the segment step
	Offer       offer.Offer // the incentive the messages must state
	Policy      content.Policy
	Previous    []Attempt
	// Locale is the source locale the campaign is written in first; every other
	// locale is translated from that text (TranslateRequest).
	Locale content.Locale
	// Theme is what the copy is about and how it should sound, read from the
	// brief by code (brief.ThemeOf). Empty fields mean "nothing special".
	Theme Theme
	// Variant is the A/B variant being written ("" or "A", or "B"). Avoid is
	// the other variant's text when there is one (Variant B is written with
	// Variant A's text in view): the new text must say the same offer in
	// different words; code rejects a copy of it.
	Variant string
	Avoid   *content.Content
}

// TranslateRequest asks for the source text in one target locale.
type TranslateRequest struct {
	Source       content.Content
	SourceLocale content.Locale
	Target       content.Locale
	Policy       content.Policy
	Previous     []Attempt
	// The campaign the text belongs to: context for the translator (the mock
	// writes from it with its phrase books).
	Brief string
	Offer offer.Offer
	Theme Theme
	// Variant is the A/B variant the source text belongs to ("" or "A", or "B").
	Variant string
}

// Theme steers the copy. It never changes who receives the campaign.
type Theme struct {
	// Occasion is a special day (brief.Occasion* ids), OccasionName its name in
	// the text being written ("Kadınlar Günü", "Women's Day").
	Occasion, OccasionName string
	// Topic is what the campaign is about: cart, winback, welcome, loyal, general.
	Topic string
	// Tones the brief asks for: friendly, formal, excited, short, elegant, urgent.
	Tones []string
}

// Topics and tones of a Theme.
const (
	TopicCart    = "cart"
	TopicWinback = "winback"
	TopicWelcome = "welcome"
	TopicLoyal   = "loyal"
	TopicGeneral = "general"

	ToneFriendly = "friendly"
	ToneFormal   = "formal"
	ToneExcited  = "excited"
	ToneShort    = "short"
	ToneElegant  = "elegant"
	ToneUrgent   = "urgent"
)

// Has reports whether the theme asks for a tone.
func (t Theme) Has(tone string) bool {
	for _, x := range t.Tones {
		if x == tone {
			return true
		}
	}
	return false
}

// Client is implemented by the Claude client and by the mock.
type Client interface {
	// Segment asks for a segment rule as JSON text.
	Segment(ctx context.Context, req SegmentRequest) (Output, error)
	// Messages asks for the SMS/email JSON and calls onDelta with raw text
	// fragments as they arrive (it may be nil).
	Messages(ctx context.Context, req MessageRequest, onDelta func(text string)) (Output, error)
	// Translate asks for the same JSON in another locale, streamed the same way.
	Translate(ctx context.Context, req TranslateRequest, onDelta func(text string)) (Output, error)
	// Mode is "mock" or "claude"; Model is the model name (informational).
	Mode() string
	Model() string
}

// RefusalError means the model declined the request. It is not retried.
type RefusalError struct{ Category string }

func (e *RefusalError) Error() string {
	if e.Category == "" {
		return "the model declined the request"
	}
	return fmt.Sprintf("the model declined the request (category: %s)", e.Category)
}

// AuthError means the credentials are wrong or missing. It is not retried.
type AuthError struct{ Err error }

func (e *AuthError) Error() string { return "LLM authentication failed: " + e.Err.Error() }
func (e *AuthError) Unwrap() error { return e.Err }

// IsFatal reports errors that retrying cannot fix.
func IsFatal(err error) bool {
	var r *RefusalError
	var a *AuthError
	return errors.As(err, &r) || errors.As(err, &a) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
