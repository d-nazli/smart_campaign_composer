package brief

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"smartcampaign/internal/segment"
)

// Age.
//
// An age or an age range written with numbers ("18-25 yaş arası", "65 yaş
// üstü", "30 yaşından küçük") becomes age conditions. A vague age word with no
// numbers ("genç", "yaşlı", "orta yaşlı") is never turned into a range by
// guessing: the person is asked which ages they mean. When the brief (or an
// answer to that question) gives the numbers, the numbers win and the vague
// word is only the theme.

// yasForm is "yaş" with the endings an age is written with ("yaşında",
// "yaşından", "yaştaki", "yaşlarındaki"). It never matches "yaşayan",
// "yaşam" or "yasal".
const yasForm = `yas(?:i|in|inda\w*|indan|ina|ta\w*|lar\w*)?\b`

type ageReading struct {
	re *regexp.Regexp
	// conds turns the numbers into conditions; words is how the explanation says it.
	conds func(a, b int) []segment.Condition
	words func(a, b int) string
}

const between = `(?:arasi\w*|araligi\w*|araligin\w*)`

// A range includes both ends: "18-25 yaş arası" is 18 to 25, 25 included.
func rangeConds(a, b int) []segment.Condition {
	return []segment.Condition{ageCond(segment.OpGte, min(a, b)), ageCond(segment.OpLte, max(a, b))}
}

func rangeWords(a, b int) string { return fmt.Sprintf("%d–%d yaş arası", min(a, b), max(a, b)) }

func ageCond(op string, n int) segment.Condition {
	return segment.Condition{Field: "age", Op: op, Value: int64(n)}
}

// The readings, on folded text. Ranges come first; each match is blanked
// before the next reading looks, so "18-25 yaşındaki" is one range, not also
// "25 yaşındaki".
var ageReadings = []ageReading{
	{ // "18-25 yaş arası", "18–25 yaşlarındaki", "18-25 yaş"
		re:    regexp.MustCompile(`\b(\d{1,3})\s*(?:-|–|—)\s*(\d{1,3})\s*` + yasForm + `(?:\s+` + between + `)?`),
		conds: rangeConds, words: rangeWords,
	},
	{ // "18 ile 25 yaş arası", "18 ve 25 yaş arasındaki": a word between needs "arası"
		re:    regexp.MustCompile(`\b(\d{1,3})\s+(?:ile|ila|ve)\s+(\d{1,3})\s*` + yasForm + `\s+` + between),
		conds: rangeConds, words: rangeWords,
	},
	{ // "18 yaş ile 25 yaş arası", "18 yaştan 25 yaşa kadar"
		re:    regexp.MustCompile(`\b(\d{1,3})\s*yas(?:tan)?\s*(?:-|–|—|ile|ila|ve)?\s*(\d{1,3})\s*` + yasForm + `(?:\s+(?:` + between + `|kadar\w*))?`),
		conds: rangeConds, words: rangeWords,
	},
	{ // "en az 18 yaşında"
		re:    regexp.MustCompile(`\ben\s+az\s+(\d{1,3})\s*` + yasForm),
		conds: func(a, _ int) []segment.Condition { return []segment.Condition{ageCond(segment.OpGte, a)} },
		words: func(a, _ int) string { return fmt.Sprintf("%d yaş ve üzeri", a) },
	},
	{ // "en fazla 30 yaşında"
		re:    regexp.MustCompile(`\ben\s+(?:fazla|cok)\s+(\d{1,3})\s*` + yasForm),
		conds: func(a, _ int) []segment.Condition { return []segment.Condition{ageCond(segment.OpLte, a)} },
		words: func(a, _ int) string { return fmt.Sprintf("%d yaş ve altı", a) },
	},
	{ // "65 yaşından büyük"
		re:    regexp.MustCompile(`\b(\d{1,3})\s*yasindan\s+(?:buyuk|fazla)\w*`),
		conds: func(a, _ int) []segment.Condition { return []segment.Condition{ageCond(segment.OpGt, a)} },
		words: func(a, _ int) string { return fmt.Sprintf("%d yaşından büyük", a) },
	},
	{ // "30 yaşından küçük"
		re:    regexp.MustCompile(`\b(\d{1,3})\s*yasindan\s+(?:kucuk|az)\w*`),
		conds: func(a, _ int) []segment.Condition { return []segment.Condition{ageCond(segment.OpLt, a)} },
		words: func(a, _ int) string { return fmt.Sprintf("%d yaşından küçük", a) },
	},
	{ // "30 yaş ve altı"
		re:    regexp.MustCompile(`\b(\d{1,3})\s*yas(?:in)?\s+ve\s+alt\w*`),
		conds: func(a, _ int) []segment.Condition { return []segment.Condition{ageCond(segment.OpLte, a)} },
		words: func(a, _ int) string { return fmt.Sprintf("%d yaş ve altı", a) },
	},
	{ // "65 yaş üstü", "65 yaş ve üzeri", "65 yaşın üzerindeki": the age itself included
		re:    regexp.MustCompile(`\b(\d{1,3})\s*yas(?:in)?\s+(?:ve\s+)?(?:ust|uzer)\w*`),
		conds: func(a, _ int) []segment.Condition { return []segment.Condition{ageCond(segment.OpGte, a)} },
		words: func(a, _ int) string { return fmt.Sprintf("%d yaş ve üzeri", a) },
	},
	{ // "30 yaş altı", "30 yaşın altındaki"
		re:    regexp.MustCompile(`\b(\d{1,3})\s*yas(?:in)?\s+alt\w*`),
		conds: func(a, _ int) []segment.Condition { return []segment.Condition{ageCond(segment.OpLt, a)} },
		words: func(a, _ int) string { return fmt.Sprintf("%d yaş altı", a) },
	},
	{ // "30 yaşındaki"
		re:    regexp.MustCompile(`\b(\d{1,3})\s*yasinda\w*`),
		conds: func(a, _ int) []segment.Condition { return []segment.Condition{ageCond(segment.OpEq, a)} },
		words: func(a, _ int) string { return fmt.Sprintf("%d yaşındaki", a) },
	},
}

var (
	// vagueAgeRe: an age group without numbers. "Gençlik" (as in "Gençlik ve Spor
	// Bayramı") is not one.
	vagueAgeRe = regexp.MustCompile(`\b(?:genc(?:ler\w*|e|i)?\b|yasli\w*|orta\s+yas\w*|ileri\s+yas\w*|yetiskin\w*)`)
	// anyAgeRe: "yaş" left over once the readings above are blanked ("yaşa göre",
	// "otuz yaş").
	anyAgeRe = regexp.MustCompile(`\b` + yasForm)
)

// AgeExample is how the questions show an age the code understands.
const AgeExample = `"18-25 yaş arası" veya "65 yaş ve üzeri"`

// analyzeAge reads the age of the audience and returns the spans of every age
// phrase (to be blanked, so no other reader takes "18-25" or "65" for a
// date, a time window or an amount).
func (in *Intent) analyzeAge(d *doc, f string) [][2]int {
	work := []byte(f)
	var spans [][2]int
	take := func(loc []int) {
		spans = append(spans, [2]int{loc[0], loc[1]})
		for i := loc[0]; i < loc[1]; i++ {
			work[i] = ' '
		}
	}
	var phrases []string
	for _, rd := range ageReadings {
		for _, m := range rd.re.FindAllStringSubmatchIndex(string(work), -1) {
			a, _ := strconv.Atoi(f[m[2]:m[3]])
			b := a
			if len(m) > 4 && m[4] >= 0 {
				b, _ = strconv.Atoi(f[m[4]:m[5]])
			}
			q := d.quote(m[0:2])
			take(m[0:2])
			phrases = append(phrases, q)
			if a < segment.MinAge || a > segment.MaxAge || b < segment.MinAge || b > segment.MaxAge {
				in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
					"%q: yaş %d ile %d arasında olmalı. Yaşı bu aralıkta yazın; örneğin %s.", q, segment.MinAge, segment.MaxAge, AgeExample))
				continue
			}
			if len(in.Ages) > 0 {
				continue // the second one is asked about below
			}
			in.Ages, in.AgePhrase, in.AgeWords = rd.conds(a, b), q, rd.words(a, b)
		}
	}
	if len(phrases) > 1 {
		in.Ages, in.AgePhrase, in.AgeWords = nil, "", ""
		in.block(segment.StatusNeedsClarification, strings.Join(phrases, ", "), fmt.Sprintf(
			"Brief'te birden fazla yaş koşulu var (%s). Tek bir yaş aralığı yazın; örneğin %s.", quoteAll(phrases), AgeExample))
	}
	vague := vagueAgeRe.FindAllStringIndex(string(work), -1)
	for _, loc := range vague {
		take(loc)
	}
	rest := anyAgeRe.FindAllStringIndex(string(work), -1)
	for _, loc := range rest {
		take(loc)
	}
	if len(phrases) > 0 {
		return spans // the numbers say which ages; "gençlere" next to them is the theme
	}
	switch {
	case len(vague) > 0:
		q := d.quote(vague[0])
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"%q hangi yaş aralığı olsun? Yaşı sayıyla yazın; örneğin %s.", q, AgeExample))
	case len(rest) > 0:
		q := d.quote(rest[0])
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"Brief'te yaş geçiyor (%q) ama hangi yaşlar olduğu anlaşılmıyor. Yaşı sayıyla yazın; örneğin %s.", q, AgeExample))
	}
	return spans
}

func quoteAll(list []string) string {
	q := make([]string, len(list))
	for i, s := range list {
		q[i] = strconv.Quote(s)
	}
	return strings.Join(q, " ve ")
}

// verifyAge: the rule's age conditions are exactly the brief's.
func (in Intent) verifyAge(r segment.Rule, bad func(string, ...any)) {
	var got []string
	for _, c := range r.Conditions {
		if c.Field == "age" {
			got = append(got, fmt.Sprintf("age %s %v", c.Op, c.Value))
		}
	}
	var want []string
	for _, c := range in.Ages {
		want = append(want, fmt.Sprintf("age %s %v", c.Op, c.Value))
	}
	sort.Strings(got)
	sort.Strings(want)
	switch {
	case len(want) == 0 && len(got) > 0:
		bad("Brief kitleyi yaşa göre sınırlamıyor ama kuralda age koşulu var; kaldır.")
	case len(want) > 0 && strings.Join(got, ", ") != strings.Join(want, ", "):
		bad("Brief kitleyi yaşa göre sınırlıyor (%q) ama kuraldaki yaş koşulları farklı. Aynen şunları yaz: %s.", in.AgePhrase, strings.Join(want, ", "))
	}
}
