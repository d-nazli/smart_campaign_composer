package llm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// AttemptLog describes one attempt, for the audit table and error details.
type AttemptLog struct {
	N        int
	Raw      string
	Valid    bool
	Problems []string
	Usage    Usage
	Latency  time.Duration
	Err      error
}

// Hooks observe the loop. All are optional.
type Hooks struct {
	OnAttempt func(n, max int)
	OnRetry   func(next int, problems []string)
	OnLog     func(AttemptLog)
}

// ExhaustedError is returned when no attempt produced a valid result.
type ExhaustedError struct {
	Attempts []AttemptLog
}

func (e *ExhaustedError) Error() string {
	return fmt.Sprintf("no valid output after %d attempts", len(e.Attempts))
}

// AllFailedWithErrors is true when every attempt failed with a transport/API
// error (the model never produced output we could judge).
func (e *ExhaustedError) AllFailedWithErrors() bool {
	for _, a := range e.Attempts {
		if a.Err == nil {
			return false
		}
	}
	return len(e.Attempts) > 0
}

// Repair is the retry-with-feedback loop used for every LLM call.
//
// For up to maxAttempts times it calls the model, then validates the raw text.
// If validation fails, the rejected text and the list of problems are passed to
// the next call so the model can fix them. Rules:
//   - invalid output (bad JSON, rule violations, too long, cut off by
//     max_tokens) is retried with feedback;
//   - transport/API errors are retried without feedback (SDK-level retries for
//     429/5xx already happened inside the call);
//   - refusals, authentication errors and context cancellation stop the loop
//     immediately and are returned as-is;
//   - when attempts run out, an *ExhaustedError carries what happened.
func Repair[T any](
	ctx context.Context,
	maxAttempts int,
	call func(ctx context.Context, previous []Attempt, n int) (Output, error),
	validate func(raw string) (T, []string),
	h Hooks,
) (T, error) {
	var zero T
	var logs []AttemptLog
	var previous []Attempt

	for n := 1; n <= maxAttempts; n++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if h.OnAttempt != nil {
			h.OnAttempt(n, maxAttempts)
		}

		started := time.Now()
		out, err := call(ctx, previous, n)
		entry := AttemptLog{N: n, Raw: out.Raw, Usage: out.Usage, Latency: time.Since(started), Err: err}

		var problems []string
		switch {
		case err != nil:
			if h.OnLog != nil {
				h.OnLog(entry)
			}
			logs = append(logs, entry)
			if IsFatal(err) {
				return zero, err
			}
			problems = []string{"model bağlantı/servis hatası: " + err.Error()}
			if n < maxAttempts && h.OnRetry != nil {
				h.OnRetry(n+1, problems)
			}
			continue // no feedback: there is nothing to correct
		case out.StopReason == StopMaxTokens:
			problems = []string{"çıktı uzunluk sınırında kesildi (max_tokens); daha kısa ve tam bir JSON döndür"}
		default:
			var value T
			value, problems = validate(out.Raw)
			if len(problems) == 0 {
				entry.Valid = true
				if h.OnLog != nil {
					h.OnLog(entry)
				}
				return value, nil
			}
		}

		entry.Problems = problems
		if h.OnLog != nil {
			h.OnLog(entry)
		}
		logs = append(logs, entry)
		previous = []Attempt{{Raw: out.Raw, Problems: problems}}
		if n < maxAttempts && h.OnRetry != nil {
			h.OnRetry(n+1, problems)
		}
	}
	return zero, &ExhaustedError{Attempts: logs}
}

// IsExhausted reports whether err is an *ExhaustedError.
func IsExhausted(err error) (*ExhaustedError, bool) {
	var e *ExhaustedError
	return e, errors.As(err, &e)
}
