package content

import (
	"strings"
	"testing"

	"smartcampaign/internal/offer"
)

func issueCodes(issues []Issue) []string {
	out := make([]string, 0, len(issues))
	for _, i := range issues {
		out = append(out, i.Field+":"+i.Code)
	}
	return out
}

func hasIssue(issues []Issue, field, code string) bool {
	for _, i := range issues {
		if i.Field == field && i.Code == code {
			return true
		}
	}
	return false
}

var (
	offPct15    = offer.Offer{Kind: offer.KindPercent, Value: 15}
	offTL100    = offer.Offer{Kind: offer.KindAmount, Value: 100}
	offShip     = offer.Offer{Kind: offer.KindFreeShipping}
	couponOffer = offer.Offer{Kind: offer.KindCouponCode, Code: "HOSGELDIN10"}
	offBoth     = offer.Offer{Kind: offer.KindPercent, Value: 15, Code: "YAZ25"}
)

func TestQualityOffer(t *testing.T) {
	tests := []struct {
		name  string
		offer offer.Offer
		c     Content
		want  []string // exact "field:code" list
	}{
		{"percent present everywhere", offPct15,
			Content{"{{first_name}}, %15 indirim seni bekliyor!", "{{first_name}}, %15 indirim", "Merhaba,\n\nSepetinde %15 indirim var."}, nil},
		{"'yüzde 15' and '15%' also count", offPct15,
			Content{"Yüzde 15 indirim seni bekliyor!", "15% indirim", "Merhaba."}, nil},
		{"offer only in the email body is enough for the email", offPct15,
			Content{"%15 indirim var!", "Sepetin seni bekliyor", "Sepetinde %15 indirim var."}, nil},
		{"SMS forgot the offer", offPct15,
			Content{"Sepetin seni bekliyor!", "%15 indirim", "Sepetinde %15 indirim var."}, []string{"sms_body:offer_missing"}},
		{"email forgot the offer", offPct15,
			Content{"%15 indirim var!", "Sepetin seni bekliyor", "Sepetin hâlâ burada."}, []string{"email_body:offer_missing"}},
		{"offBoth forgot the offer", offPct15,
			Content{"Sepetin seni bekliyor!", "Sepetin seni bekliyor", "Sepetin hâlâ burada."}, []string{"sms_body:offer_missing", "email_body:offer_missing"}},
		{"wrong percentage in SMS", offPct15,
			Content{"%30 indirim var!", "%15 indirim", "Sepetinde %15 indirim var."}, []string{"sms_body:offer_missing", "sms_body:offer_conflict"}},
		{"extra different percentage in the email", offPct15,
			Content{"%15 indirim var!", "%15 indirim", "%15 indirim, hatta bazı ürünlerde %50 indirim var."}, []string{"email_body:offer_conflict"}},
		{"amount present", offTL100,
			Content{"100 TL indirim seni bekliyor!", "100 TL indirim", "Sepetinde 100 TL indirim var."}, nil},
		{"amount with ₺ sign", offTL100,
			Content{"₺100 indirim seni bekliyor!", "₺100 indirim", "Sepetinde 100 ₺ indirim var."}, nil},
		{"wrong amount", offTL100,
			Content{"50 TL indirim seni bekliyor!", "100 TL indirim", "Sepetinde 100 TL indirim var."}, []string{"sms_body:offer_missing", "sms_body:offer_conflict"}},
		{"an invented worked example is a conflict", offer.Offer{Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300},
			Content{"Her 1000 TL alışverişe 100 TL indirim, toplamda en fazla 300 TL!", "Her 1000 TL'ye 100 TL indirim",
				"Her 1000 TL alışverişe 100 TL indirim, toplamda en fazla 300 TL. Örneğin 3000 TL'lik alışverişte 300 TL kazanırsın."}, []string{"email_body:offer_conflict"}},
		{"no offer in the brief: an invented amount is rejected", offer.Offer{Kind: offer.KindNone},
			Content{"Sepetin seni bekliyor!", "Sepetin seni bekliyor", "500 TL değerinde ürünlerin hâlâ burada."}, []string{"email_body:offer_conflict"}},
		{"free shipping present", offShip,
			Content{"Ücretsiz kargo fırsatı seni bekliyor!", "Kargo ücretsiz", "Sepetin için ücretsiz kargo var."}, nil},
		{"free shipping forgotten in SMS", offShip,
			Content{"Sepetin seni bekliyor!", "Kargo ücretsiz", "Sepetin için ücretsiz kargo var."}, []string{"sms_body:offer_missing"}},
		{"coupon code present, any case", couponOffer,
			Content{"Kodun: hosgeldin10", "HOSGELDIN10 ile indirim", "Kupon kodu HOSGELDIN10."}, nil},
		{"coupon code forgotten in SMS", couponOffer,
			Content{"Kuponun seni bekliyor!", "HOSGELDIN10 ile indirim", "Kupon kodu HOSGELDIN10."}, []string{"sms_body:offer_missing"}},
		{"percent and code: code forgotten", offBoth,
			Content{"%15 indirim var!", "%15 indirim", "%15 indirim için YAZ25 kodunu kullan."}, []string{"sms_body:offer_missing"}},
		{"no offer in the brief: nothing required", offer.Offer{Kind: offer.KindNone},
			Content{"Sepetin seni bekliyor!", "Sepetin seni bekliyor", "Sepetin hâlâ burada."}, nil},
		{"no offer in the brief: an invented discount is rejected", offer.Offer{Kind: offer.KindNone},
			Content{"%20 indirim var!", "Sepetin seni bekliyor", "Sepetin hâlâ burada."}, []string{"sms_body:offer_conflict"}},
	}
	p := mustPolicy(t, nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			norm, rep := p.Validate(tt.c)
			if !rep.OK {
				t.Fatalf("test content must satisfy the hard rules: %+v", rep.Issues)
			}
			got := issueCodes(p.Quality(norm, tt.offer))
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("issues = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQualityRepeatedWords(t *testing.T) {
	tests := []struct {
		name string
		sms  string
		want bool // a repeated_word issue on the SMS
	}{
		{"the reported one: 'özel fırsat fırsatından'", "Sayın {{first_name}}, sepetinizdeki ürünler sizi bekliyor. özel fırsat fırsatından yararlanın.", true},
		{"same word twice in a row", "Merhaba merhaba {{first_name}}, indirim seni bekliyor.", true},
		{"same stem one word apart", "Bu indirim ile indirimi kaçırma, sepetin seni bekliyor.", true},
		{"same stem across a sentence break is fine", "Sepetin seni bekliyor. Sepetini tamamlamak için tıkla.", false},
		{"placeholders do not count as words", "{{first_name}}, {{first_name}} için özel bir mesaj var.", false},
		{"short words are not compared", "Yeni ürünler yeniden stokta, hemen bak.", false},
		{"ordinary copy", "{{first_name}}, sepetindeki ürünler seni bekliyor! %15 indirim fırsatını kaçırma.", false},
		{"digits and symbols are ignored", "%15 indirim, %15 fırsat.", false},
	}
	p := mustPolicy(t, nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Content{SMSBody: tt.sms, EmailSubject: "Konu", EmailBody: "Gövde"}
			got := hasIssue(p.Quality(c, offer.Offer{Kind: offer.KindNone}), FieldSMSBody, CodeRepeatedWord)
			if got != tt.want {
				t.Errorf("repeated_word on SMS = %v, want %v (issues: %v)", got, tt.want, issueCodes(p.Quality(c, offer.Offer{})))
			}
		})
	}
}

func TestQualityRepeatedWordsAlsoCheckSubjectAndBody(t *testing.T) {
	p := mustPolicy(t, nil)
	c := Content{SMSBody: "Merhaba!", EmailSubject: "Fırsat fırsatı seni bekliyor", EmailBody: "Kampanya kampanyası başladı."}
	issues := p.Quality(c, offer.Offer{})
	if !hasIssue(issues, FieldEmailSubject, CodeRepeatedWord) || !hasIssue(issues, FieldEmailBody, CodeRepeatedWord) {
		t.Errorf("issues = %v", issueCodes(issues))
	}
}

func TestQualityMessagesNameTheProblemForTheModel(t *testing.T) {
	p := mustPolicy(t, nil)
	issues := p.Quality(Content{SMSBody: "Sepetin seni bekliyor!", EmailSubject: "Konu", EmailBody: "Gövde"}, offPct15)
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "%15 indirim") {
		t.Fatalf("the repair feedback must say what to write: %+v", issues)
	}
}

// The built-in template is what the person sees when the model fails. It must
// not lose the offer either.
func TestFallbackStatesTheOfferAndIsClean(t *testing.T) {
	p := mustPolicy(t, nil)
	for _, o := range []offer.Offer{{Kind: offer.KindNone}, offPct15, offTL100, offShip, couponOffer, offBoth} {
		fb := p.Fallback(o)
		norm, rep := p.Validate(fb)
		if !rep.OK {
			t.Errorf("%s: fallback violates the hard rules: %+v", o.Describe(), rep.Issues)
		}
		if q := p.Quality(norm, o); len(q) != 0 {
			t.Errorf("%s: fallback quality issues: %v\n%+v", o.Describe(), issueCodes(q), norm)
		}
	}
}
