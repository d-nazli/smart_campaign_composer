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
// translated again. Missing and Stale are Variant A's; Variants has every
// variant's lists (A and, with Variant B, B), so the screen knows which
// variant waits for which locale.
type StaleTranslationsError struct {
	Missing  []string
	Stale    []string
	Variants map[string]PendingLocales
}

// PendingLocales is one variant's translations that are missing or stale.
type PendingLocales struct {
	Missing []string `json:"missing"`
	Stale   []string `json:"stale"`
}

func (e *StaleTranslationsError) Error() string {
	if b, ok := e.Variants[VariantB]; ok && len(b.Missing)+len(b.Stale) > 0 {
		return fmt.Sprintf("çeviriler hazır değil: A eksik %v, güncel değil %v; B eksik %v, güncel değil %v", e.Missing, e.Stale, b.Missing, b.Stale)
	}
	return fmt.Sprintf("çeviriler hazır değil: eksik %v, güncel değil %v", e.Missing, e.Stale)
}

// DuplicateVariantError means Variant B would be the same text as Variant A
// (D102): an A/B test of one text measures nothing.
type DuplicateVariantError struct{}

func (e *DuplicateVariantError) Error() string { return "Varyant B, Varyant A ile aynı" }

// ReadOnlyTranslationError means a request tried to write a translation's
// text: only the source locale is edited by a person (D97).
type ReadOnlyTranslationError struct{ Locale, Source string }

func (e *ReadOnlyTranslationError) Error() string {
	return fmt.Sprintf("%s bir çeviridir ve elle düzenlenemez; yalnız %s düzenlenir, çeviriler ondan yeniden çevrilir", e.Locale, e.Source)
}
