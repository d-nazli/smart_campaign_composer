package content

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"smartcampaign/internal/offer"
)

// ConsistentWith checks that a translation says the same thing as the source:
// the same discount percentages, amounts, dates, free-shipping promise and the
// same {{placeholders}} (so {{first_name}} is never lost), separately for the
// SMS and for the email (subject and body together). A translation that
// promises 20% while the source promises 30% is a hard error, never a warning:
// it would reach real people.
//
// The numbers are read with the same locale-neutral readers that check the
// offer (offer.Percents / Amounts / Dates), which is why every locale writes
// them the same way: "%30", "30% off" and "30 % Rabatt" all read as 30. Free
// shipping is read with each locale's own words; a locale that has none is not
// compared on it.
func ConsistentWith(src Locale, ref Content, loc Locale, c Content) []Issue {
	var out []Issue
	for _, ch := range []struct {
		field, label string
		a, b         string
	}{
		{FieldSMSBody, "SMS", ref.SMSBody, c.SMSBody},
		{FieldEmailBody, "E-posta", ref.EmailSubject + "\n" + ref.EmailBody, c.EmailSubject + "\n" + c.EmailBody},
	} {
		fa, fb := factsOf(ch.a, src.FreeShippingTerms), factsOf(ch.b, loc.FreeShippingTerms)
		for _, d := range fa.differences(fb, len(loc.FreeShippingTerms) > 0) {
			out = append(out, Issue{
				Field: ch.field, Code: CodeLocaleMismatch, Locale: loc.Code,
				Message: fmt.Sprintf("%s: %s, %s ile aynı şeyi söylemeli ama %s farklı (%s: %s, %s: %s). Sayıları, tarihleri ve {{değişkenleri}} kaynakla aynı yaz.",
					ch.label, loc.Code, src.Code, d.what, src.Code, d.a, loc.Code, d.b),
			})
		}
	}
	return out
}

type facts struct {
	percents, amounts, dates, placeholders []string
	freeShipping                           bool
}

func factsOf(text string, freeShippingTerms []string) facts {
	f := facts{freeShipping: offer.HasFreeShippingIn(text, freeShippingTerms)}
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
// often or in what order the source and the translation mention them.
func distinct(in []string) []string {
	sort.Strings(in)
	return slices.Compact(in)
}

type difference struct{ what, a, b string }

func (f facts) differences(g facts, compareShipping bool) []difference {
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
	if compareShipping && f.freeShipping != g.freeShipping {
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
