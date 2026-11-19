package campaign_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
)

// A translation reaches real people in their language: the coupon code they
// must type has to be in it exactly (LLM-3), and it has to be a translation,
// not the Turkish text again (LLM-4).

const couponBrief = "Son 30 günde sepete ürün ekleyip almayanlara YAZ30 koduyla %15 indirim gönder"

func contentJSON(t *testing.T, c content.Content) mock.Step {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return raw(string(b))
}

func hasCode(c content.Content, code string) (sms, email bool) {
	return strings.Contains(c.SMSBody, code), strings.Contains(c.EmailSubject+"\n"+c.EmailBody, code)
}

// lastFeedback is the repair feedback the latest translation call into code got.
func lastFeedback(m *mock.Client, code string) string {
	var out string
	for _, c := range m.Calls() {
		if c.Kind == "translate" && c.Locale == code && len(c.Previous) > 0 {
			out = strings.Join(c.Previous[len(c.Previous)-1].Problems, " ")
		}
	}
	return out
}

func TestATranslationThatLosesTheCouponCodeIsRepaired(t *testing.T) {
	t.Parallel()
	m := mock.NewScripted(nil, nil)
	svc, _ := newService(t, m)
	ctx := context.Background()
	v, err := svc.Create(ctx, couponBrief)
	if err != nil || v.Segment == nil || v.Segment.Offer.Code != "YAZ30" {
		t.Fatalf("setup: the brief must promise code YAZ30: %v %+v", err, v)
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	made, _ := svc.Get(ctx, v.ID)

	tests := []struct {
		name, locale string
		break_       func(string) string
		inSMS        bool // the broken text still has the code in the SMS
	}{
		{"the code translated (YAZ30 -> SUMMER30)", "en-GB", func(s string) string { return strings.ReplaceAll(s, "YAZ30", "SUMMER30") }, false},
		{"the code dropped from the email only", "de-DE", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			good := made.Translations[tt.locale].Content
			if s, e := hasCode(good, "YAZ30"); !s || !e {
				t.Fatalf("setup: the working translation has the code in both: %+v", good)
			}
			bad := good
			if tt.break_ != nil {
				bad = content.Content{SMSBody: tt.break_(good.SMSBody), EmailSubject: tt.break_(good.EmailSubject), EmailBody: tt.break_(good.EmailBody)}
			} else {
				bad.EmailSubject = strings.ReplaceAll(good.EmailSubject, "YAZ30", "")
				bad.EmailBody = strings.ReplaceAll(good.EmailBody, "YAZ30", "")
			}
			m.WithTranslation(tt.locale, contentJSON(t, bad), mock.Step{Kind: mock.Smart})

			after, err := svc.TranslateVariant(ctx, v.ID, campaign.VariantA, []string{tt.locale})
			if err != nil || len(after.TranslationFailures) != 0 {
				t.Fatalf("translate: %v %v", err, after.TranslationFailures)
			}
			if s, e := hasCode(after.Translations[tt.locale].Content, "YAZ30"); !s || !e {
				t.Fatalf("a translation without the code was stored: %+v", after.Translations[tt.locale].Content)
			}
			if fb := lastFeedback(m, tt.locale); !strings.Contains(fb, "YAZ30") {
				t.Fatalf("the feedback must name the code: %q", fb)
			}
		})
	}
}

func TestATranslationThatIsTheSourceTextIsRepaired(t *testing.T) {
	t.Parallel()
	m := mock.NewScripted(nil, nil)
	svc, _ := newService(t, m)
	ctx := context.Background()
	id := offerCampaign(t, svc)
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	made, _ := svc.Get(ctx, id)
	src := made.Content.Content
	m.WithTranslation("fr-FR", contentJSON(t, src), mock.Step{Kind: mock.Smart})

	after, err := svc.TranslateVariant(ctx, id, campaign.VariantA, []string{"fr-FR"})
	if err != nil || len(after.TranslationFailures) != 0 {
		t.Fatalf("translate: %v %v", err, after.TranslationFailures)
	}
	if got := after.Translations["fr-FR"].Content; got == src {
		t.Fatalf("the Turkish source was stored as the French translation: %+v", got)
	}
	if fb := lastFeedback(m, "fr-FR"); !strings.Contains(fb, "fr-FR") {
		t.Fatalf("the feedback must ask for the target language: %q", fb)
	}
}
