package content

import (
	"fmt"
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
			return v.FallbackFor(r)
		}
		return val
	})
}

// TooLongPrefix starts the reason stored for a message that was not sent
// because its rendered text breaks a limit (the dashboard groups by it).
const TooLongPrefix = "message too long"

// RenderedProblem measures a rendered message right before it is sent, as the
// last line of defence behind Validate + budgets: if the sender's budgets ever
// differ from the ones the template was validated with, the limit still holds.
// channel is "sms" or "email". It returns "" when the message may be sent,
// otherwise the reason (starting with TooLongPrefix).
func RenderedProblem(channel string, c Content) string {
	switch channel {
	case "sms":
		if n := utf8.RuneCountInString(c.SMSBody); n > SMSMaxRunes {
			return fmt.Sprintf("%s: sms is %d characters after personalization, limit %d", TooLongPrefix, n, SMSMaxRunes)
		}
	case "email":
		if n := utf8.RuneCountInString(c.EmailSubject); n > SubjectMaxRunes {
			return fmt.Sprintf("%s: email subject is %d characters after personalization, limit %d", TooLongPrefix, n, SubjectMaxRunes)
		}
		if strings.ContainsAny(c.EmailSubject, "\n\r\u0085  ") {
			return TooLongPrefix + ": email subject is not a single line after personalization"
		}
		if n := utf8.RuneCountInString(c.EmailBody); n > EmailBodyMaxRunes {
			return fmt.Sprintf("%s: email body is %d characters after personalization, limit %d", TooLongPrefix, n, EmailBodyMaxRunes)
		}
	}
	return ""
}

// RenderContent renders all three fields.
func (p Policy) RenderContent(c Content, r Recipient) Content {
	return Content{
		SMSBody:      p.Render(c.SMSBody, r),
		EmailSubject: p.Render(c.EmailSubject, r),
		EmailBody:    p.Render(c.EmailBody, r),
	}
}
