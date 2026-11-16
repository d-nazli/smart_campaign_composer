package content

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"smartcampaign/internal/offer"
)

// Consistency checks that every language of a campaign says the same thing as
// the reference language (the default one, else the first present): the same
// discount percentages, amounts, dates, free-shipping promise and the same
// {{placeholders}}, separately for the SMS and for the email (subject and body
// together). A message in one language that promises 20% while the others
// promise 30% is a hard error, never a warning: it would reach real people.
//
// The numbers are read with the same language-neutral readers that check the
// offer (offer.Percents / Amounts / Dates), which is why every language writes
// them the same way: "%30", "30% off" and "30 % Rabatt" all read as 30.
func Consistency(set map[string]Content) []Issue {
	ref := ""
	for _, l := range Languages {
		if _, ok := set[l.Code]; ok {
			ref = l.Code
			break
		}
	}
	out := []Issue{}
	for _, l := range Languages {
		c, ok := set[l.Code]
		if !ok || l.Code == ref {
			continue
		}
		out = append(out, ConsistentWith(ref, set[ref], l.Code, c)...)
	}
	return out
}

// ConsistentWith compares one language with the reference language.
func ConsistentWith(refLang string, ref Content, lang string, c Content) []Issue {
	var out []Issue
	for _, ch := range []struct {
		field, label string
		a, b         string
	}{
		{FieldSMSBody, "SMS", ref.SMSBody, c.SMSBody},
		{FieldEmailBody, "E-posta", ref.EmailSubject + "\n" + ref.EmailBody, c.EmailSubject + "\n" + c.EmailBody},
	} {
		fa, fb := factsOf(ch.a), factsOf(ch.b)
		for _, d := range fa.differences(fb) {
			out = append(out, Issue{
				Field: ch.field, Code: CodeLanguageMismatch, Language: lang,
				Message: fmt.Sprintf("%s: %s (%s) ile %s (%s) aynı şeyi söylemeli ama %s farklı (%s: %s, %s: %s). Sayıları, tarihleri ve {{değişkenleri}} her dilde aynı yaz.",
					ch.label, displayOf(refLang), refLang, displayOf(lang), lang, d.what, refLang, d.a, lang, d.b),
			})
		}
	}
	return out
}

func displayOf(code string) string {
	if l, ok := LanguageByCode(code); ok {
		return l.Display
	}
	return code
}

type facts struct {
	percents, amounts, dates, placeholders []string
	freeShipping                           bool
}

func factsOf(text string) facts {
	f := facts{freeShipping: offer.HasFreeShipping(text)}
	for _, v := range offer.Percents(text) {
		f.percents = append(f.percents, "%"+offer.FormatNumber(v))
	}
	for _, v := range offer.Amounts(text) {
		f.amounts = append(f.amounts, offer.FormatNumber(v)+" TL")
	}
	for _, d := range offer.Dates(text) {
		f.dates = append(f.dates, offer.FormatDate(d))
	}
	for _, m := range placeholderRe.FindAllStringSubmatch(text, -1) {
		f.placeholders = append(f.placeholders, "{{"+m[1]+"}}")
	}
	f.percents, f.amounts, f.dates, f.placeholders =
		distinct(f.percents), distinct(f.amounts), distinct(f.dates), distinct(f.placeholders)
	return f
}

// distinct sorts and de-duplicates: only WHICH values appear matters, not how
// often or in what order the languages mention them.
func distinct(in []string) []string {
	sort.Strings(in)
	return slices.Compact(in)
}

type difference struct{ what, a, b string }

func (f facts) differences(g facts) []difference {
	var out []difference
	add := func(what string, a, b []string) {
		if !slices.Equal(a, b) {
			out = append(out, difference{what, listOrNone(a), listOrNone(b)})
		}
	}
	add("indirim oranları", f.percents, g.percents)
	add("tutarlar", f.amounts, g.amounts)
	add("tarihler", f.dates, g.dates)
	add("değişkenler", f.placeholders, g.placeholders)
	if f.freeShipping != g.freeShipping {
		out = append(out, difference{"ücretsiz kargo vaadi", yesNo(f.freeShipping), yesNo(g.freeShipping)})
	}
	return out
}

func listOrNone(l []string) string {
	if len(l) == 0 {
		return "yok"
	}
	return strings.Join(l, ", ")
}

func yesNo(b bool) string {
	if b {
		return "var"
	}
	return "yok"
}
