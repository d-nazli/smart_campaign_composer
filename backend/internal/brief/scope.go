package brief

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"smartcampaign/internal/segment"
	"smartcampaign/internal/trtext"
)

// Offer scope.
//
// "Kış ürünlerinde geçerli 200 TL indirim", "seçili ürünlerde %20 indirim",
// "ayakkabı kategorisinde %15", "spor ayakkabılarda ücretsiz kargo": the
// products are what the OFFER is valid on. They go into the messages
// (offer.scope) and never narrow the audience. A brief that names products
// and nobody else goes to everyone, as for a special day.
//
// Selecting PEOPLE by products ("kış ürünü almış müşteriler", "ayakkabı
// kategorisini görüntüleyenler") needs purchase or browsing history the
// database does not have: unsupported, said out loud.

// productWords are product names the reader knows (folded stems).
const productWords = `(?:ayakkabi|canta|elbise|pantolon|gomlek|tisort|ceket|kazak|giyim|telefon|bilgisayar|laptop|televizyon|kulaklik|parfum|makyaj|kozmetik|mobilya|oyuncak|kitap|elektronik|aksesuar)`

var (
	// scopeNounRe: the products in the locative ("… ürünlerinde", "…
	// kategorisinde", "ayakkabılarda").
	scopeNounRe = regexp.MustCompile(`\b(?:urun(?:ler|lerin|lerimiz|un|umuz)de|urun\s+grub(?:un|larin)da|kategori(?:sin|lerin|lerimiz)de|` + productWords + `(?:lar|ler)?(?:da|de|ta|te))\b(?:\s+gecerli\w*)?`)
	// offerFollowsRe: an incentive right after the products ("… ürünlerinde
	// 200 TL indirim"); without one the products are not an offer's scope.
	offerFollowsRe = regexp.MustCompile(`^[\s,]+(?:\S+\s+){0,2}(?:%|\d|yuzde\b|ucretsiz|bedava|indirim)`)
	// productHistoryRe: people chosen by what they bought or looked at.
	productHistoryRe = regexp.MustCompile(`\b(?:([a-z0-9]+)\s+)?(urun\w*|kategori\w*)\s+(?:satin\s+)?(?:al(?:mis|an|ip|dig)\w*|goruntule\w*|incele\w*|bak(?:an|mis)\w*|ziyaret\s+et\w*)`)
	plainWordRe      = regexp.MustCompile(`^[a-z]+$`)
	productStartRe   = regexp.MustCompile(`^` + productWords)
)

// Words that are not a product qualifier in front of "ürünlerinde".
var scopeStopWords = map[string]bool{
	"ve": true, "ile": true, "veya": true, "icin": true, "ozel": true, "tum": true, "butun": true, "her": true,
	"herkese": true, "herkes": true, "bu": true, "su": true, "o": true, "olan": true, "gecerli": true,
	"indirim": true, "indirimi": true, "kampanya": true, "kampanyasi": true, "firsat": true, "firsati": true,
	"bir": true, "baz": true, "bazi": true,
}

// Qualifiers that may stand in front of another one ("yeni sezon ürünleri").
var scopeModifiers = map[string]bool{
	"yeni": true, "eski": true, "secili": true, "belirli": true, "indirimli": true, "spor": true,
	"kis": true, "yaz": true, "ilkbahar": true, "sonbahar": true, "bahar": true,
	"kadin": true, "erkek": true, "cocuk": true, "bebek": true, "ev": true, "okul": true,
}

// Generic words in front of "ürün almış": "bir ürün almış" is any order
// (order_count), not product data.
var genericProduct = map[string]bool{"": true, "bir": true, "herhangi": true, "hic": true, "en": true, "az": true}

// analyzeScope reads the products the offer is valid on and reports people
// chosen by products. It returns the spans to blank, so no later reader takes
// "kategorisinde" for an unsupported category or "kadın" for a gender.
func (in *Intent) analyzeScope(d *doc, f string) [][2]int {
	var spans [][2]int
	for _, m := range productHistoryRe.FindAllStringSubmatchIndex(f, -1) {
		qual := ""
		if m[2] >= 0 {
			qual = f[m[2]:m[3]]
		}
		if strings.HasPrefix(f[m[4]:m[5]], "urun") && genericProduct[qual] {
			continue
		}
		q := d.quote(m[0:2])
		in.block(segment.StatusUnsupported, q, fmt.Sprintf(
			"Brief'te %q geçiyor: kişileri satın aldıkları ya da baktıkları ürünlere göre seçmek için ürün verisi gerekir; müşteri veritabanımızda ürün satın alma / görüntüleme geçmişi yok, bu yüzden kitleyi oluşturmadık. Ürünler indirimin kapsamıysa \"kış ürünlerinde geçerli %%20 indirim\" gibi yazın.", q))
		spans = append(spans, [2]int{m[0], m[1]})
	}

	for _, m := range scopeNounRe.FindAllStringIndex(f, -1) {
		tail := m[1]
		if !strings.Contains(f[m[0]:m[1]], "gecerli") && !offerFollowsRe.MatchString(f[tail:]) {
			continue
		}
		noun := strings.TrimSpace(f[m[0]:m[1]])
		if i := strings.Index(noun, " gecerli"); i >= 0 {
			noun = noun[:i]
		}
		start := m[0]
		fields := strings.Fields(f[:m[0]])
		qualified := false
		if n := len(fields); n > 0 && scopeQualifier(fields[n-1]) {
			start = strings.LastIndex(f[:m[0]], fields[n-1])
			qualified = true
			if n > 1 && scopeModifiers[fields[n-2]] {
				start = strings.LastIndex(f[:start], fields[n-2])
			}
		}
		spans = append(spans, [2]int{start, m[1]})
		if !qualified && !productStartRe.MatchString(noun) {
			continue // "ürünlerde %20": every product, nothing to say
		}
		if in.Scope == "" {
			in.Scope = scopeName(d.quote([]int{start, m[0] + len(noun)}))
		}
	}
	return spans
}

// scopeQualifier: a plain word that can name the products ("kış", "seçili").
func scopeQualifier(w string) bool {
	if !plainWordRe.MatchString(w) || len(w) < 2 || scopeStopWords[w] {
		return false
	}
	return !strings.HasSuffix(w, "lere") && !strings.HasSuffix(w, "lara") // "müşterilere": an audience
}

var inLocativeRe = regexp.MustCompile(`[iu]n(?:de|da)$`)

// scopeName turns the words of the brief into a noun phrase: "Kış ürünlerinde"
// → "kış ürünleri", "seçili ürünlerde" → "seçili ürünler", "ayakkabılarda" →
// "ayakkabılar".
func scopeName(phrase string) string {
	words := strings.Fields(strings.ToLowerSpecial(unicode.TurkishCase, phrase))
	if len(words) == 0 {
		return ""
	}
	last := []rune(words[len(words)-1])
	cut := 2
	if inLocativeRe.MatchString(trtext.Fold(string(last))) {
		cut = 3
	}
	if len(last) > cut {
		words[len(words)-1] = string(last[:len(last)-cut])
	}
	return strings.Join(words, " ")
}
