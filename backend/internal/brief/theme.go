package brief

import (
	"regexp"

	"smartcampaign/internal/llm"
)

// ThemeOf reads what the copy should be about and how it should sound: the
// special day, the topic (from the audience the brief describes) and the tones
// it asks for. The same theme is given to the real model and to the mock.
func ThemeOf(text, lang string) llm.Theme {
	in := Analyze(text)
	t := llm.Theme{Topic: in.topic(), Tones: tonesOf(in.folded)}
	if in.Occasion != nil {
		t.Occasion, t.OccasionName = in.Occasion.ID, in.Occasion.NameIn(lang)
	}
	return t
}

var (
	welcomeRe = regexp.MustCompile(`\bhos\s*geldi\w*|\bilk\s+(?:alisveris|siparis)\w*|\byeni\s+(?:uye|musteri)\w*`)
	thanksRe  = regexp.MustCompile(`\btesekkur\w*|\bsadik\w*|\bvip\b`)
	winbackRe = regexp.MustCompile(`\bozle\w*|\bgeri\s+(?:kazan|getir)\w*|\buzun\s+(?:zaman|suredir)\w*`)
)

func (in Intent) topic() string {
	switch {
	case in.Cart:
		return llm.TopicCart
	case in.NotPurchased && in.HasWindow, winbackRe.MatchString(in.folded):
		return llm.TopicWinback
	case in.NotPurchased, welcomeRe.MatchString(in.folded):
		return llm.TopicWelcome
	case in.Purchased, in.HasSpend, thanksRe.MatchString(in.folded):
		return llm.TopicLoyal
	}
	return llm.TopicGeneral
}

var toneLexicon = []struct {
	tone string
	re   *regexp.Regexp
}{
	{llm.ToneFriendly, regexp.MustCompile(`\b(?:samimi\w*|sicak\w*|icten\w*|dostane\w*|sen\s+dili\w*)`)},
	{llm.ToneFormal, regexp.MustCompile(`\b(?:resmi\w*|kurumsal\w*|ciddi\w*|profesyonel\w*|siz\s+dili\w*)`)},
	{llm.ToneExcited, regexp.MustCompile(`\b(?:heyecan\w*|cosku\w*|enerji\w*|eglenceli\w*|neseli\w*)`)},
	{llm.ToneShort, regexp.MustCompile(`\b(?:kisa\w*|oz\b|ozlu\w*)`)},
	{llm.ToneElegant, regexp.MustCompile(`\b(?:zarif\w*|sik\b|luks\w*|premium\w*|sofistike\w*)`)},
	{llm.ToneUrgent, regexp.MustCompile(`\b(?:acil\w*|son\s+sans\w*|son\s+firsat\w*|sinirli\s+sure\w*|kacirma\w*)`)},
}

func tonesOf(f string) []string {
	var out []string
	for _, t := range toneLexicon {
		if t.re.MatchString(f) {
			out = append(out, t.tone)
		}
	}
	return out
}
