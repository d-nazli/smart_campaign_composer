package content

import (
	"strings"
	"unicode/utf8"
)

// Render fills a validated template for one recipient.
//
// A variable whose value is empty, or longer than its budget, is replaced by
// the variable's Fallback (which always fits the budget). Together with
// Validate this guarantees: the rendered SMS is never longer than SMSMaxRunes
// and the rendered subject never longer than SubjectMaxRunes, for every
// recipient, however long their name is. Newlines inside values become spaces
// so a name can never break a single-line subject.
func (p Policy) Render(template string, r Recipient) string {
	return placeholderRe.ReplaceAllStringFunc(template, func(m string) string {
		v, ok := lookup(placeholderRe.FindStringSubmatch(m)[1])
		if !ok {
			return "" // cannot happen for validated templates
		}
		val := strings.Join(strings.Fields(v.Value(r)), " ")
		if val == "" || utf8.RuneCountInString(val) > p.budgets[v.Name] {
			return v.FallbackFor(NormalizeLanguage(r.Language))
		}
		return val
	})
}

// RenderContent renders all three fields.
func (p Policy) RenderContent(c Content, r Recipient) Content {
	return Content{
		SMSBody:      p.Render(c.SMSBody, r),
		EmailSubject: p.Render(c.EmailSubject, r),
		EmailBody:    p.Render(c.EmailBody, r),
	}
}
