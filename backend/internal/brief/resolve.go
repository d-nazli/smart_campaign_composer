package brief

import (
	"context"
	"errors"
	"strings"

	"smartcampaign/internal/llm"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// MaxAttempts is how many times the model may try before the person is asked.
const MaxAttempts = 3

// Mode says who understands the brief.
type Mode int

const (
	// Strict: the rule reader in this package understands the brief; the model
	// must agree with it. Used with the mock LLM, which IS the rule reader.
	Strict Mode = iota
	// Grounded: the real model understands the brief; code only checks what can
	// be checked objectively (D87).
	Grounded
)

// ResolveSegment is ResolveSegmentWith in Strict mode.
func ResolveSegment(ctx context.Context, client llm.Client, text string, hooks llm.Hooks) (segment.Rule, error) {
	return ResolveSegmentWith(ctx, client, text, hooks, Strict)
}

// ResolveSegmentWith turns a brief into a segment rule that is safe to use.
//
// Strict (mock LLM):
//  1. Parts of the brief that cannot be served (age, product category,
//     calendar months ...) or are ambiguous are decided here. The answer is
//     unsupported or needs_clarification and quotes the words responsible. The
//     model is not asked.
//  2. Otherwise the model answers, the answer is parsed strictly, and then it
//     must contain every part the brief asks for (Verify). Missing, changed or
//     invented parts go back to the model as repair feedback.
//  3. If the model never gets it right, the person gets needs_clarification
//     naming what could not be applied. A different audience is never
//     returned silently.
//
// Grounded (real Claude, D87): the brief goes to the model as it is, and the
// model decides what it means, including ok / needs_clarification /
// unsupported. Its answer must pass the objective checks only: the schema and
// whitelist (segment.Parse, which also checks the offer with Offer.Validate)
// and Ground (every city, number, amount, code and date is in the brief).
// Failures go back as repair feedback; if they persist, the person is asked.
func ResolveSegmentWith(ctx context.Context, client llm.Client, text string, hooks llm.Hooks, mode Mode) (segment.Rule, error) {
	if mode == Grounded {
		return resolveGrounded(ctx, client, text, hooks)
	}
	in := Analyze(text)
	if rule, blocked := in.Blocked(); blocked {
		return rule, nil
	}
	rule, problems, err := ask(ctx, client, text, hooks, func(r segment.Rule) []string { return Verify(in, r) })
	if problems != nil {
		return unresolved(problems), nil
	}
	if err == nil {
		// The note is decided by code from the brief, never taken from the model.
		rule.Note = ""
		if in.AudienceDefaulted {
			rule.Note = AudienceDefaultedNote
		}
	}
	return rule, err
}

func resolveGrounded(ctx context.Context, client llm.Client, text string, hooks llm.Hooks) (segment.Rule, error) {
	// lastUnused belongs to the latest rule that parsed. When the loop gives up
	// on check problems, the latest attempt parsed, so it is that attempt's.
	var lastUnused []UnusedValue
	rule, problems, err := ask(ctx, client, text, hooks, func(r segment.Rule) []string {
		lastUnused = UnusedValues(text, r)
		return append(Ground(text, r), unusedFeedback(lastUnused)...)
	})
	if problems != nil {
		if len(lastUnused) > 0 {
			// The model never placed an amount of the brief: ask the person, with the choices.
			return segment.Rule{Status: segment.StatusNeedsClarification, Conditions: []segment.Condition{},
				Question: unusedQuestion(lastUnused), Offer: offer.Offer{Kind: offer.KindNone}}, nil
		}
		return unresolved(problems), nil
	}
	if err != nil {
		return rule, err
	}
	rule.Note, rule.Suggestion = "", ""
	switch {
	case rule.Status == segment.StatusOK && len(rule.Conditions) == 1 && rule.Conditions[0].Field == "all_customers" && !SaysEveryone(text):
		// The model chose everyone although the brief does not say so (for example
		// for a special day): the audience step says the audience was not given.
		rule.Note = AudienceDefaultedNote
	case rule.Status == segment.StatusNeedsClarification && strings.TrimSpace(rule.Question) == WhoQuestion:
		rule.Suggestion = segment.SuggestAllCustomers
	}
	return rule, nil
}

// ask runs the ask-parse-check-repair loop. check returns the problems of a
// rule that parsed; they are sent back to the model. When the model never
// passes check, problems is the latest attempt's list (and err is nil).
func ask(ctx context.Context, client llm.Client, text string, hooks llm.Hooks, check func(segment.Rule) []string) (segment.Rule, []string, error) {
	var lastCoverage []string // check problems of the latest attempt (nil if it failed for another reason)
	rule, err := llm.Repair(ctx, MaxAttempts,
		func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
			lastCoverage = nil
			return client.Segment(ctx, llm.SegmentRequest{Brief: text, Previous: prev})
		},
		func(raw string) (segment.Rule, []string) {
			lastCoverage = nil
			r, err := segment.Parse(raw)
			if err != nil {
				var pe *segment.ParseError
				if errors.As(err, &pe) {
					return r, pe.Problems
				}
				return r, []string{err.Error()}
			}
			if problems := check(r); len(problems) > 0 {
				lastCoverage = problems
				return r, problems
			}
			return r, nil
		},
		hooks,
	)
	if err == nil {
		rule.Offer = rule.Offer.Normalize()
		return rule, nil, nil
	}
	if _, exhausted := llm.IsExhausted(err); exhausted && len(lastCoverage) > 0 {
		return segment.Rule{}, lastCoverage, nil
	}
	return segment.Rule{}, nil, err
}

// unresolved is the answer when the model repeatedly left something out.
func unresolved(problems []string) segment.Rule {
	if len(problems) > 2 {
		problems = problems[:2]
	}
	q := "Brief'teki bazı ifadeleri kitle kuralına güvenilir biçimde çeviremedim, bu yüzden tahmin etmedim: " +
		strings.Join(problems, " ") + " Brief'i daha net yazıp tekrar deneyin."
	return segment.Rule{
		Status:     segment.StatusNeedsClarification,
		Conditions: []segment.Condition{},
		Question:   clip(q, segment.MaxQuestion),
		Offer:      offer.Offer{Kind: offer.KindNone},
	}
}
