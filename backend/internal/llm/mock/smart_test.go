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
// real model's text has to pass, in every locale, for every kind of offer, and
// every translation must say what the source says.
func TestEveryLocaleOfTheMockIsValidAndConsistent(t *testing.T) {
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
	for oname, o := range offers {
		for bname, b := range briefs {
			t.Run(oname+"/"+bname, func(t *testing.T) { writeAll(t, policy, b, o) })
		}
	}
}

func TestEachPhraseBookLocaleIsReallyThatLanguage(t *testing.T) {
	policy, _ := content.NewPolicy(nil)
	set := writeAll(t, policy, "Sepet hatırlatması", offer.Offer{Kind: offer.KindPercent, Value: 30})
	for code, want := range map[string]string{"tr-TR": "indirim", "en-GB": "30% off", "de-DE": "30 % Rabatt"} {
		if !strings.Contains(set[code].SMSBody, want) {
			t.Errorf("%s: %s should contain %q", code, set[code].SMSBody, want)
		}
	}
}

// A locale without a phrase book gets the source text marked with its code:
// the facts and {{placeholders}} are the source's own, and a new locale needs
// no code. The mark is dropped where it would break a limit.
func TestALocaleWithoutAPhraseBookGetsTheMarkedSource(t *testing.T) {
	policy, _ := content.NewPolicy(nil)
	src := content.Content{SMSBody: "{{first_name}}, %30 indirim seni bekliyor!", EmailSubject: "%30 indirim", EmailBody: "Merhaba {{first_name}},\n\n%30 indirim."}
	out, err := mock.New(0).Translate(context.Background(), llm.TranslateRequest{Source: src, SourceLocale: trTR, Target: frFR, Policy: policy}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var c content.Content
	_ = json.Unmarshal([]byte(out.Raw), &c)
	if c.SMSBody != mock.PseudoPrefix("fr-FR")+src.SMSBody || c.EmailSubject != "[fr-FR] %30 indirim" || !strings.HasPrefix(c.EmailBody, "[fr-FR] Merhaba") {
		t.Fatalf("%+v", c)
	}
	long := src
	long.SMSBody = strings.Repeat("a", 155)
	out, _ = mock.New(0).Translate(context.Background(), llm.TranslateRequest{Source: long, SourceLocale: trTR, Target: frFR, Policy: policy}, nil)
	_ = json.Unmarshal([]byte(out.Raw), &c)
	if c.SMSBody != long.SMSBody {
		t.Fatalf("the mark would break 160: %q", c.SMSBody)
	}
}

// When the source was edited by hand, a phrase book cannot say the same thing
// (it writes from the offer): the mock then falls back to the marked copy.
func TestAPhraseBookThatDisagreesWithAnEditedSourceIsNotUsed(t *testing.T) {
	policy, _ := content.NewPolicy(nil)
	edited := content.Content{SMSBody: "{{first_name}}, bugün %45 indirim!", EmailSubject: "%45 indirim", EmailBody: "Merhaba,\n\n%45 indirim."}
	out, err := mock.New(0).Translate(context.Background(), llm.TranslateRequest{Source: edited, SourceLocale: trTR, Target: enGB, Policy: policy,
		Brief: "Herkese %30 indirim", Offer: offer.Offer{Kind: offer.KindPercent, Value: 30}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var c content.Content
	_ = json.Unmarshal([]byte(out.Raw), &c)
	if !strings.HasPrefix(c.SMSBody, "[en-GB] ") || len(content.ConsistentWith(trTR, edited, enGB, c)) != 0 {
		t.Fatalf("%+v", c)
	}
}

// A script describes the source text; translations are scripted per locale
// and answered smartly otherwise.
func TestScriptsApplyToTheirOwnLocaleOnly(t *testing.T) {
	policy, _ := content.NewPolicy(nil)
	m := mock.NewScripted(nil, []mock.Step{{Kind: mock.Raw, Raw: `{"sms_body":"tr"}`}}).
		WithTranslation("en-GB", mock.Step{Kind: mock.Raw, Raw: `{"sms_body":"en"}`})
	out, err := m.Messages(context.Background(), llm.MessageRequest{Brief: "b", Policy: policy, Locale: trTR}, nil)
	if err != nil || out.Raw != `{"sms_body":"tr"}` {
		t.Fatalf("source: %v %s", err, out.Raw)
	}
	tr := func(l content.Locale) string {
		out, err := m.Translate(context.Background(), llm.TranslateRequest{Source: content.Content{SMSBody: "x", EmailSubject: "y", EmailBody: "z"},
			SourceLocale: trTR, Target: l, Policy: policy}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return out.Raw
	}
	if tr(enGB) != `{"sms_body":"en"}` {
		t.Fatal("scripted translation")
	}
	if got := tr(deDE); !strings.Contains(got, "email_subject") {
		t.Fatalf("an unscripted locale is answered by the smart mock: %s", got)
	}
	calls := m.Calls()
	if calls[0].Kind != "messages" || calls[0].Locale != "tr-TR" || calls[1].Kind != "translate" || calls[1].Locale != "en-GB" {
		t.Fatalf("%+v", calls)
	}
}
