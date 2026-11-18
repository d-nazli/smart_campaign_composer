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

func TestEachLocaleIsReallyThatLanguage(t *testing.T) {
	policy, _ := content.NewPolicy(nil)
	set := writeAll(t, policy, "Sepet hatırlatması", offer.Offer{Kind: offer.KindPercent, Value: 30})
	for code, want := range map[string]string{
		"tr-TR": "%30 indirim", "en-GB": "30% off", "de-DE": "30 % Rabatt", "fr-FR": "30 % de réduction", "es-MX": "30 % de descuento",
		"it-IT": "30% di sconto", "nl-NL": "30% korting", "pt-BR": "30% de desconto", "ar-SA": "خصم 30%",
	} {
		if !strings.Contains(set[code].SMSBody, want) {
			t.Errorf("%s: %s should contain %q", code, set[code].SMSBody, want)
		}
	}
}

// The translation follows the Turkish text as it is now, not the brief: a
// number, a code, a date changed by hand, a sentence removed or added (from
// the bank) changes every language. A sentence the bank does not know is left
// out rather than shown in Turkish; the facts in it are kept.
func TestAnEditedSourceChangesEveryTranslation(t *testing.T) {
	p, _ := content.NewPolicy(nil)
	o := offer.Offer{Kind: offer.KindPercent, Value: 30, Code: "ANNELER", MinSpend: 1000, ExpiresAt: "2026-05-10"}
	b := "Anneler Günü'ne özel kampanya, samimi olsun"
	m := mock.New(0)
	set := writeAll(t, p, b, o)
	src := set["tr-TR"]

	edited := src
	r := strings.NewReplacer("%30", "%45", "ANNELER", "BAHAR45", "10.05.2026", "15.06.2026", "1000 TL", "1500 TL")
	edited.SMSBody = r.Replace(src.SMSBody)
	edited.EmailSubject = r.Replace(src.EmailSubject)
	edited.EmailBody = r.Replace(src.EmailBody) + "\n\nGörüşmek üzere!\n\nYarın mağazamız kapalı."
	for _, l := range targets {
		c := translateChecked(t, m, p, edited, l, b, o)
		before := set[l.Code]
		all := c.SMSBody + "\n" + c.EmailSubject + "\n" + c.EmailBody
		for _, want := range []string{"45", "BAHAR45", "15.06.2026", "1500"} {
			if !strings.Contains(all, want) {
				t.Errorf("%s: the edited %q is not in the translation:\n%s", l.Code, want, all)
			}
		}
		for _, old := range []string{"ANNELER", "10.05.2026", "1000 TL"} {
			if strings.Contains(all, old) {
				t.Errorf("%s: the old %q is still in the translation:\n%s", l.Code, old, all)
			}
		}
		if c.EmailBody == before.EmailBody || len(c.EmailBody) <= len(before.EmailBody) {
			t.Errorf("%s: the added sentence is not translated:\n%s", l.Code, c.EmailBody)
		}
	}
	en := translateChecked(t, m, p, edited, enGB, b, o)
	if !strings.HasSuffix(en.EmailBody, "\n\nSee you soon!") {
		t.Errorf("the added bank sentence is translated, the unknown one left out:\n%s", en.EmailBody)
	}

	// A sentence removed from the source is removed from every language.
	cut := src
	cut.SMSBody = strings.TrimSpace(src.SMSBody[:strings.Index(src.SMSBody, "!")+1])
	for _, l := range targets {
		if c := translateChecked(t, m, p, cut, l, b, o); strings.Contains(c.SMSBody, "ANNELER") {
			t.Errorf("%s: the removed code is still in the SMS: %s", l.Code, c.SMSBody)
		}
	}
}

// A source the bank cannot read at all is never shown in Turkish: its facts
// stay, its words go; a language the bank does not have is an error.
func TestNoTurkishEverReachesAnotherLanguage(t *testing.T) {
	p, _ := content.NewPolicy(nil)
	free := content.Content{SMSBody: "{{first_name}}, bugün mağazada %45 indirimi kaçırma!", EmailSubject: "Yarın kapalıyız", EmailBody: "Merhaba {{first_name}},\n\nYarın mağazamız kapalı."}
	out, err := mock.New(0).Translate(context.Background(), llm.TranslateRequest{Source: free, SourceLocale: trTR, Target: frFR, Policy: p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var c content.Content
	_ = json.Unmarshal([]byte(out.Raw), &c)
	if w := turkishIn(c); w != "" || !strings.Contains(c.SMSBody, "{{first_name}}") || !strings.Contains(c.SMSBody, "%45") || c.EmailBody != "Bonjour {{first_name}}," {
		t.Fatalf("%q: %+v", w, c)
	}
	_, err = mock.New(0).Translate(context.Background(), llm.TranslateRequest{Source: free, SourceLocale: trTR, Policy: p,
		Target: content.Locale{Code: "ja-JP", Name: "Japanese (Japan)", Direction: "ltr"}}, nil)
	if err == nil {
		t.Fatal("a language outside the bank must be an error, never the Turkish text")
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
