package content

import (
	"strings"
	"testing"

	"smartcampaign/internal/offer"
)

// The offer of "1000 TL üzeri alışverişe 100 TL indirim, son kullanım 31.12.2026".
var offTerms = offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}

func termsContent() Content {
	return Content{
		SMSBody:      "{{first_name}}, 1000 TL ve üzeri alışverişte 100 TL indirim seni bekliyor! Son kullanım: 31.12.2026",
		EmailSubject: "{{first_name}}, 100 TL indirim fırsatı",
		EmailBody:    "Merhaba {{first_name}},\n\n1000 TL ve üzeri alışverişlerde 100 TL indirim seni bekliyor. Son kullanım: 31.12.2026\n\nGörüşmek üzere!",
	}
}

func TestQualityAcceptsMessagesThatStateTheTerms(t *testing.T) {
	p := mustPolicy(t, nil)
	if q := p.Quality(termsContent(), offTerms); len(q) != 0 {
		t.Fatalf("%+v", q)
	}
}

func TestQualityRequiresTheMinimumAndTheLastDayInBothChannels(t *testing.T) {
	p := mustPolicy(t, nil)
	for name, tc := range map[string]struct {
		edit func(*Content)
		want []string
	}{
		"sms: no minimum": {func(c *Content) {
			c.SMSBody = "{{first_name}}, 100 TL indirim seni bekliyor! Son kullanım: 31.12.2026"
		}, []string{FieldSMSBody + ":" + CodeOfferMissing}},
		"sms: no date": {func(c *Content) {
			c.SMSBody = "{{first_name}}, 1000 TL ve üzeri alışverişte 100 TL indirim seni bekliyor!"
		}, []string{FieldSMSBody + ":" + CodeOfferMissing}},
		"email: nothing of the terms": {func(c *Content) {
			c.EmailBody = "Merhaba {{first_name}},\n\n100 TL indirim seni bekliyor.\n\nGörüşmek üzere!"
		}, []string{FieldEmailBody + ":" + CodeOfferMissing}},
		"email: the terms may be in the subject": {func(c *Content) {
			c.EmailSubject = "1000 TL ve üzeri alışverişte 100 TL indirim, son kullanım 31.12.2026"
			c.EmailBody = "Merhaba {{first_name}},\n\nSeni bekliyoruz.\n\nGörüşmek üzere!"
		}, nil},
	} {
		c := termsContent()
		tc.edit(&c)
		got := issueCodes(p.Quality(c, offTerms))
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestQualityMessageNamesWhatIsMissing(t *testing.T) {
	p := mustPolicy(t, nil)
	c := termsContent()
	c.SMSBody = "{{first_name}}, 100 TL indirim seni bekliyor!"
	q := p.Quality(c, offTerms)
	if len(q) == 0 || !strings.Contains(q[0].Message, "1000 TL ve üzeri alışverişte") || !strings.Contains(q[0].Message, "31.12.2026") {
		t.Fatalf("the repair feedback must spell out the terms: %+v", q)
	}
}

func TestQualityFlagsADateThatIsNotTheLastDay(t *testing.T) {
	p := mustPolicy(t, nil)
	c := termsContent()
	c.SMSBody = "{{first_name}}, 1000 TL ve üzeri alışverişte 100 TL indirim! Son kullanım: 30.11.2026"
	q := p.Quality(c, offTerms)
	if !hasIssue(q, FieldSMSBody, CodeOfferConflict) || !hasIssue(q, FieldSMSBody, CodeOfferMissing) {
		t.Fatalf("a wrong date is both a conflict and a missing date: %+v", q)
	}

	// The brief gives no last day: a date in the text is an invention.
	noExpiry := offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000}
	c = termsContent()
	q = p.Quality(c, noExpiry)
	if !hasIssue(q, FieldSMSBody, CodeOfferConflict) || !hasIssue(q, FieldEmailBody, CodeOfferConflict) {
		t.Fatalf("invented date not flagged: %+v", q)
	}
}

func TestFallbackStatesTheTermsToo(t *testing.T) {
	p := mustPolicy(t, nil)
	for _, o := range []offer.Offer{
		offTerms,
		{Kind: offer.KindPercent, Value: 10, MinSpend: 1500.5},
		{Kind: offer.KindFreeShipping, ExpiresAt: "2027-01-05"},
		{Kind: offer.KindCouponCode, Code: "YAZ25", MinSpend: 500, ExpiresAt: "2026-12-31"},
		{Kind: offer.KindAmount, Value: 100, Code: "YAZ25", MinSpend: 1000, ExpiresAt: "2026-12-31"},
	} {
		fb := p.Fallback(o)
		if _, rep := p.Validate(fb); !rep.OK {
			t.Errorf("[%s] fallback breaks the limits: %+v", o.Describe(), rep.Issues)
		}
		if q := p.Quality(fb, o); len(q) != 0 {
			t.Errorf("[%s] fallback does not state the offer and its terms: %+v\n%+v", o.Describe(), q, fb)
		}
	}
}
