// Package campaign implements the brief -> segment -> message content flow.
package campaign

import (
	"errors"
	"fmt"

	"smartcampaign/internal/content"
)

// Error codes returned to API clients.
const (
	CodeSegmentFailed    = "segment_generation_failed"
	CodeLLMUnavailable   = "llm_unavailable"
	CodeLLMRefused       = "llm_refused"
	CodeLLMMisconfigured = "llm_misconfigured"
)

var ErrNotFound = errors.New("campaign not found")

// InputError is a problem with what the caller sent.
type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }

// ScheduleError is a well-formed send time the rules do not allow: in the past,
// within the next minute, more than 30 days ahead, or after the offer's last day.
// (A time that cannot be parsed is an InputError.)
type ScheduleError struct{ Message string }

func (e *ScheduleError) Error() string { return e.Message }

// StateError means the campaign is not in a state that allows the action.
type StateError struct{ Message string }

func (e *StateError) Error() string { return e.Message }

// GenerationError means the LLM step failed in a way the caller should see.
// Problems lists, per attempt, why the output was rejected (empty for
// transport errors).
type GenerationError struct {
	Code     string
	Message  string
	Attempts [][]string
	Cause    error
}

func (e *GenerationError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func (e *GenerationError) Unwrap() error { return e.Cause }

// InvalidContentError is returned when edited content breaks the rules.
type InvalidContentError struct {
	Report content.Report
}

func (e *InvalidContentError) Error() string { return "content does not satisfy the limits" }

// StaleTranslationsError means the campaign has translations that are missing
// or made from an older source text: it cannot be approved until they are
// translated again (or marked up to date).
type StaleTranslationsError struct {
	Missing []string
	Stale   []string
}

func (e *StaleTranslationsError) Error() string {
	return fmt.Sprintf("çeviriler hazır değil: eksik %v, güncel değil %v", e.Missing, e.Stale)
}

// ReadOnlyTranslationError means a request tried to write a translation's
// text: only the source locale is edited by a person (D97).
type ReadOnlyTranslationError struct{ Locale, Source string }

func (e *ReadOnlyTranslationError) Error() string {
	return fmt.Sprintf("%s bir çeviridir ve elle düzenlenemez; yalnız %s düzenlenir, çeviriler ondan yeniden çevrilir", e.Locale, e.Source)
}
