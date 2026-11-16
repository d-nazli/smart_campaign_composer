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
package brief

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	NegationPhrase     string // what the person wrote for "did not buy"
	Offer              offer.Offer
	Blockers           []Blocker

	folded string // the folded brief, for checks that need to look at it again
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
	hoursRe       = regexp.MustCompile(`\bson\s+(?:` + num + `\s*)?(?:saat|dakika)\w*`)
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

	thresholdRe = regexp.MustCompile(`\d[\d.,]*\s*(?:tl|lira|₺)\s*(?:ve\s+)?(?:uzeri|ustu|altinda|alti|den\s+fazla|dan\s+fazla|den\s+az|dan\s+az)\w*`)
	wordPctRe   = regexp.MustCompile(`\byuzde\s+(` + numWords + `)\b`)
	kodRe       = regexp.MustCompile(`\bkod\w*`)
)

// What we cannot serve: the data model has no such column.
var unsupportedLexicon = []struct {
	label string
	re    *regexp.Regexp
}{
	{"yaş", regexp.MustCompile(`\b(?:yas|yasi|yasli\w*|yasinda\w*|yaslar\w*|genc\w*)\b`)},
	{"cinsiyet", regexp.MustCompile(`\b(?:cinsiyet\w*|kadin\w*|bayan\w*|erkek\w*|kiz\w*)\b`)},
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
	in := Intent{Offer: offer.Offer{Kind: offer.KindNone}, folded: d.foldString}
	f := d.foldString
	work := append([]rune(nil), d.fold...) // known phrases are blanked out of this copy

	in.analyzeUnsupported(d, f)
	in.analyzeTime(d, f)
	in.analyzeOffer(d, text)
	in.analyzeCities(f)
	in.analyzePurchase(d, f, work)
	in.crossChecks()
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
			in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
				"%q belirsiz. Kaç gün olduğunu sayıyla yazın; örneğin \"son 30 gün\" veya \"son 2 hafta\".", q))
		}
	}
	for _, loc := range hoursRe.FindAllStringIndex(f, -1) {
		q := d.quote(loc)
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"%q saat/dakika düzeyinde bir aralık; kitle yalnızca gün cinsinden tanımlanabiliyor. Örneğin \"son 1 gün\" yazın.", q))
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

func (in *Intent) analyzeOffer(d *doc, text string) {
	f := d.foldString
	// A spending threshold ("500 TL üzeri") is not an offer; it is a condition we cannot express.
	scan := f
	for _, loc := range thresholdRe.FindAllStringIndex(f, -1) {
		q := d.quote(loc)
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"%q bir tutar eşiği. Bunun toplam harcama mı yoksa tek bir sepet/sipariş tutarı mı olduğu belli değil ve sepet/sipariş tutarı kitle kuralında yok. Toplam harcamayı kastediyorsanız \"toplam harcaması 500 TL'den fazla\" yazın.", q))
	}
	scan = thresholdRe.ReplaceAllString(scan, " ")

	pcts := offer.Percents(scan)
	for _, m := range wordPctRe.FindAllStringSubmatch(scan, -1) {
		if n, ok := parseCount(m[1]); ok {
			pcts = append(pcts, float64(n))
		}
	}
	pcts = distinct(pcts)
	amts := distinct(offer.Amounts(scan))
	free := offer.HasFreeShipping(scan)
	code := findCoupon(text)

	var found []string
	for _, p := range pcts {
		found = append(found, "%"+offer.FormatNumber(p))
	}
	for _, a := range amts {
		found = append(found, offer.FormatNumber(a)+" TL")
	}
	if free {
		found = append(found, "ücretsiz kargo")
	}
	kinds := 0
	for _, present := range []bool{len(pcts) > 0, len(amts) > 0, free} {
		if present {
			kinds++
		}
	}
	switch {
	case kinds > 1 || len(pcts) > 1 || len(amts) > 1:
		in.block(segment.StatusNeedsClarification, strings.Join(found, ", "), fmt.Sprintf(
			"Brief'te birden fazla teklif var (%s). Mesajlarda yalnızca bir teklif kullanılabilir; hangisini istediğinizi yazın.", strings.Join(found, ", ")))
		return
	case len(pcts) == 1:
		in.Offer = offer.Offer{Kind: offer.KindPercent, Value: pcts[0], Code: code}
	case len(amts) == 1:
		in.Offer = offer.Offer{Kind: offer.KindAmount, Value: amts[0], Code: code}
	case free:
		in.Offer = offer.Offer{Kind: offer.KindFreeShipping, Code: code}
	case code != "":
		in.Offer = offer.Offer{Kind: offer.KindCouponCode, Code: code}
	}
	in.Offer = in.Offer.Normalize()
	if err := in.Offer.Validate(); err != nil {
		in.block(segment.StatusNeedsClarification, strings.Join(found, ", "), fmt.Sprintf(
			"Teklif geçersiz (%s): %v. Yüzde 1-100 arasında olmalı.", strings.Join(found, ", "), err))
		in.Offer = offer.Offer{Kind: offer.KindNone}
		return
	}
	if code == "" && kodRe.MatchString(f) {
		loc := kodRe.FindStringIndex(f)
		q := d.quote(loc)
		in.block(segment.StatusNeedsClarification, q, fmt.Sprintf(
			"Brief'te kupon %q geçiyor ama kodun kendisini bulamadım. Kupon kodunu yazar mısınız? (örneğin \"HOSGELDIN10 kodu\")", q))
	}
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

	in.Cart = cartRe.MatchString(f)
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
			Question:   "Mesajı kimlere göndermek istiyorsunuz? Örneğin bir şehir, sepet davranışı (sepete ekleyip satın almayanlar) veya sipariş geçmişi belirtebilirsiniz.",
			Offer:      offer.Offer{Kind: offer.KindNone},
		}
	}
	explanation := strings.Join(parts, ", ")
	if in.Abandoned && strings.Contains(trtext.Fold(in.NegationPhrase), "tamamla") {
		explanation += " (ödeme adımı verimizde ayrı tutulmuyor; \"ödemeyi tamamlamamış\", sepete ekleyip satın almamış olarak yorumlandı)"
	}
	return segment.Rule{Status: segment.StatusOK, Match: segment.MatchAll, Conditions: conds, Explanation: explanation, Offer: in.Offer}
}
