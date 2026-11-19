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
	// ModelDriven: the real model (Claude) understands the brief and turns it
	// into conditions on its own. Code validates only what is technical: the
	// schema, the field/operator/type whitelist, ranges, the business
	// constraints of a rule and of its offer (segment.Parse), and the campaign
	// name. It never reads the brief to second-guess the model (D113).
	ModelDriven
)

// ResolveSegment is ResolveSegmentWith in Strict mode.
func ResolveSegment(ctx context.Context, client llm.Client, text string, hooks llm.Hooks) (segment.Rule, error) {
	return ResolveSegmentWith(ctx, client, text, hooks, Strict)
}

// WithAnswers is the brief followed by the person's answers, each in its own
// paragraph: the text the rule reader (Strict) and the message step read. The
// questions are left out: they are the model's words, not the person's.
func WithAnswers(text string, clarifications []llm.Clarification) string {
	out := strings.TrimSpace(text)
	for _, c := range clarifications {
		if a := strings.TrimSpace(c.Answer); a != "" {
			out += "\n\n" + a
		}
	}
	return out
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
// ModelDriven (real Claude, D113): the brief goes to the model as it is. The
// model alone reads it and decides ok / needs_clarification / unsupported;
// when it cannot tell what the brief means it asks instead of guessing (the
// system prompt says so). Its answer passes only the technical checks of
// segment.Parse and the name check. Technical problems go back as repair
// feedback; if they persist the call fails with the attempts (the screen shows
// them). There is no keyword, regex or suffix check of the brief against the
// rule, and no second model call.
//
// clarifications are the questions asked before and the person's answers.
// ModelDriven gives them to the model next to the brief; Strict reads the
// brief with the answers appended (WithAnswers), as one text.
func ResolveSegmentWith(ctx context.Context, client llm.Client, text string, hooks llm.Hooks, mode Mode, clarifications ...llm.Clarification) (segment.Rule, error) {
	if mode == ModelDriven {
		return resolveModelDriven(ctx, client, llm.SegmentRequest{Brief: text, Clarifications: clarifications}, hooks)
	}
	text = WithAnswers(text, clarifications)
	in := Analyze(text)
	if rule, blocked := in.Blocked(); blocked {
		return rule, nil
	}
	rule, problems, err := ask(ctx, client, llm.SegmentRequest{Brief: text}, hooks, func(r segment.Rule) []string { return Verify(in, r) })
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

func resolveModelDriven(ctx context.Context, client llm.Client, req llm.SegmentRequest, hooks llm.Hooks) (segment.Rule, error) {
	rule, _, err := ask(ctx, client, req, hooks, nil)
	if err != nil {
		return rule, err
	}
	// Warnings were written by the removed brief checks; a model never writes them.
	rule.Suggestion, rule.Warnings = "", nil
	switch rule.Status {
	case segment.StatusOK:
		// The note is the model's own: an assumption it made, for example that the
		// audience is everyone because the brief names none.
	case segment.StatusNeedsClarification:
		rule.Note = ""
		// The prompt fixes this one question word for word; the screen then offers
		// its one-click answer. This reads the model's answer, not the brief.
		if strings.TrimSpace(rule.Question) == WhoQuestion {
			rule.Suggestion = segment.SuggestAllCustomers
		}
	default:
		rule.Note = ""
	}
	return rule, nil
}

// ask runs the ask-parse-check-repair loop. check (nil: none) returns the
// problems of a rule that parsed; they are sent back to the model. When the
// model never passes check, problems is the latest attempt's list (and err is
// nil).
func ask(ctx context.Context, client llm.Client, req llm.SegmentRequest, hooks llm.Hooks, check func(segment.Rule) []string) (segment.Rule, []string, error) {
	var lastCoverage []string // check problems of the latest attempt (nil if it failed for another reason)
	rule, err := llm.Repair(ctx, MaxAttempts,
		func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
			lastCoverage = nil
			req.Previous = prev
			return client.Segment(ctx, req)
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
			// The model must name the campaign (a stored rule may have no name).
			if problems := segment.NameProblems(r.Name); len(problems) > 0 {
				return r, problems
			}
			if check == nil {
				return r, nil
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
