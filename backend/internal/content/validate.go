package content

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Content is one campaign's message templates.
type Content struct {
	SMSBody      string `json:"sms_body"`
	EmailSubject string `json:"email_subject"`
	EmailBody    string `json:"email_body"`
}

// Field names, shared with the JSON schema, the API and the UI.
const (
	FieldSMSBody      = "sms_body"
	FieldEmailSubject = "email_subject"
	FieldEmailBody    = "email_body"
)

// Issue codes.
const (
	CodeRequired          = "required"
	CodeTooLong           = "too_long"
	CodeNewlineInSubject  = "newline_in_subject"
	CodeControlCharacter  = "control_character"
	CodeUnknownVariable   = "unknown_variable"
	CodeMalformedTemplate = "malformed_placeholder"
	CodeOfferMissing      = "offer_missing"
	CodeOfferConflict     = "offer_conflict"
	CodeRepeatedWord      = "repeated_word"
)

// Issue is one validation problem. Message is written in Turkish for the UI and
// is also what the LLM sees as repair feedback.
type Issue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Length  int    `json:"length,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// Report is the outcome of Validate. SMSLength and SubjectLength are the
// effective lengths (variables counted at their budget) for UI counters.
type Report struct {
	OK     bool    `json:"ok"`
	Issues []Issue `json:"issues"`
	// Warnings are quality problems (missing offer, repeated word). They never
	// make OK false: a person may still send what they edited.
	Warnings      []Issue `json:"warnings"`
	SMSLength     int     `json:"sms_length"`
	SubjectLength int     `json:"subject_length"`
}

// Messages returns the issue messages (used as LLM repair feedback).
func (r Report) Messages() []string {
	out := make([]string, 0, len(r.Issues))
	for _, i := range r.Issues {
		out = append(out, i.Message)
	}
	return out
}

// placeholderRe finds {{name}} / {{ name }}; the name may be anything without
// braces or whitespace so that typos are reported as unknown variables.
var placeholderRe = regexp.MustCompile(`\{\{\s*([^{}\s]*)\s*\}\}`)

// Validate checks c against the policy and returns a normalized copy
// (trimmed, \r\n -> \n, {{ first_name }} -> {{first_name}}). When the report is
// not OK the returned Content must not be stored or sent.
func (p Policy) Validate(c Content) (Content, Report) {
	out := Content{
		SMSBody:      strings.TrimSpace(normalizeNewlines(c.SMSBody)),
		EmailSubject: strings.TrimSpace(c.EmailSubject),
		EmailBody:    strings.TrimSpace(normalizeNewlines(c.EmailBody)),
	}
	rep := Report{Issues: []Issue{}, Warnings: []Issue{}}

	out.SMSBody = p.checkText(&rep, FieldSMSBody, out.SMSBody, SMSMaxRunes, "SMS", true)
	out.EmailSubject = p.checkText(&rep, FieldEmailSubject, out.EmailSubject, SubjectMaxRunes, "E-posta konusu", false)
	out.EmailBody = p.checkText(&rep, FieldEmailBody, out.EmailBody, EmailBodyMaxRunes, "E-posta metni", true)

	rep.SMSLength = p.EffectiveLength(out.SMSBody)
	rep.SubjectLength = p.EffectiveLength(out.EmailSubject)
	rep.OK = len(rep.Issues) == 0
	return out, rep
}

func (p Policy) checkText(rep *Report, field, text string, limit int, label string, multiline bool) string {
	add := func(code, msg string, length, lim int) {
		rep.Issues = append(rep.Issues, Issue{Field: field, Code: code, Message: msg, Length: length, Limit: lim})
	}

	if text == "" {
		add(CodeRequired, label+" boş olamaz.", 0, 0)
		return text
	}

	if !multiline && strings.ContainsAny(text, "\n\r\u0085\u2028\u2029") {
		add(CodeNewlineInSubject, label+" tek satır olmalı; satır sonu içeremez.", 0, 0)
	}
	for _, r := range text {
		if r == '\n' || r == '\r' || (r == '\t' && multiline) {
			continue // line breaks are legal in multiline fields; in the subject they were reported above
		}
		if r < 0x20 || r == 0x7f {
			add(CodeControlCharacter, label+" kontrol karakteri içeriyor.", 0, 0)
			break
		}
	}

	// Placeholders: only known variables, no stray braces.
	for _, m := range placeholderRe.FindAllStringSubmatch(text, -1) {
		if _, ok := lookup(m[1]); !ok {
			add(CodeUnknownVariable, fmt.Sprintf("%s: bilinmeyen değişken {{%s}}. Kullanılabilir değişkenler: %s.", label, m[1], p.variableList()), 0, 0)
		}
	}
	if strings.ContainsAny(placeholderRe.ReplaceAllString(text, ""), "{}") {
		add(CodeMalformedTemplate, label+": hatalı süslü parantez. Değişkenler yalnızca {{değişken_adı}} biçiminde yazılmalı.", 0, 0)
	}

	if n := p.EffectiveLength(text); n > limit {
		add(CodeTooLong, fmt.Sprintf("%s en fazla %d karakter olmalı; değişkenler paylarıyla sayıldığında %d karakter (%d fazla).", label, limit, n, n-limit), n, limit)
	}

	// Normalize placeholder spacing for known variables.
	return placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		if _, ok := lookup(name); ok {
			return "{{" + name + "}}"
		}
		return m
	})
}

// EffectiveLength is the length in runes with every known {{variable}} counted
// as its budget. This is the number compared against the 160 / 80 limits.
func (p Policy) EffectiveLength(text string) int {
	total, last := 0, 0
	for _, loc := range placeholderRe.FindAllStringSubmatchIndex(text, -1) {
		total += utf8.RuneCountInString(text[last:loc[0]])
		total += p.budgets[text[loc[2]:loc[3]]] // 0 for unknown names (reported separately)
		last = loc[1]
	}
	return total + utf8.RuneCountInString(text[last:])
}

func (p Policy) variableList() string {
	names := make([]string, 0, len(Variables))
	for _, v := range Variables {
		names = append(names, "{{"+v.Name+"}}")
	}
	return strings.Join(names, ", ")
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
