package mock_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
)

// The mock imitates a working model, so its text must pass the same checks the
// real model's text has to pass, in every language, for every kind of offer, and
// the languages must say the same thing.
func TestEveryLanguageOfTheMockIsValidAndConsistent(t *testing.T) {
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	offers := map[string]offer.Offer{
		"none":          {Kind: offer.KindNone},
		"percent":       {Kind: offer.KindPercent, Value: 30},
		"percent+code":  {Kind: offer.KindPercent, Value: 15, Code: "YAZ25"},
		"amount":        {Kind: offer.KindAmount, Value: 100},
		"free shipping": {Kind: offer.KindFreeShipping},
		"coupon only":   {Kind: offer.KindCouponCode, Code: "MERHABA10"},
		"terms":         {Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"},
		"terms+code":    {Kind: offer.KindPercent, Value: 30, Code: "ANNELER", MinSpend: 1000, ExpiresAt: "2026-05-10"},
		"coupon+terms":  {Kind: offer.KindCouponCode, Code: "YAZ25", MinSpend: 750, ExpiresAt: "2026-08-31"},
	}
	briefs := map[string]string{
		"plain":          "Tüm müşterilere kampanya duyurusu gönder.",
		"cart, informal": "Sepetini unutanlara samimi ve kısa bir hatırlatma gönder.",
		"cart, formal":   "Sepetinde ürün bırakan müşterilere hatırlatma gönder.",
	}
	m := mock.New(0)
	for oname, o := range offers {
		for bname, b := range briefs {
			t.Run(oname+"/"+bname, func(t *testing.T) {
				set := map[string]content.Content{}
				for _, l := range content.Languages {
					out, err := m.Messages(context.Background(), llm.MessageRequest{Brief: b, Offer: o, Policy: policy, Language: l.Code}, nil)
					if err != nil {
						t.Fatal(err)
					}
					var c content.Content
					if err := json.Unmarshal([]byte(out.Raw), &c); err != nil {
						t.Fatalf("%s: %v\n%s", l.Code, err, out.Raw)
					}
					norm, rep := policy.Validate(c)
					if !rep.OK {
						t.Fatalf("%s: %+v\n%+v", l.Code, rep.Issues, c)
					}
					if q := policy.Quality(norm, o); len(q) != 0 {
						t.Fatalf("%s: %+v\n%+v", l.Code, q, norm)
					}
					set[l.Code] = norm
				}
				if issues := content.Consistency(set); len(issues) != 0 {
					t.Fatalf("the languages disagree: %+v\n%+v", issues, set)
				}
			})
		}
	}
}

func TestEachLanguageIsReallyThatLanguage(t *testing.T) {
	policy, _ := content.NewPolicy(nil)
	o := offer.Offer{Kind: offer.KindPercent, Value: 30}
	m := mock.New(0)
	for lang, want := range map[string]string{"tr": "indirim", "en": "30% off", "de": "30 % Rabatt"} {
		out, err := m.Messages(context.Background(), llm.MessageRequest{Brief: "Sepet hatırlatması", Offer: o, Policy: policy, Language: lang}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.Raw, want) {
			t.Errorf("%s: %s should contain %q", lang, out.Raw, want)
		}
	}
}

// A script describes the default language; other languages are scripted
// separately and answered smartly otherwise.
func TestScriptsApplyToTheirOwnLanguageOnly(t *testing.T) {
	policy, _ := content.NewPolicy(nil)
	m := mock.NewScripted(nil, []mock.Step{{Kind: mock.Raw, Raw: `{"sms_body":"tr"}`}}).
		WithMessages("en", mock.Step{Kind: mock.Raw, Raw: `{"sms_body":"en"}`})
	ask := func(lang string) string {
		out, err := m.Messages(context.Background(), llm.MessageRequest{Brief: "b", Policy: policy, Language: lang}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return out.Raw
	}
	if ask("tr") != `{"sms_body":"tr"}` || ask("en") != `{"sms_body":"en"}` {
		t.Fatal("scripted answers")
	}
	if got := ask("de"); !strings.Contains(got, "email_subject") {
		t.Fatalf("an unscripted language is answered by the smart mock: %s", got)
	}
}
