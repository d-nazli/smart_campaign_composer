package brief

import (
	"regexp"

	"smartcampaign/internal/segment"
)

// Special days and gender.
//
// A special day ("Kadınlar Günü'ne özel indirim") is the THEME of the
// campaign: it decides what the messages talk about and how they sound, never
// who receives them. Its words are taken out of the text before anything else
// reads it, so "Kadınlar" in "Kadınlar Günü" is not a gender, "8 Mart" is not
// a date without a year and "bayram" is not a time window.
//
// The audience is narrowed by gender only when the brief says so in so many
// words ("kadın müşterilere özel", "erkeklere", "cinsiyeti kadın olan"). A
// gender word that is not clearly about who receives the message ("kadın
// koleksiyonu") changes nothing. "Kadın ve erkek müşterilere" is everyone.

// Occasion is a special day the brief names.
type Occasion struct {
	ID string `json:"id"`
	// Names is how the day is written in each language (tr, en, de).
	Names map[string]string `json:"names"`
}

// NameIn returns the day's name in a language (Turkish when it has none).
func (o Occasion) NameIn(lang string) string {
	if n, ok := o.Names[lang]; ok && n != "" {
		return n
	}
	return o.Names["tr"]
}

// Occasion IDs.
const (
	OccasionWomensDay    = "womens_day"
	OccasionMothersDay   = "mothers_day"
	OccasionFathersDay   = "fathers_day"
	OccasionValentines   = "valentines_day"
	OccasionTeachersDay  = "teachers_day"
	OccasionChildrensDay = "childrens_day"
	OccasionNewYear      = "new_year"
	OccasionBlackFriday  = "black_friday"
	OccasionRamadanFeast = "ramadan_feast"
	OccasionEidAlAdha    = "eid_al_adha"
	OccasionHoliday      = "holiday"
)

type occasionEntry struct {
	occ Occasion
	re  *regexp.Regexp
	// dayMonth marks a "day month" pattern ("8 mart"): it is a special day only
	// when no year follows; "8 Mart 2027'ye kadar" is a last day.
	dayMonth bool
}

func occ(id, tr, en, de string) Occasion {
	return Occasion{ID: id, Names: map[string]string{"tr": tr, "en": en, "de": de}}
}

// The lexicon, on folded text. More specific entries come first: their words
// are blanked before the generic "bayram" is looked for.
var occasionLexicon = []occasionEntry{
	{occ: occ(OccasionWomensDay, "Kadınlar Günü", "Women's Day", "Weltfrauentag"), re: regexp.MustCompile(`\b(?:dunya\s+)?kadinlar\s+gun\w*`)},
	{occ: occ(OccasionWomensDay, "Kadınlar Günü", "Women's Day", "Weltfrauentag"), re: regexp.MustCompile(`\b8\s+mart\w*`), dayMonth: true},
	{occ: occ(OccasionMothersDay, "Anneler Günü", "Mother's Day", "Muttertag"), re: regexp.MustCompile(`\banneler\s+gun\w*`)},
	{occ: occ(OccasionFathersDay, "Babalar Günü", "Father's Day", "Vatertag"), re: regexp.MustCompile(`\bbabalar\s+gun\w*`)},
	{occ: occ(OccasionValentines, "Sevgililer Günü", "Valentine's Day", "Valentinstag"), re: regexp.MustCompile(`\bsevgililer\s+gun\w*`)},
	{occ: occ(OccasionValentines, "Sevgililer Günü", "Valentine's Day", "Valentinstag"), re: regexp.MustCompile(`\b14\s+subat\w*`), dayMonth: true},
	{occ: occ(OccasionTeachersDay, "Öğretmenler Günü", "Teachers' Day", "Lehrertag"), re: regexp.MustCompile(`\bogretmenler\s+gun\w*`)},
	{occ: occ(OccasionTeachersDay, "Öğretmenler Günü", "Teachers' Day", "Lehrertag"), re: regexp.MustCompile(`\b24\s+kasim\w*`), dayMonth: true},
	{occ: occ(OccasionChildrensDay, "23 Nisan Çocuk Bayramı", "Children's Day", "Kindertag"), re: regexp.MustCompile(`\b23\s+nisan\w*(?:\s+(?:ulusal\s+egemenlik\s+ve\s+)?cocuk\s+bayram\w*)?`), dayMonth: true},
	{occ: occ(OccasionChildrensDay, "23 Nisan Çocuk Bayramı", "Children's Day", "Kindertag"), re: regexp.MustCompile(`\bcocuk\s+bayram\w*`)},
	{occ: occ(OccasionNewYear, "Yılbaşı", "New Year", "Neujahr"), re: regexp.MustCompile(`\b(?:yilbasi\w*|yeni\s+yil\w*)`)},
	{occ: occ(OccasionBlackFriday, "Black Friday", "Black Friday", "Black Friday"), re: regexp.MustCompile(`\b(?:black\s+friday\w*|kara\s+cuma\w*|efsane\s+cuma\w*)`)},
	{occ: occ(OccasionRamadanFeast, "Ramazan Bayramı", "Eid al-Fitr", "Zuckerfest"), re: regexp.MustCompile(`\b(?:ramazan|seker)\s+bayram\w*`)},
	{occ: occ(OccasionEidAlAdha, "Kurban Bayramı", "Eid al-Adha", "Opferfest"), re: regexp.MustCompile(`\bkurban\s+bayram\w*`)},
	{occ: occ(OccasionHoliday, "Bayram", "the holidays", "die Feiertage"), re: regexp.MustCompile(`\bbayram\w*`)},
}

var yearFollowsRe = regexp.MustCompile(`^\s+\d{4}\b`)

// findOccasions returns the first special day the folded text names and the
// byte spans of every special-day phrase (to be blanked).
func findOccasions(f string) (*Occasion, [][2]int) {
	var first *Occasion
	firstAt := len(f) + 1
	var spans [][2]int
	work := []byte(f)
	for _, e := range occasionLexicon {
		for _, loc := range e.re.FindAllStringIndex(string(work), -1) {
			if e.dayMonth && yearFollowsRe.MatchString(f[loc[1]:]) {
				continue
			}
			spans = append(spans, [2]int{loc[0], loc[1]})
			for i := loc[0]; i < loc[1]; i++ {
				work[i] = ' '
			}
			if loc[0] < firstAt {
				o := e.occ
				first, firstAt = &o, loc[0]
			}
		}
	}
	return first, spans
}

// Gender phrases, on folded text with the special days already blanked.
var (
	bothGendersRe = regexp.MustCompile(`\b(?:kadin|bayan)\w*\s+(?:ve|veya|ile|ya\s+da)\s+erkek\w*|\berkek\w*\s+(?:ve|veya|ile|ya\s+da)\s+(?:kadin|bayan)\w*`)
	femaleRe      = regexp.MustCompile(`\b(?:(?:kadin|bayan)\s+(?:musteri|kullanici|uye|alici|abone|kisi)\w*|(?:kadin|bayan)lar(?:a|imiz\w*)\b|(?:kadin|bayan)lar\s+icin\b|kizlara\b|cinsiyet\w*\s+(?:kadin|bayan)\w*)`)
	maleRe        = regexp.MustCompile(`\b(?:erkek\s+(?:musteri|kullanici|uye|alici|abone|kisi)\w*|erkekler(?:e|imiz\w*)\b|erkekler\s+icin\b|baylara\b|bay\s+(?:musteri|kullanici|uye)\w*|cinsiyet\w*\s+erkek\w*)`)
	// genderWordRe is any remaining gender word (blanked so nothing else reads it).
	genderWordRe = regexp.MustCompile(`\b(?:kadin\w*|bayan\w*|erkek\w*|kizlar\w*|cinsiyet\w*)`)
)

// analyzeGender decides the gender condition and returns the spans of every
// gender word (to be blanked).
func (in *Intent) analyzeGender(d *doc, f string) [][2]int {
	var spans [][2]int
	if loc := bothGendersRe.FindStringIndex(f); loc != nil {
		// "kadın ve erkek müşterilere" is everyone who may be contacted: no filter.
		in.GenderPhrase, in.BothGenders = d.quote(loc), true
		for _, l := range genderWordRe.FindAllStringIndex(f, -1) {
			spans = append(spans, [2]int{l[0], l[1]})
		}
		return spans
	}
	female, male := femaleRe.FindStringIndex(f), maleRe.FindStringIndex(f)
	switch {
	case female != nil && male != nil:
		in.block(segment.StatusNeedsClarification, d.quote(female), "Brief hem kadın hem erkek müşterilerden ayrı ayrı söz ediyor. Kampanya kime gidecek: kadınlara mı, erkeklere mi, herkese mi?")
	case female != nil:
		in.Gender, in.GenderPhrase = "female", d.quote(female)
	case male != nil:
		in.Gender, in.GenderPhrase = "male", d.quote(male)
	}
	for _, l := range genderWordRe.FindAllStringIndex(f, -1) {
		spans = append(spans, [2]int{l[0], l[1]})
	}
	return spans
}

// genderWords are the audience words for the explanation.
func genderWords(g string) string {
	if g == "male" {
		return "erkek müşteriler"
	}
	return "kadın müşteriler"
}

// mask blanks byte spans of the folded text, rune by rune, keeping the folded
// and original texts aligned (a blanked rune becomes one space).
func (d *doc) mask(spans [][2]int) {
	if len(spans) == 0 {
		return
	}
	for _, sp := range spans {
		for i := d.byteToRune[sp[0]]; i < d.byteToRune[sp[1]]; i++ {
			d.fold[i] = ' '
		}
	}
	d.foldString = string(d.fold)
	d.byteToRune = make([]int, len(d.foldString)+1)
	r := 0
	for i := range d.foldString {
		d.byteToRune[i] = r
		r++
	}
	d.byteToRune[len(d.foldString)] = r
}

// AudienceDefaultedNote is shown in the audience step when a special-day brief
// names nobody and the campaign goes to everyone who may be contacted.
const AudienceDefaultedNote = "Kitle belirtilmediği için tüm müşteriler seçildi."
