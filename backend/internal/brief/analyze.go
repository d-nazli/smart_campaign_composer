// Package brief reads a marketer's brief with plain code and checks the
// model's segment rule against it.
//
// Why code and not only the model: a model that does not understand part of a
// brief tends to leave that part out and answer confidently. Nothing in the
// rule itself shows what is missing. This package extracts, from the brief
// text alone, what the brief asks for (cities, a time window, bought/not
// bought, the offer) and what we cannot serve. Then:
//
//   - if the brief needs something we cannot express, the answer is
//     unsupported or needs_clarification, decided here, with the offending
//     words quoted. The model is not asked.
//   - otherwise the model's rule must contain every part found here (Verify).
//     If it left one out it is told which and asked again; if it never fixes
//     it, the person gets a question, not a quietly different audience.
//
// The lexicon is deliberately limited to what the data model can express. It
// is a safety net, not a language model: words it does not know are caught by
// the model's own "unsupported" judgement and by the unknown-negation check.
//
// All of this is the mock LLM's flow (Strict). With the real model
// (ModelDriven, D113) the segment step does not read the brief with code at
// all: Claude interprets it and only the technical checks of segment.Parse
// apply (see resolve.go).
package brief

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
	"smartcampaign/internal/trtext"
)

// Blocker is something in the brief that cannot be turned into a rule.
type Blocker struct {
	Status  string // segment.StatusUnsupported or segment.StatusNeedsClarification
	Phrase  string // the words the person wrote
	Message string // shown on screen (Turkish)
	// Suggestion is a one-click answer the screen can offer (segment.SuggestAllCustomers).
	Suggestion string
}

// Intent is what the brief text asks for.
type Intent struct {
	Included, Excluded []string // canonical city names
	Cart               bool     // the brief is about cart additions
	Abandoned          bool     // "sepeti terk eden", "ödemeyi tamamlamamış"
	NotPurchased       bool     // did not buy / order
	Purchased          bool     // bought / ordered
	HasWindow          bool
	WindowDays         int
	WindowPhrase       string
	NegationPhrase     string      // what the person wrote for "did not buy"
	Offer              offer.Offer // including its terms: minimum basket and last day
	Everyone           bool        // the campaign goes to all customers (said so, or defaulted for a special day)
	// AudienceDefaulted: a special-day brief named nobody, so Everyone was chosen
	// (the screen says so; no question is asked).
	AudienceDefaulted bool
	// Occasion is the special day the brief names (nil if none): the theme of the
	// messages, never an audience.
	Occasion *Occasion
	// Gender is "female" or "male" when the brief explicitly restricts the
	// audience by gender; GenderPhrase is the words it used. BothGenders: the
	// brief names both ("kadın ve erkek müşterilere"), which is everyone.
	Gender, GenderPhrase string
	BothGenders          bool
	HasSpend             bool   // an explicit total-spending audience (toplam harcaması N TL'den fazla)
	SpendOp              string // gt, gte or lt
	SpendValue           float64
	Blockers             []Blocker

	folded   string   // the folded brief, for checks that need to look at it again
	consumed [][2]int // spans (byte offsets in folded) already explained as part of an offer term
}

// ---- small text helpers ----------------------------------------------------

// doc keeps the original and folded text aligned rune for rune, so a match
// found in the folded text can be quoted the way the person wrote it.
type doc struct {
	orig, fold  []rune
	foldString  string
	byteToRune  []int
	alignedText bool
}

func newDoc(text string) *doc {
	text = strings.TrimSpace(text)
	d := &doc{orig: []rune(text)}
	d.foldString = trtext.Fold(text)
	d.fold = []rune(d.foldString)
	d.alignedText = len(d.orig) == len(d.fold)
	d.byteToRune = make([]int, len(d.foldString)+1)
	r := 0
	for i := range d.foldString {
		d.byteToRune[i] = r
		r++
	}
	d.byteToRune[len(d.foldString)] = r
	return d
}

// quote returns the original words for a byte span of the folded text.
func (d *doc) quote(loc []int) string {
	a, b := d.byteToRune[loc[0]], d.byteToRune[loc[1]]
	if d.alignedText {
		return strings.TrimSpace(string(d.orig[a:b]))
	}
	return strings.TrimSpace(string(d.fold[a:b]))
}

// blank replaces a span of the working copy with spaces (same rune count).
func blank(work []rune, d *doc, loc []int) {
	for i := d.byteToRune[loc[0]]; i < d.byteToRune[loc[1]]; i++ {
		work[i] = ' '
	}
}

// ---- lexicon ---------------------------------------------------------------

const numWords = `(?:bir|iki|uc|dort|bes|alti|yedi|sekiz|dokuz|on|yirmi|otuz|kirk|elli|altmis|yetmis|seksen|doksan)(?:\s+(?:bir|iki|uc|dort|bes|alti|yedi|sekiz|dokuz))?`
const num = `(?:\d+|` + numWords + `)`
const unit = `(?:gun(?:de|u|luk|dur|den)?|hafta(?:da|yi|lik|dir)?|ay(?:da|i|lik|dir)?|yil(?:da|i|lik|dir)?)`

var (
	windowRe      = regexp.MustCompile(`\bson\s+(` + num + `)\s*(` + unit + `)\b`)
	windowInRe    = regexp.MustCompile(`\b(` + num + `)\s*(` + unit + `)\s+(?:icinde|icerisinde)\b`)
	vagueWindowRe = regexp.MustCompile(`\bson\s+(\S+)\s+(` + unit + `)\b`)
	hoursRe       = regexp.MustCompile(`\bson\s+(?:(` + num + `)\s*)?(saat|dakika)\w*`)
	calendarRe    = regexp.MustCompile(`\b(?:(?:bu|gecen|onceki|gelecek|gelen)\s+(?:ay|hafta|yil|sezon|donem|pazartesi|sali|carsamba|persembe|cuma|cumartesi|pazar)|bugun|dun|yarin)\b`)

	cartRe = regexp.MustCompile(`\bsepet\w*`)

	abandonRes = []*regexp.Regexp{
		regexp.MustCompile(`\bsepet\w*\s+(?:terk|birak)\w*`),
		regexp.MustCompile(`\bterk\s+edilmis\s+sepet\w*`),
		regexp.MustCompile(`\b(?:odeme\w*|siparis\w*|alisveris\w*|satin\s+alma\w*|islem\w*|sepet\w*)\s+(?:\w+\s+)?tamamla(?:mamis|mayan)\w*`),
	}
	// The trailing \w* accepts Turkish endings: "almayanlara", "yapmayanlar".
	notBoughtRes = []*regexp.Regexp{
		regexp.MustCompile(`\b(?:siparis|alisveris|satin\s+alma|alim)\s+(?:ver|olustur|yap|et)(?:memis|meyen|mamis|mayan)\w*`),
		regexp.MustCompile(`\bsatin\s+al(?:mamis|mayan)\w*`),
		regexp.MustCompile(`\balmamis\w*|\balmayan\w*`),
	}
	boughtRes = []*regexp.Regexp{
		regexp.MustCompile(`\b(?:siparis)\s+(?:veren|vermis|olusturan|olusturmus|yapan|yapmis)\w*`),
		regexp.MustCompile(`\b(?:alisveris|satin\s+alma|alim)\s+(?:yapan|yapmis|eden|etmis)\w*`),
		regexp.MustCompile(`\bsatin\s+(?:alan|almis)\w*`),
	}
	unknownNegationRe = regexp.MustCompile(`\b[a-z]+(?:mamis|memis|mayan|meyen)\w*`)

	wordPctRe = regexp.MustCompile(`\byuzde\s+(` + numWords + `)\b`)

	// ---- amounts that are NOT offers: thresholds -------------------------
	// "1000 TL üzeri alışverişe 100 TL indirim": the 1000 is a TERM of the offer.
	// "toplam harcaması 1000 TL'den fazla olanlar": it is an AUDIENCE.
	amtPattern = `(\d+(?:[.,]\d+)*)\s*(?:tl|try|lira|₺)`
	amtSuffix  = `(?:['’]?[a-z]*)`
	overRe     = regexp.MustCompile(amtPattern + amtSuffix + `\s*(?:ve\s+)?(?:d[ae]n\s+)?(?:uzer\w*|ust\w*|asan\w*|fazla\w*)`)
	underRe    = regexp.MustCompile(amtPattern + amtSuffix + `\s*(?:ve\s+)?(?:d[ae]n\s+)?(?:altinda\w*|alti\w*|az\b|azi\w*)`)
	enAzRe     = regexp.MustCompile(`\ben\s+az\s+` + amtPattern + amtSuffix)
	likRe      = regexp.MustCompile(amtPattern + `['’]?lik\b`)

	explicitSpendRe = regexp.MustCompile(`\b(?:toplam\w*|harcamasi|harcayan\w*|harcamis\w*|harcama\s+tutari)\b`)
	shopAfterRe     = regexp.MustCompile(`^[\s,.]*(?:[a-z0-9'’]+\s+)?(?:alisveris|sepet|siparis|harcama|urun|alim|odeme)\w*`)
	audAfterRe      = regexp.MustCompile(`^[\s,.]*(?:[a-z0-9'’]+\s+){0,3}?(?:yapan|eden|olan|musteri|kullanici|kisi|uye|kitle|abone)\w*`)
	andOverRe       = regexp.MustCompile(`\bve\s+(?:uzer|ust)`)

	everyoneRe  = regexp.MustCompile(`\b(?:herkes\w*|(?:tum|butun)\s+(?:musteri|kullanici|uye|kisi|kitle|abone|alici|kayitli)\w*)`)
	spendWordRe = regexp.MustCompile(`\b(?:toplam\s+harcama\w*|harcayan\w*|harcamis\w*|harcamasi\w*)`)
	expiryCueRe = regexp.MustCompile(`\b(?:son\s+(?:kullan\w*|gecerl\w*|gun\w*)|gecerli\w*|gecerlilik\w*|kadar|bitis\w*|sona\s+er\w*)`)
	kodRe       = regexp.MustCompile(`\bkod\w*`)
)

// What we cannot serve: the data model has no such column.
var unsupportedLexicon = []struct {
	label string
	re    *regexp.Regexp
}{
	{"yaş", regexp.MustCompile(`\b(?:yas|yasi|yasli\w*|yasinda\w*|yaslar\w*|genc\w*)\b`)},
	{"ürün kategorisi / marka", regexp.MustCompile(`\b(?:kategori\w*|urun\s+grub\w*|marka\w*|ayakkabi\w*|canta\w*|elbise\w*|pantolon\w*|gomlek\w*|tisort\w*|ceket\w*|kazak\w*|giyim\w*|telefon\w*|bilgisayar\w*|laptop\w*|televizyon\w*|kulaklik\w*|parfum\w*|makyaj\w*|kozmetik\w*|mobilya\w*|oyuncak\w*|kitap\w*|elektronik\w*|aksesuar\w*|samsung\w*|apple|nike|adidas)\b`)},
	{"doğum günü", regexp.MustCompile(`\bdogum\w*`)},
	{"üyelik / kayıt tarihi", regexp.MustCompile(`\b(?:uyelik\w*|uye\s+ol\w*|kayit\s+(?:ol\w*|tarih\w*))\b`)},
	{"e-posta/SMS etkileşimi", regexp.MustCompile(`\b(?:acan\w*|tiklayan\w*|tiklamis|acmayan\w*|acmamis)\b`)},
}

// ---- numbers ---------------------------------------------------------------

var wordValues = map[string]int{
	"bir": 1, "iki": 2, "uc": 3, "dort": 4, "bes": 5, "alti": 6, "yedi": 7, "sekiz": 8, "dokuz": 9,
	"on": 10, "yirmi": 20, "otuz": 30, "kirk": 40, "elli": 50, "altmis": 60, "yetmis": 70, "seksen": 80, "doksan": 90,
}

// parseCount reads "30", "iki" or "on bes".
func parseCount(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	parts := strings.Fields(s)
	switch len(parts) {
	case 1:
		v, ok := wordValues[parts[0]]
		return v, ok
	case 2:
		a, okA := wordValues[parts[0]]
		b, okB := wordValues[parts[1]]
		if okA && okB && a >= 10 && b < 10 {
			return a + b, true
		}
	}
	return 0, false
}

func unitDays(u string) int {
	switch {
	case strings.HasPrefix(u, "gun"):
		return 1
	case strings.HasPrefix(u, "hafta"):
		return 7
	case strings.HasPrefix(u, "ay"):
		return 30
	}
	return 365
}

// ---- Analyze ---------------------------------------------------------------

// Analyze extracts what the brief asks for and what cannot be served.
func Analyze(text string) Intent {
	d := newDoc(text)
	in := Intent{Offer: offer.Offer{Kind: offer.KindNone}}
	// Special days and gender words are read first and taken out of the text:
	// "Kadınlar Günü" is a theme, not a gender; "8 Mart" is not a date.
	var spans [][2]int
	in.Occasion, spans = findOccasions(d.foldString)
	d.mask(spans)
	d.mask(in.analyzeGender(d, d.foldString))
	in.folded = d.foldString
	f := d.foldString
	work := append([]rune(nil), d.fold...) // known phrases are blanked out of this copy

	in.analyzeUnsupported(d, f)
	in.analyzeTime(d, f)
	in.analyzeOffer(d, text)
	in.analyzeCities(f)
	in.analyzePurchase(d, f, work)
	in.crossChecks()
	in.analyzeAudience(f)
	return in
}

func (in *Intent) block(status, phrase, msg string) {
	in.Blockers = append(in.Blockers, Blocker{Status: status, Phrase: phrase, Message: msg})
}

func (in *Intent) analyzeUnsupported(d *doc, f string) {
	for _, u := range unsupportedLexicon {
		if loc := u.re.FindStringIndex(f); loc != nil {
			q := d.quote(loc)
			what := fmt.Sprintf("%q", q)
			if trtext.Fold(q) != trtext.Fold(u.label) { // "kadınlara" -> (cinsiyet); not "yaş" (yaş)
				what += " (" + u.label + ")"
			}
			in.block(segment.StatusUnsupported, q, fmt.Sprintf(
				"Brief'te %s geçiyor. Bu bilgi müşteri veritabanımızda yok, bu yüzden bu koşulu uygulayamıyoruz ve kitleyi oluşturmadık.", what))
		}
	}
}

func (in *Intent) analyzeTime(d *doc, f string) {
	type win struct {
		days   int
		phrase string
	}
	var wins []win
	for _, re := range []*regexp.Regexp{windowRe, windowInRe} {
		for _, m := range re.FindAllStringSubmatchIndex(f, -1) {
			n, ok := parseCount(f[m[2]:m[3]])
			if !ok || n <= 0 {
				continue
			}
			wins = append(wins, win{n * unitDays(f[m[4]:m[5]]), d.quote(m[0:2])})
		}
	}
	for _, m := range vagueWindowRe.FindAllStringSubmatchIndex(f, -1) {
		if _, ok := parseCount(f[m[2]:m[3]]); !ok {
			q := d.quote(m[0:2])
			example := `"son 30 gün" veya "son 2 hafta"`
			// The example uses the number the person wrote ("son 45+ gün" -> "son 45 gün").
			if digits := digitsRe.FindString(f[m[2]:m[3]]); digits != "" {
				example = fmt.Sprintf(`"son %s %s"`, digits, unitWord(f[m[4]:m[5]]))
			}
			in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
				"%q belirsiz. Kaç gün olduğunu sayıyla yazın; örneğin %s.", q, example))
		}
	}
	for _, m := range hoursRe.FindAllStringSubmatchIndex(f, -1) {
		q := d.quote(m[0:2])
		days := 1 // "son 3 saat" -> "son 1 gün"; "son 48 saat" -> "son 2 gün"
		if m[2] >= 0 && strings.HasPrefix(f[m[4]:m[5]], "saat") {
			if n, ok := parseCount(f[m[2]:m[3]]); ok && n > 24 {
				days = (n + 23) / 24
			}
		}
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"%q saat/dakika düzeyinde bir aralık; kitle yalnızca gün cinsinden tanımlanabiliyor. Örneğin \"son %d gün\" yazın.", q, days))
	}
	for _, loc := range calendarRe.FindAllStringIndex(f, -1) {
		if strings.HasPrefix(strings.TrimLeft(f[loc[1]:], " "), "sonu") { // "bu ay sonuna kadar": a deadline in the copy
			continue
		}
		q := d.quote(loc)
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"%q takvime bağlı bir zaman ifadesi; kitle kuralında karşılığı yok. Kayan bir aralık yazın; örneğin \"son 30 gün\" veya \"son 7 gün\".", q))
	}

	switch {
	case len(wins) == 0:
	default:
		in.HasWindow, in.WindowDays, in.WindowPhrase = true, wins[0].days, wins[0].phrase
		for _, w := range wins[1:] {
			if w.days != wins[0].days {
				in.block(segment.StatusNeedsClarification, w.phrase, fmt.Sprintf(
					"Brief'te birden fazla zaman aralığı var (%q ve %q). Hangisinin kullanılacağını netleştirin.", wins[0].phrase, w.phrase))
				break
			}
		}
		if in.WindowDays > 3650 {
			in.block(segment.StatusNeedsClarification, in.WindowPhrase, fmt.Sprintf("%q en fazla 3650 gün (10 yıl) olabilir.", in.WindowPhrase))
		}
	}
}

// Tiers and caps are TERMS of an offer, read before anything else looks at
// the amounts: "her 1000 TL'lik alışverişe 100 TL" is one tiered discount, not
// a minimum basket plus a second offer, and "en fazla 300 TL" is a cap.
var (
	tierRe = regexp.MustCompile(`\bher\s+(\d+(?:[.,]\d+)*)\s*(?:tl|try|lira|₺)(?:['’]?[a-z]*)?(?:\s+(?:alisveris|harcama|sepet|siparis|tutar)\w*)?\s+(?:icin\s+|basina\s+)?(\d+(?:[.,]\d+)*)\s*(?:tl|try|lira|₺)(?:['’]?[a-z]*)?`)
	capRe  = regexp.MustCompile(`\b(?:toplam(?:da)?\s+)?(?:en\s+fazla|en\s+cok|maksimum|maks|azami|max)\s+(\d+(?:[.,]\d+)*)\s*(?:tl|try|lira|₺)(?:['’]?[a-z]*)?`)
	// "300 TL'ye kadar / varan" (also written "300 tl ye varan"). What follows decides
	// what it is: "... indirim" is a cap; "1000 TL'ye varan siparişte" is a basket
	// that REACHES 1000 TL, i.e. the minimum basket (D88).
	capUpToRe   = regexp.MustCompile(`(\d+(?:[.,]\d+)*)\s*(?:tl|try|lira|₺)\s?['’]?\s?(?:ye|ya|e|a)\s+(kadar|varan|ulasan)\b`)
	reachShopRe = regexp.MustCompile(`^\s+(?:siparis|alisveris|sepet|harcama|tutar)\w*`)
	// Words that turn two incentives into a choice ("%20 ya da ücretsiz kargo").
	alternativeRe = regexp.MustCompile(`\b(?:ya\s+da|veya|yahut|yoksa)\b`)
	// Where an incentive is written, to look between the first and the last one.
	offerMentionRe = regexp.MustCompile(`%\s*\d|\d\s*%|\byuzde\s|\d\s*(?:tl|try|lira)\b|\d\s*₺|₺\s*\d|\bucretsiz\s+(?:kargo|gonderim|teslimat)|\bbedava\s+kargo|\bkargo\s+(?:ucretsiz|bedava)`)
)

func (in *Intent) analyzeOffer(d *doc, text string) {
	f := d.foldString
	// Work on a copy in which every phrase that is NOT an incentive itself (a
	// tier, a cap, a threshold, a date) is blanked, so none of them is mistaken
	// for another offer.
	scan := []byte(f)
	blank := func(a, b int) {
		for i := a; i < b; i++ {
			scan[i] = ' '
		}
	}
	quote := func(a, b int) string { return d.quote([]int{a, b}) }

	var tierStep, tierValue float64
	for _, m := range tierRe.FindAllStringSubmatchIndex(f, -1) {
		step, ok1 := offer.ParseNumber(f[m[2]:m[3]])
		value, ok2 := offer.ParseNumber(f[m[4]:m[5]])
		if !ok1 || !ok2 {
			continue
		}
		if tierStep > 0 && (step != tierStep || value != tierValue) {
			in.block(segment.StatusNeedsClarification, quote(m[0], m[1]), "Brief'te birden fazla kademeli indirim var. Tek bir kademe yazın; örneğin \"her 1000 TL'lik alışverişe 100 TL indirim\".")
		}
		tierStep, tierValue = step, value
		blank(m[0], m[1])
	}
	var capValue, reachMin float64
	capPhrase := ""
	for _, re := range []*regexp.Regexp{capRe, capUpToRe} {
		for _, m := range re.FindAllStringSubmatchIndex(string(scan), -1) {
			v, ok := offer.ParseNumber(f[m[2]:m[3]])
			if !ok {
				continue
			}
			if loc := reachShopRe.FindStringIndex(f[m[1]:]); re == capUpToRe && loc != nil {
				blank(m[0], m[1])
				// The shopping word belongs to the offer ("sepete"), not to a cart audience.
				in.consumed = append(in.consumed, [2]int{m[1] + loc[0], m[1] + loc[1]})
				if f[m[4]:m[5]] == "kadar" { // "1000 TL'ye kadar alışverişte": an upper basket limit
					q := quote(m[0], m[1])
					in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
						"%q alışverişin üst sınırı gibi okunuyor; teklifler yalnızca alt limitle (\"%s TL ve üzeri alışverişte\") ya da indirim üst sınırıyla (\"en fazla %s TL indirim\") tanımlanabiliyor. Hangisini kastettiğinizi yazın.",
						q, offer.FormatNumber(v), offer.FormatNumber(v)))
					continue
				}
				if reachMin > 0 && v != reachMin {
					in.block(segment.StatusNeedsClarification, quote(m[0], m[1]), "Brief'te birden fazla alışveriş alt limiti var. Teklifin tek bir alt limiti olabilir; hangisini istediğinizi yazın.")
				}
				reachMin = v
				continue
			}
			if capValue > 0 && v != capValue {
				in.block(segment.StatusNeedsClarification, quote(m[0], m[1]), "Brief'te birden fazla indirim üst sınırı var. Tek bir üst sınır yazın; örneğin \"toplamda en fazla 300 TL\".")
			}
			capValue, capPhrase = v, quote(m[0], m[1])
			blank(m[0], m[1])
		}
	}

	masked := string(scan)
	minSpend := in.analyzeThresholds(d, masked, blank)
	if reachMin > 0 {
		if minSpend > 0 && minSpend != reachMin {
			in.block(segment.StatusNeedsClarification, "", "Brief'te birden fazla alışveriş alt limiti var. Teklifin tek bir alt limiti olabilir; hangisini istediğinizi yazın.")
		}
		minSpend = reachMin
	}
	expires := in.analyzeDates(d, masked, blank)
	s := string(scan)

	pcts := offer.Percents(s)
	for _, m := range wordPctRe.FindAllStringSubmatch(s, -1) {
		if n, ok := parseCount(m[1]); ok {
			pcts = append(pcts, float64(n))
		}
	}
	pcts = distinct(pcts)
	amts := distinct(offer.Amounts(s))
	free := offer.HasFreeShipping(s)
	code := findCoupon(text)

	var found []string
	for _, p := range pcts {
		found = append(found, "%"+offer.FormatNumber(p))
	}
	if tierStep > 0 {
		found = append(found, fmt.Sprintf("her %s TL'ye %s TL", offer.FormatNumber(tierStep), offer.FormatNumber(tierValue)))
	}
	for _, a := range amts {
		found = append(found, offer.FormatNumber(a)+" TL")
	}
	if free {
		found = append(found, "ücretsiz kargo")
	}
	shown := strings.Join(found, ", ")

	// Two values of the same kind contradict each other: which one is meant?
	if len(pcts) > 1 || len(amts) > 1 || (tierStep > 0 && len(amts) > 0) {
		in.block(segment.StatusNeedsClarification, shown, fmt.Sprintf(
			"Brief'te aynı türden birden fazla teklif var (%s). Hangisinin geçerli olduğunu yazın; farklı türden teklifler (örneğin %%20 indirim + ücretsiz kargo) birlikte verilebilir.", shown))
		return
	}
	var parts []offer.Part
	if len(pcts) == 1 {
		parts = append(parts, offer.Part{Kind: offer.KindPercent, Value: pcts[0]})
	}
	switch {
	case tierStep > 0:
		parts = append(parts, offer.Part{Kind: offer.KindAmount, Value: tierValue})
	case len(amts) == 1:
		parts = append(parts, offer.Part{Kind: offer.KindAmount, Value: amts[0]})
	}
	if free {
		parts = append(parts, offer.Part{Kind: offer.KindFreeShipping})
	}
	// Several incentives of different kinds go together, unless the brief offers
	// them as a choice.
	if len(parts) > 1 {
		locs := offerMentionRe.FindAllStringIndex(s, -1)
		if len(locs) > 1 && alternativeRe.MatchString(s[locs[0][0]:locs[len(locs)-1][1]]) {
			in.block(segment.StatusNeedsClarification, shown, fmt.Sprintf(
				"Brief teklifleri seçenek olarak veriyor (%s, \"ya da / veya\" ile). Mesajda hepsi birlikte mi verilecek, yoksa yalnızca biri mi? Birlikteyse \"ve\" ya da \"+\" ile yazın.", shown))
			return
		}
	}
	if tierStep > 0 && len(pcts) > 0 {
		in.block(segment.StatusNeedsClarification, shown, fmt.Sprintf(
			"Brief'te kademeli TL indirimi (%s) ile yüzde indirim birlikte var. Kademeli indirim yüzdeyle birleştirilemiyor; hangisini istediğinizi yazın.", shown))
		return
	}

	switch {
	case len(parts) > 0:
		in.Offer = offer.Offer{Kind: parts[0].Kind, Value: parts[0].Value, Code: code}
		if len(parts) > 1 {
			in.Offer.Extras = parts[1:]
		}
	case code != "":
		in.Offer = offer.Offer{Kind: offer.KindCouponCode, Code: code}
	}
	if tierStep > 0 {
		in.Offer.StepSpend = tierStep
	}
	if capValue > 0 {
		_, hasPct := partOf(parts, offer.KindPercent)
		_, hasAmt := partOf(parts, offer.KindAmount)
		switch {
		case hasPct && !hasAmt, tierStep > 0:
			in.Offer.MaxDiscount = capValue
		default:
			in.block(segment.StatusNeedsClarification, capPhrase, fmt.Sprintf(
				"%q bir üst sınır ama hangi indirimin üst sınırı olduğu belli değil. Üst sınır yalnızca yüzde indirimde (\"%%20 indirim, en fazla %s TL\") ya da kademeli indirimde (\"her 1000 TL'ye 100 TL, toplamda en fazla %s TL\") kullanılabilir.",
				capPhrase, offer.FormatNumber(capValue), offer.FormatNumber(capValue)))
			in.Offer = offer.Offer{Kind: offer.KindNone}
			return
		}
	}
	in.Offer = in.Offer.Normalize()
	if err := in.Offer.Validate(); err != nil {
		in.block(segment.StatusNeedsClarification, shown, fmt.Sprintf(
			"Teklif geçersiz (%s): %v.", shown, err))
		in.Offer = offer.Offer{Kind: offer.KindNone}
		return
	}
	if code == "" && kodRe.MatchString(f) {
		loc := kodRe.FindStringIndex(f)
		q := d.quote(loc)
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"Brief'te kupon %q geçiyor ama kodun kendisini bulamadım. Kupon kodunu yazar mısınız? (örneğin \"HOSGELDIN10 kodu\")", q))
	}

	// The terms belong to the offer. Without an offer they would be dropped
	// silently, so ask instead.
	if minSpend > 0 || expires != "" {
		if in.Offer.IsNone() {
			var what []string
			if minSpend > 0 {
				what = append(what, offer.FormatNumber(minSpend)+" TL alt limiti")
			}
			if expires != "" {
				what = append(what, offer.FormatDate(expires)+" son kullanma tarihi")
			}
			in.block(segment.StatusNeedsClarification, strings.Join(what, ", "), fmt.Sprintf(
				"Brief'te teklif şartı var (%s) ama hangi teklif için olduğu yazılmamış. Teklifi de yazın: yüzde indirim, TL tutarı, ücretsiz kargo ya da kupon kodu.", strings.Join(what, ", ")))
			return
		}
		in.Offer.MinSpend, in.Offer.ExpiresAt = minSpend, expires
	}
}

func partOf(parts []offer.Part, k offer.Kind) (offer.Part, bool) {
	for _, p := range parts {
		if p.Kind == k {
			return p, true
		}
	}
	return offer.Part{}, false
}

// spanKind says what an amount with "üzeri / fazla / altı" in front of it is.
type spanKind int

const (
	spanTerm      spanKind = iota // a term of the offer: "1000 TL üzeri alışverişe 100 TL indirim"
	spanSpend                     // an audience: "toplam harcaması 1000 TL'den fazla"
	spanAmbiguous                 // could be either: ask
	spanIgnore                    // not a threshold at all
)

type threshold struct {
	start, end int
	number     string // as the person wrote it
	under      bool
	orEqual    bool // "ve üzeri", "en az"
	form       string
}

// analyzeThresholds finds amount thresholds. A threshold that qualifies a
// purchase ("1000 TL üzeri alışverişe") is the offer's minimum basket; one that
// describes a customer's total spending is an audience condition; anything
// else is ambiguous and is asked about, with the person's own number in the
// example. It returns the minimum basket (0 if none) and blanks every span it
// consumed.
func (in *Intent) analyzeThresholds(d *doc, f string, blank func(a, b int)) float64 {
	var found []threshold
	collect := func(re *regexp.Regexp, under bool, form string) {
		for _, m := range re.FindAllStringSubmatchIndex(f, -1) {
			t := threshold{start: m[0], end: m[1], number: f[m[2]:m[3]], under: under, form: form}
			t.orEqual = form == "enaz" || andOverRe.MatchString(f[m[0]:m[1]])
			found = append(found, t)
		}
	}
	collect(overRe, false, "over")
	collect(underRe, true, "under")
	collect(enAzRe, false, "enaz")
	collect(likRe, false, "lik")
	sort.Slice(found, func(i, j int) bool { return found[i].start < found[j].start })

	var minSpends []float64
	lastEnd := -1
	for _, t := range found {
		if t.start < lastEnd {
			continue // overlaps one we already handled
		}
		lastEnd = t.end
		value, ok := offer.ParseNumber(t.number)
		if !ok {
			continue
		}
		before := f[max(0, t.start-40):t.start]
		after := f[t.end:min(len(f), t.end+60)]
		kind := classifyThreshold(t, before, after)
		switch kind {
		case spanIgnore:
			continue
		case spanTerm:
			minSpends = append(minSpends, value)
			// The shopping word that ends the term ("... üzeri sepete") belongs to the
			// offer; it must not be read again as "the brief is about carts".
			if loc := shopAfterRe.FindStringIndex(after); loc != nil {
				in.consumed = append(in.consumed, [2]int{t.end + loc[0], t.end + loc[1]})
			}
		case spanSpend:
			op := segment.OpGt
			switch {
			case t.under:
				op = segment.OpLt
			case t.orEqual:
				op = segment.OpGte
			}
			if in.HasSpend && (in.SpendValue != value || in.SpendOp != op) {
				in.block(segment.StatusNeedsClarification, d.quote([]int{t.start, t.end}),
					"Brief'te birden fazla toplam harcama tutarı var. Hangisinin kullanılacağını netleştirin.")
			}
			in.HasSpend, in.SpendOp, in.SpendValue = true, op, value
		default:
			q := d.quote([]int{t.start, t.end})
			more := "fazla"
			if t.under {
				more = "az"
			}
			in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
				"%q bir tutar eşiği ama neyin eşiği olduğu belli değil. Müşterinin toplam harcamasıysa \"toplam harcaması %s TL'den %s\" yazın. "+
					"Teklifin şartıysa (hangi alışverişte geçerli olduğu) \"%s TL üzeri alışverişe ... indirim\" gibi yazın; bu kitleyi değil teklifi sınırlar.",
				q, t.number, more, t.number))
		}
		blank(t.start, t.end)
	}
	if len(distinct(minSpends)) > 1 {
		in.block(segment.StatusNeedsClarification, "", "Brief'te birden fazla alışveriş alt limiti var. Teklifin tek bir alt limiti olabilir; hangisini istediğinizi yazın.")
	}
	if len(minSpends) > 0 {
		return minSpends[0]
	}
	return 0
}

func classifyThreshold(t threshold, before, after string) spanKind {
	if explicitSpendRe.MatchString(before) || explicitSpendRe.MatchString(after[:min(len(after), 40)]) {
		return spanSpend
	}
	if t.under {
		return spanAmbiguous
	}
	if audAfterRe.MatchString(after) {
		return spanAmbiguous
	}
	if shopAfterRe.MatchString(after) {
		return spanTerm
	}
	if t.form == "lik" { // "100 TL'lik indirim kuponu": the amount IS the discount
		return spanIgnore
	}
	return spanAmbiguous
}

// analyzeDates reads the offer's last day. Every date in the brief must be
// accounted for: an impossible date, a day-and-month without a year, two
// different dates, or a date that is not said to be a deadline are all asked
// about instead of being dropped.
func (in *Intent) analyzeDates(d *doc, f string, blank func(a, b int)) string {
	matches := offer.FindDates(f)
	var valid []string
	for _, m := range matches {
		blank(m.Start, m.End)
		q := d.quote([]int{m.Start, m.End})
		if !m.Valid {
			in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
				"%q geçerli bir tarih değil. Gün, ay ve yılı gerçek bir tarih olarak yazın (gün.ay.yıl).", q))
			continue
		}
		valid = append(valid, m.ISO)
	}
	for _, span := range offer.FindDayMonths(f) {
		blank(span[0], span[1])
		q := d.quote(span[:])
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"%q için yıl yazılmamış. Son kullanma tarihini yılıyla yazın; örneğin \"%s %d\".", q, q, time.Now().Year()))
	}
	uniq := map[string]bool{}
	var distinctDates []string
	for _, v := range valid {
		if !uniq[v] {
			uniq[v] = true
			distinctDates = append(distinctDates, v)
		}
	}
	switch {
	case len(distinctDates) == 0:
		return ""
	case len(distinctDates) > 1:
		var shown []string
		for _, v := range distinctDates {
			shown = append(shown, offer.FormatDate(v))
		}
		in.block(segment.StatusNeedsClarification, strings.Join(shown, ", "), fmt.Sprintf(
			"Brief'te birden fazla tarih var (%s). Kuponun tek bir son kullanma tarihi olabilir; hangisini istediğinizi yazın.", strings.Join(shown, " ve ")))
		return ""
	case !expiryCueRe.MatchString(f):
		shown := offer.FormatDate(distinctDates[0])
		in.block(segment.StatusNeedsClarification, shown, fmt.Sprintf(
			"Brief'te %s tarihi geçiyor ama ne için olduğu belli değil. Kuponun son kullanma tarihiyse \"son kullanım %s\" diye yazın.", shown, shown))
		return ""
	}
	return distinctDates[0]
}

// analyzeAudience decides who receives the campaign when the brief names no
// condition. "Herkese / tüm müşterilere" is an explicit audience (every
// customer who may be contacted); saying nothing is a question, never an empty
// rule that would select everyone by accident.
func (in *Intent) analyzeAudience(f string) {
	if in.HasSpend || len(in.Included)+len(in.Excluded) > 0 || in.Cart || in.NotPurchased || in.Purchased || in.HasWindow || in.Gender != "" {
		return
	}
	if spendWordRe.MatchString(f) {
		if len(in.Blockers) == 0 {
			in.block(segment.StatusNeedsClarification, "", "Harcama tutarına göre bir kitle istiyorsunuz ama tutar yazılmamış. Örneğin \"toplam harcaması 1000 TL'den fazla olan müşteriler\" yazın.")
		}
		return
	}
	if everyoneRe.MatchString(f) || in.BothGenders {
		in.Everyone = true
		return
	}
	// A special day names a theme, not an audience: when nothing else narrows it,
	// the campaign goes to everyone who may be contacted, and the screen says so.
	if in.Occasion != nil && len(in.Blockers) == 0 {
		in.Everyone, in.AudienceDefaulted = true, true
		return
	}
	if len(in.Blockers) == 0 {
		in.Blockers = append(in.Blockers, Blocker{Status: segment.StatusNeedsClarification, Message: WhoQuestion, Suggestion: segment.SuggestAllCustomers})
	}
}

// WhoQuestion is asked when the brief does not say who should receive the campaign.
const WhoQuestion = "Bu kampanya kime gönderilsin? Tüm müşterilere mi?"

var digitsRe = regexp.MustCompile(`\d+`)

// unitWord writes a folded unit ("gun", "haftada") the way a person would.
func unitWord(u string) string {
	switch {
	case strings.HasPrefix(u, "gun"):
		return "gün"
	case strings.HasPrefix(u, "hafta"):
		return "hafta"
	case strings.HasPrefix(u, "ay"):
		return "ay"
	}
	return "yıl"
}

func distinct(v []float64) []float64 {
	var out []float64
	for _, x := range v {
		dup := false
		for _, y := range out {
			if x == y {
				dup = true
			}
		}
		if !dup {
			out = append(out, x)
		}
	}
	return out
}

var codeTokenRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{3,31}$`)

// findCoupon looks for a token that is written like a code (UPPERCASE, or
// letters with digits) right next to the word "kod" or "kupon".
func findCoupon(text string) string {
	raw := strings.Fields(text)
	tokens := make([]string, len(raw))
	for i, t := range raw {
		if k := strings.IndexAny(t, "'’"); k >= 0 {
			t = t[:k]
		}
		tokens[i] = strings.Trim(t, ".,;:!?()[]\"“”")
	}
	next := func(i int) bool {
		if i < 0 || i >= len(tokens) {
			return false
		}
		f := trtext.Fold(tokens[i])
		return strings.HasPrefix(f, "kod") || strings.HasPrefix(f, "kupon")
	}
	for i, t := range tokens {
		if !codeTokenRe.MatchString(t) || (!next(i-1) && !next(i+1)) {
			continue
		}
		if f := trtext.Fold(t); strings.HasPrefix(f, "kod") || strings.HasPrefix(f, "kupon") {
			continue
		}
		hasDigit := strings.ContainsAny(t, "0123456789")
		if !hasDigit && t != strings.ToUpper(t) {
			continue
		}
		if _, isCity := segment.CanonicalCity(t); isCity {
			continue
		}
		return strings.ToUpper(t)
	}
	return ""
}

var cityConnectors = map[string]bool{"ve": true, "veya": true, "ile": true}
var citySuffixes = map[string]bool{"da": true, "de": true, "ta": true, "te": true, "daki": true, "deki": true, "taki": true, "teki": true, "dan": true, "den": true, "li": true, "lu": true}
var excludeWords = map[string]bool{"haric": true, "haricinde": true, "haricindeki": true, "harici": true, "disinda": true, "disindaki": true}
var tokenRe = regexp.MustCompile(`[a-z0-9]+`)

func (in *Intent) analyzeCities(f string) {
	tokens := tokenRe.FindAllString(f, -1)
	cityOf := func(tok string) (string, bool) {
		for _, c := range segment.KnownCities {
			cf := trtext.Fold(c)
			if strings.HasPrefix(tok, cf) && utf8.RuneCountInString(tok)-utf8.RuneCountInString(cf) <= 8 {
				return c, true
			}
		}
		return "", false
	}
	var run []string
	flush := func(next string) {
		if len(run) == 0 {
			return
		}
		if excludeWords[next] {
			in.Excluded = append(in.Excluded, run...)
		} else {
			in.Included = append(in.Included, run...)
		}
		run = nil
	}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if c, ok := cityOf(tok); ok {
			run = append(run, c)
			continue
		}
		if len(run) > 0 && (cityConnectors[tok] || citySuffixes[tok]) {
			continue
		}
		flush(tok)
	}
	flush("")
	in.Included, in.Excluded = orderCities(in.Included), orderCities(in.Excluded)
}

// orderCities removes duplicates and sorts into KnownCities order.
func orderCities(list []string) []string {
	set := map[string]bool{}
	for _, c := range list {
		set[c] = true
	}
	var out []string
	for _, c := range segment.KnownCities {
		if set[c] {
			out = append(out, c)
		}
	}
	return out
}

func (in *Intent) analyzePurchase(d *doc, f string, work []rune) {
	first := func(res []*regexp.Regexp) (string, bool) {
		found := ""
		ok := false
		for _, re := range res {
			for _, loc := range re.FindAllStringIndex(f, -1) {
				if !ok {
					found, ok = d.quote(loc), true
				}
				blank(work, d, loc)
			}
		}
		return found, ok
	}
	var abandonPhrase, boughtPhrase string
	var abandon, notBought, bought bool
	abandonPhrase, abandon = first(abandonRes)
	in.NegationPhrase, notBought = first(notBoughtRes)
	boughtPhrase, bought = first(boughtRes)

	in.Cart = cartRe.MatchString(maskSpans(f, in.consumed))
	if abandon {
		in.Abandoned, in.Cart, in.NotPurchased = true, true, true
		if in.NegationPhrase == "" {
			in.NegationPhrase = abandonPhrase
		}
	}
	if notBought {
		in.NotPurchased = true
	}
	if bought {
		in.Purchased = true
	}
	if in.Purchased && in.NotPurchased {
		in.block(segment.StatusNeedsClarification, boughtPhrase, fmt.Sprintf(
			"Brief hem satın alanları (%q) hem de satın almayanları (%q) tarif ediyor. Hangisini istediğinizi netleştirin.", boughtPhrase, in.NegationPhrase))
	}

	// Anything that still looks like "people who did not X" and is not a phrase
	// we know is a condition we would have to drop: ask instead.
	scan := string(work)
	for _, loc := range unknownNegationRe.FindAllStringIndex(scan, -1) {
		q := d.quote(loc)
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"%q ifadesini bir kitle koşuluna çeviremedim (yalnızca sepet ve sipariş davranışı destekleniyor). Bu kısmı sessizce atlamadım; lütfen yeniden yazın veya kaldırın.", q))
	}
}

// crossChecks reports combinations that cannot be expressed.
func (in *Intent) crossChecks() {
	switch {
	case in.HasWindow && !in.Cart && !in.Purchased && !in.NotPurchased:
		in.block(segment.StatusNeedsClarification, in.WindowPhrase, fmt.Sprintf(
			"%q neye uygulanacak? Zaman aralığı sepete ekleme veya sipariş ile birlikte yazılmalı; örneğin \"%s sepete ürün ekleyip sipariş vermemiş\".", in.WindowPhrase, in.WindowPhrase))
	}
}

// ---- outcome ---------------------------------------------------------------

// Blocked returns the unsupported / needs_clarification answer when the brief
// contains something that cannot be turned into a rule.
func (in Intent) Blocked() (segment.Rule, bool) {
	if len(in.Blockers) == 0 {
		return segment.Rule{}, false
	}
	status := segment.StatusNeedsClarification
	for _, b := range in.Blockers {
		if b.Status == segment.StatusUnsupported {
			status = segment.StatusUnsupported
		}
	}
	var msgs []string
	for _, b := range in.Blockers {
		if b.Status == status {
			msgs = append(msgs, b.Message)
		}
	}
	rule := segment.Rule{Status: status, Conditions: []segment.Condition{}, Offer: offer.Offer{Kind: offer.KindNone}}
	for _, b := range in.Blockers {
		if b.Status == status && b.Suggestion != "" {
			// "Who should get this?" is the only open question: keep the offer that was
			// understood, so the screen can show it next to the question.
			rule.Suggestion, rule.Offer = b.Suggestion, in.Offer.Normalize()
		}
	}
	if status == segment.StatusUnsupported {
		rule.Explanation = clip(strings.Join(msgs, " ")+" Kitleyi şehir, sepete ekleme zamanı ve sipariş geçmişiyle tanımlayın.", segment.MaxExplanation)
	} else {
		rule.Question = clip(strings.Join(msgs, " "), segment.MaxQuestion)
	}
	return rule, true
}

func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// Rule builds the rule the brief describes. It is what a flawless model would
// answer, and it is what the smart mock answers.
func (in Intent) Rule() segment.Rule {
	if r, blocked := in.Blocked(); blocked {
		return r
	}
	var conds []segment.Condition
	var parts []string
	add := func(field, op string, v any) {
		conds = append(conds, segment.Condition{Field: field, Op: op, Value: v})
	}

	if in.Everyone {
		add("all_customers", segment.OpEq, true)
		parts = append(parts, "tüm müşteriler (izin vermiş olanlar)")
	}
	if in.Gender != "" {
		add("gender", segment.OpEq, in.Gender)
		parts = append(parts, genderWords(in.Gender))
	}
	if in.HasSpend {
		add("total_spent", in.SpendOp, in.SpendValue)
		parts = append(parts, "toplam harcaması "+offer.FormatNumber(in.SpendValue)+" TL"+map[string]string{segment.OpGt: "'den fazla", segment.OpGte: " ve üzeri", segment.OpLt: "'den az"}[in.SpendOp]+" olan müşteriler")
	}
	switch len(in.Included) {
	case 0:
	case 1:
		add("city", segment.OpEq, in.Included[0])
		parts = append(parts, in.Included[0]+"'daki müşteriler")
	default:
		add("city", segment.OpIn, append([]string(nil), in.Included...))
		parts = append(parts, strings.Join(in.Included, ", ")+" şehirlerindeki müşteriler")
	}
	switch len(in.Excluded) {
	case 0:
	case 1:
		add("city", segment.OpNeq, in.Excluded[0])
		parts = append(parts, in.Excluded[0]+" dışındaki müşteriler")
	default:
		add("city", segment.OpNotIn, append([]string(nil), in.Excluded...))
		parts = append(parts, strings.Join(in.Excluded, ", ")+" dışındaki müşteriler")
	}

	switch {
	case in.Cart:
		if in.HasWindow {
			add("days_since_last_cart_add", segment.OpLte, in.WindowDays)
			parts = append(parts, fmt.Sprintf("son %d günde sepete ürün ekleyen", in.WindowDays))
		} else if !in.Abandoned && !in.NotPurchased && !in.Purchased {
			add("days_since_last_cart_add", segment.OpGte, 0)
			parts = append(parts, "sepete ürün eklemiş")
		}
		switch {
		case in.Abandoned:
			add("purchased_since_last_cart_add", segment.OpEq, false)
			parts = append(parts, "sepetini terk etmiş, sonrasında satın almamış")
		case in.NotPurchased:
			add("purchased_since_last_cart_add", segment.OpEq, false)
			parts = append(parts, "sonrasında satın almamış")
		case in.Purchased:
			add("purchased_since_last_cart_add", segment.OpEq, true)
			parts = append(parts, "sonrasında satın almış")
		}
	case in.NotPurchased && in.HasWindow:
		// "No order in the last N days": never having ordered counts too.
		add("days_since_last_order_or_never", segment.OpGt, in.WindowDays)
		parts = append(parts, fmt.Sprintf("son %d günde sipariş vermemiş (hiç sipariş vermemişler dahil)", in.WindowDays))
	case in.NotPurchased:
		add("order_count", segment.OpEq, 0)
		parts = append(parts, "hiç sipariş vermemiş")
	case in.Purchased && in.HasWindow:
		add("days_since_last_order", segment.OpLte, in.WindowDays)
		parts = append(parts, fmt.Sprintf("son %d günde sipariş veren", in.WindowDays))
	case in.Purchased:
		add("order_count", segment.OpGt, 0)
		parts = append(parts, "en az bir sipariş vermiş")
	}

	if len(conds) == 0 {
		return segment.Rule{
			Status:     segment.StatusNeedsClarification,
			Conditions: []segment.Condition{},
			Question:   WhoQuestion,
			Suggestion: segment.SuggestAllCustomers,
			Offer:      offer.Offer{Kind: offer.KindNone},
		}
	}
	explanation := strings.Join(parts, ", ")
	if in.Abandoned && strings.Contains(trtext.Fold(in.NegationPhrase), "tamamla") {
		explanation += " (ödeme adımı verimizde ayrı tutulmuyor; \"ödemeyi tamamlamamış\", sepete ekleyip satın almamış olarak yorumlandı)"
	}
	rule := segment.Rule{Status: segment.StatusOK, Match: segment.MatchAll, Conditions: conds, Explanation: explanation, Offer: in.Offer}
	if in.AudienceDefaulted {
		rule.Note = AudienceDefaultedNote
	}
	return rule
}

// maskSpans blanks the given byte spans of s (same length), so later readers do
// not see words that were already explained.
func maskSpans(s string, spans [][2]int) string {
	if len(spans) == 0 {
		return s
	}
	b := []byte(s)
	for _, sp := range spans {
		for i := sp[0]; i < sp[1] && i < len(b); i++ {
			b[i] = ' '
		}
	}
	return string(b)
}
