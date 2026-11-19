package mock

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
)

// The mock translator. It reads the Turkish text as a sequence of sentences
// from the bank (bank.go) and writes each one in the target language, with the
// numbers, dates, codes and {{placeholders}} of the source text. So:
//
//   - a translation always follows the CURRENT source: a number, a date or a
//     code edited in the Turkish text is edited in every language, a sentence
//     removed from it is removed everywhere, a bank sentence added is added;
//   - no Turkish word ever reaches another language: a word the bank does not
//     know is left out (a number, a date, a code or a {{placeholder}} in it is
//     kept as it is, so the facts survive). If what was left out carried part
//     of the offer, the backend's comparison with the source rejects the
//     translation and reports it, as it would for a real model.
//
// The bank's languages are those of bankLangs; for another language the mock
// refuses to translate (an error, never the Turkish text).

type kind int

const (
	kindOther kind = iota
	kindGreet
	kindHook
	kindOpen
	kindCTA
	kindSign
	kindPart // one part of the offer ("%20 indirim", "ücretsiz kargo")
	kindCode // "Kupon kodu: X"
	kindCap  // "en fazla 300 TL"
	kindMin  // "1000 TL ve üzeri alışverişte"
	kindTerm // "Son kullanım: 31.12.2026"
)

// entry is one compiled bank sentence in one register: a pattern that finds
// its Turkish in a text, and the sentence in every other language.
type entry struct {
	tr    string
	re    *regexp.Regexp
	slots []string          // what each capture group holds: "1", "2", "d", "c", "o" or "p" (a placeholder)
	out   map[string]string // language → template
	kind  kind
	short *entry // the shorter form, for a translation that would not fit
}

var (
	bankOnce    sync.Once
	bankEntries []*entry
	bankByKey   map[string]*entry
)

// slotRe finds the slots of a template.
var slotRe = regexp.MustCompile(`\{\{\s*[A-Za-z_]\w*\s*\}\}|\{[12dcos]\}`)

var slotPatterns = map[string]string{
	"1": `(\d+(?:[.,]\d+)*)`,
	"2": `(\d+(?:[.,]\d+)*)`,
	"d": `(\d{1,2}\.\d{1,2}\.\d{4})`,
	"c": `([\p{L}\p{N}_-]+)`,
	"o": `(.+?)`,
	// The offer's scope, copied as it is written (the mock has no dictionary for
	// free text): everything up to the end of the sentence.
	"s": `([^.!?\n{}]+)`,
	"p": `(\{\{\s*[A-Za-z_]\w*\s*\}\})`,
}

func slotName(s string) string {
	if strings.HasPrefix(s, "{{") {
		return "p"
	}
	return s[1 : len(s)-1]
}

// trimEnd drops the sentence's own end mark: the source text's punctuation is
// copied instead.
func trimEnd(s string) string {
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), ".!,"))
}

// key is how a Turkish sentence is looked up: without its end mark and with
// the first letter in lower case (the source may start a sentence with it).
func key(tr string) string {
	s := trimEnd(tr)
	r, n := utf8.DecodeRuneInString(s)
	return string(trLower(r)) + s[n:]
}

func trLower(r rune) rune {
	switch r {
	case 'I':
		return 'ı'
	case 'İ':
		return 'i'
	}
	return unicode.ToLower(r)
}

func trUpper(r rune) rune {
	switch r {
	case 'i':
		return 'İ'
	case 'ı':
		return 'I'
	}
	return unicode.ToUpper(r)
}

func compile(tr string, out map[string]string, k kind) *entry {
	tr = trimEnd(tr)
	var b strings.Builder
	b.WriteString("^")
	var slots []string
	pos := 0
	literal := func(s string) {
		if s == "" {
			return
		}
		if pos == 0 { // the first letter in either case
			if r, n := utf8.DecodeRuneInString(s); unicode.IsLetter(r) {
				b.WriteString("[" + string(trLower(r)) + string(trUpper(r)) + "]")
				s = s[n:]
			}
		}
		b.WriteString(strings.ReplaceAll(regexp.QuoteMeta(s), " ", `[ \t]+`))
	}
	for _, m := range slotRe.FindAllStringIndex(tr, -1) {
		literal(tr[pos:m[0]])
		name := slotName(tr[m[0]:m[1]])
		slots = append(slots, name)
		b.WriteString(slotPatterns[name])
		pos = m[1]
	}
	literal(tr[pos:])
	return &entry{tr: tr, re: regexp.MustCompile(b.String()), slots: slots, out: out, kind: k}
}

// loadBank compiles every line of the bank in both registers. A Turkish
// sentence that appears twice must have one translation.
func loadBank() {
	bankByKey = map[string]*entry{}
	shorts := map[*entry]string{}
	add := func(l line, k kind, short string) {
		for reg, tr := range l.tr {
			if tr == "" {
				continue
			}
			t := l.t
			if reg == 1 && l.f != nil {
				t = *l.f
			}
			out := map[string]string{}
			for i, lang := range bankLangs {
				out[lang] = trimEnd(t[i])
			}
			e := compile(tr, out, k)
			if old, ok := bankByKey[key(tr)]; ok {
				for lang, s := range out {
					if old.out[lang] != s {
						panic(fmt.Sprintf("mock bank: %q has two %s translations: %q, %q", tr, lang, old.out[lang], s))
					}
				}
				continue
			}
			bankByKey[key(tr)] = e
			bankEntries = append(bankEntries, e)
			if short != "" {
				shorts[e] = short
			}
		}
	}
	k := turkishKit
	for _, l := range k.greetSMS {
		add(l, kindGreet, "")
	}
	for _, l := range k.greetBody {
		add(l, kindGreet, "")
	}
	for _, m := range []struct {
		lines map[string][]line
		kind  kind
	}{{k.hooks, kindHook}, {k.opens, kindOpen}, {k.ctas, kindCTA}} {
		for _, ls := range m.lines {
			for _, l := range ls {
				add(l, m.kind, "")
			}
		}
	}
	for _, l := range k.offerSentence {
		add(l, kindOther, "")
	}
	for _, l := range k.subjectOffer {
		add(l, kindOther, "")
	}
	for _, l := range k.signoff {
		add(l, kindSign, "")
	}
	for _, t := range termLines {
		add(t.l, t.kind, t.short)
	}
	for e, s := range shorts {
		e.short = bankByKey[key(s)]
		if e.short == nil {
			panic("mock bank: unknown short form " + s)
		}
	}
}

func bank() []*entry {
	bankOnce.Do(loadBank)
	return bankEntries
}

// bankHas reports whether the bank can write a language ("en" for en-GB).
func bankHas(lang string) bool {
	for _, l := range bankLangs {
		if l == lang {
			return true
		}
	}
	return false
}

// ---- reading the source ---------------------------------------------------

// tok is one piece of a parsed line: a bank sentence with its slot values, or
// text copied as it is (punctuation, white space, a kept number or code).
type tok struct {
	e    *entry
	vals []string
	subs map[int][]tok // the parsed {o} slots, by group
	text string
}

// isSep is punctuation and white space: copied, never translated.
func isSep(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune(",.:;!?()+\"«»“”¡¿؟،", r)
}

var keptRe = regexp.MustCompile(`^(\{\{\s*[A-Za-z_]\w*\s*\}\}|TL|₺|.*\d.*)$`)

// parseLine reads one line. strict (an {o} slot) accepts only the offer's
// lines and fails on any unknown word; otherwise an unknown word is dropped,
// or kept when it is a fact (it has a digit) or a {{placeholder}}.
func parseLine(s string, strict bool) ([]tok, []string, bool) {
	var toks []tok
	var unknown []string
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if isSep(r) {
			j := i + size
			if unicode.IsSpace(r) {
				for j < len(s) {
					r2, s2 := utf8.DecodeRuneInString(s[j:])
					if !unicode.IsSpace(r2) {
						break
					}
					j += s2
				}
			}
			if strict && !strings.Contains(" \t+()", strings.TrimSpace(s[i:j])) {
				return nil, nil, false
			}
			toks = append(toks, tok{text: s[i:j]})
			i = j
			continue
		}
		if t, n := matchAt(s[i:], strict); n > 0 {
			toks = append(toks, t)
			i += n
			continue
		}
		j := i
		for j < len(s) {
			r2, s2 := utf8.DecodeRuneInString(s[j:])
			if isSep(r2) {
				break
			}
			j += s2
		}
		if strict {
			return nil, nil, false
		}
		if w := s[i:j]; keptRe.MatchString(w) {
			toks = append(toks, tok{text: w})
		} else {
			unknown = append(unknown, w)
		}
		i = j
	}
	return reorderMin(toks), unknown, true
}

var offerKinds = map[kind]bool{kindPart: true, kindCode: true, kindCap: true, kindMin: true}

// matchAt finds the longest bank sentence at the start of s that ends at a
// word boundary and whose {o} slots read as an offer.
func matchAt(s string, strict bool) (tok, int) {
	var best tok
	bestN := 0
	for _, e := range bank() {
		if strict && !offerKinds[e.kind] {
			continue
		}
		m := e.re.FindStringSubmatchIndex(s)
		if m == nil || m[1] <= bestN {
			continue
		}
		if r, _ := utf8.DecodeRuneInString(s[m[1]:]); m[1] < len(s) && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			continue
		}
		t := tok{e: e, vals: make([]string, len(e.slots))}
		ok := true
		for g, name := range e.slots {
			t.vals[g] = s[m[2+2*g]:m[3+2*g]]
			if name == "o" {
				sub, _, valid := parseLine(t.vals[g], true)
				if !valid || !hasOffer(sub) {
					ok = false
					break
				}
				if t.subs == nil {
					t.subs = map[int][]tok{}
				}
				t.subs[g] = sub
			}
		}
		if ok {
			best, bestN = t, m[1]
		}
	}
	return best, bestN
}

func hasOffer(ts []tok) bool {
	for _, t := range ts {
		if t.e != nil && (t.e.kind == kindPart || t.e.kind == kindCode) {
			return true
		}
	}
	return false
}

// reorderMin moves the minimum basket after the offer it belongs to: Turkish
// says "1000 TL ve üzeri alışverişte %20 indirim", every other language of
// the bank "20% off on orders of 1000 TL and over".
func reorderMin(ts []tok) []tok {
	isOffer := func(t tok) bool {
		return t.e != nil && (t.e.kind == kindPart || t.e.kind == kindCode || t.e.kind == kindCap)
	}
	for i := 0; i+2 < len(ts); i++ {
		if ts[i].e == nil || ts[i].e.kind != kindMin || ts[i+1].e != nil || strings.TrimSpace(ts[i+1].text) != "" || !isOffer(ts[i+2]) {
			continue
		}
		end := i + 2
		for j := i + 3; j < len(ts); j++ {
			if ts[j].e != nil {
				if !isOffer(ts[j]) {
					break
				}
				end = j
				continue
			}
			s := strings.TrimSpace(ts[j].text)
			if s == ")" {
				end = j
				continue
			}
			if s != "" && s != "+" && s != "(" {
				break
			}
		}
		moved := append(append([]tok{}, ts[i+2:end+1]...), tok{text: " "}, ts[i])
		ts = append(append(append([]tok{}, ts[:i]...), moved...), ts[end+1:]...)
		i += len(moved) - 1
	}
	return ts
}

// ---- writing the target ---------------------------------------------------

func (e *entry) render(lang string, t tok) string {
	var placeholders []string
	for g, name := range e.slots {
		if name == "p" {
			placeholders = append(placeholders, t.vals[g])
		}
	}
	p := 0
	return slotRe.ReplaceAllStringFunc(e.out[lang], func(s string) string {
		name := slotName(s)
		if name == "p" {
			if p < len(placeholders) {
				p++
				return placeholders[p-1]
			}
			return s
		}
		for g, n := range e.slots {
			if n != name {
				continue
			}
			if name == "o" {
				return renderToks(t.subs[g], lang, false)
			}
			v := t.vals[g]
			if lang == "en" && (name == "1" || name == "2") {
				v = strings.Replace(v, ",", ".", 1) // 12,5 → 12.5
			}
			return v
		}
		return s
	})
}

// renderToks writes a parsed line in lang. A sentence that starts the line or
// follows ".", "!" or "?" starts with a capital letter.
func renderToks(ts []tok, lang string, capitalize bool) string {
	var b strings.Builder
	for _, t := range ts {
		if t.e == nil {
			b.WriteString(t.text)
			continue
		}
		s := t.e.render(lang, t)
		if capitalize && sentenceStart(b.String()) {
			s = upperFirst(s)
		}
		b.WriteString(s)
	}
	return b.String()
}

func sentenceStart(written string) bool {
	w := strings.TrimSpace(written)
	if w == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(w)
	return strings.ContainsRune(".!?؟", r)
}

func upperFirst(s string) string {
	for i, r := range s {
		switch {
		case r == '¡' || r == '¿':
			continue
		case unicode.IsLetter(r):
			return s[:i] + string(unicode.ToUpper(r)) + s[i+utf8.RuneLen(r):]
		}
		return s
	}
	return s
}

var (
	spaceRunRe    = regexp.MustCompile(`[ \t]{2,}`)
	spaceBeforeRe = regexp.MustCompile(`[ \t]+([,.:;!?)؟،])`)
	openParenRe   = regexp.MustCompile(`\([ \t]+`)
	emptyParenRe  = regexp.MustCompile(`\([ \t]*\)`)
	doublePunctRe = regexp.MustCompile(`([,:;،])[ \t]*([,.:;!?؟،])`)
	leadPunctRe   = regexp.MustCompile(`^[\s,.:;!?؟،]+`)
	endRunRe      = regexp.MustCompile(`([.!?؟])[ \t]*\.`) // "!." after a left-out call to action
	blankLinesRe  = regexp.MustCompile(`\n{3,}`)
)

// tidy repairs the punctuation a left-out word or sentence leaves behind.
func tidy(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		for range 2 {
			l = spaceRunRe.ReplaceAllString(l, " ")
			l = spaceBeforeRe.ReplaceAllString(l, "$1")
			l = openParenRe.ReplaceAllString(l, "(")
			l = emptyParenRe.ReplaceAllString(l, "")
			l = doublePunctRe.ReplaceAllString(l, "$2")
			l = leadPunctRe.ReplaceAllString(l, "")
			l = endRunRe.ReplaceAllString(l, "$1")
		}
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(blankLinesRe.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// ---- one field, one text --------------------------------------------------

// field is a parsed text: its lines.
type field [][]tok

func parseField(text string) (field, []string) {
	var f field
	var unknown []string
	for _, l := range strings.Split(text, "\n") {
		ts, u, _ := parseLine(l, false)
		f = append(f, ts)
		unknown = append(unknown, u...)
	}
	return f, unknown
}

func (f field) render(lang string) string {
	lines := make([]string, len(f))
	for i, ts := range f {
		lines[i] = renderToks(ts, lang, true)
	}
	return tidy(strings.Join(lines, "\n"))
}

// each calls fn on every sentence of the field, {o} slots included.
func (f field) each(fn func(t *tok)) {
	var walk func(ts []tok)
	walk = func(ts []tok) {
		for i := range ts {
			if ts[i].e == nil {
				continue
			}
			fn(&ts[i])
			for _, sub := range ts[i].subs {
				walk(sub)
			}
		}
	}
	for _, ts := range f {
		walk(ts)
	}
}

// shrinkSteps make a translation that is longer than the source shorter, the
// way a translator would: the short form of the terms, then without the call
// to action, the hook, the greeting.
var shrinkSteps = []func(t *tok){
	func(t *tok) {
		if t.e.short != nil {
			t.e = t.e.short
		}
	},
	drop(kindCTA),
	drop(kindHook),
	drop(kindGreet),
}

func drop(k kind) func(t *tok) {
	return func(t *tok) {
		if t.e.kind == k {
			t.e, t.text = nil, ""
		}
	}
}

func translateField(text, lang string, fits func(string) bool) string {
	f, _ := parseField(text)
	out := f.render(lang)
	for _, step := range shrinkSteps {
		if fits(out) {
			break
		}
		f.each(step)
		out = f.render(lang)
	}
	return out
}

// translateContent writes the three fields in lang, each within its limit.
func translateContent(c content.Content, lang string, p content.Policy) content.Content {
	within := func(max int) func(string) bool {
		return func(s string) bool { return p.EffectiveLength(s) <= max }
	}
	return content.Content{
		SMSBody:      translateField(c.SMSBody, lang, within(content.SMSMaxRunes)),
		EmailSubject: translateField(c.EmailSubject, lang, within(content.SubjectMaxRunes)),
		EmailBody:    translateField(c.EmailBody, lang, within(content.EmailBodyMaxRunes)),
	}
}

// smartTranslate is the mock translator's answer (see the top of this file).
func smartTranslate(req llm.TranslateRequest) string {
	if req.Target.Lang() == req.SourceLocale.Lang() {
		return toJSON(req.Source)
	}
	return toJSON(translateContent(req.Source, req.Target.Lang(), req.Policy))
}
