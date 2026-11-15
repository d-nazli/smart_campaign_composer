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
	Explanation string // what the audience is, from the segment step
	Policy      content.Policy
	Previous    []Attempt
}

// Client is implemented by the Claude client and by the mock.
type Client interface {
	// Segment asks for a segment rule as JSON text.
	Segment(ctx context.Context, req SegmentRequest) (Output, error)
	// Messages asks for the SMS/email JSON and calls onDelta with raw text
	// fragments as they arrive (it may be nil).
	Messages(ctx context.Context, req MessageRequest, onDelta func(text string)) (Output, error)
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
