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

// ResolveSegment turns a brief into a segment rule that is safe to use.
//
//  1. Parts of the brief that cannot be served (age, gender, product
//     category, calendar months ...) or are ambiguous are decided here. The
//     answer is unsupported or needs_clarification and quotes the words
//     responsible. The model is not asked.
//  2. Otherwise the model answers, the answer is parsed strictly, and then it
//     must contain every part the brief asks for (Verify). Missing, changed or
//     invented parts go back to the model as repair feedback.
//  3. If the model never gets it right, the person gets needs_clarification
//     naming what could not be applied. A different audience is never
//     returned silently.
func ResolveSegment(ctx context.Context, client llm.Client, text string, hooks llm.Hooks) (segment.Rule, error) {
	in := Analyze(text)
	if rule, blocked := in.Blocked(); blocked {
		return rule, nil
	}

	var lastCoverage []string // coverage problems of the latest attempt (nil if it failed for another reason)
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
			if problems := Verify(in, r); len(problems) > 0 {
				lastCoverage = problems
				return r, problems
			}
			return r, nil
		},
		hooks,
	)
	if err == nil {
		rule.Offer = rule.Offer.Normalize()
		// The note is decided by code from the brief, never taken from the model.
		rule.Note = ""
		if in.AudienceDefaulted {
			rule.Note = AudienceDefaultedNote
		}
		return rule, nil
	}
	if _, exhausted := llm.IsExhausted(err); exhausted && len(lastCoverage) > 0 {
		return unresolved(lastCoverage), nil
	}
	return segment.Rule{}, err
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
