package offer

import (
	"strings"
	"testing"
)

var (
	free     = Part{Kind: KindFreeShipping}
	tiered   = Offer{Kind: KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300}
	combined = Offer{Kind: KindPercent, Value: 20, Extras: []Part{free}}
)

func TestCombinedAndTieredOffersAreValidated(t *testing.T) {
	ok := map[string]Offer{
		"percent + free shipping":         combined,
		"three parts":                     {Kind: KindPercent, Value: 10, Extras: []Part{{Kind: KindAmount, Value: 50}, free}},
		"tier with a cap":                 tiered,
		"tier without a cap":              {Kind: KindAmount, Value: 50, StepSpend: 500},
		"tier + free shipping":            {Kind: KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300, Extras: []Part{free}},
		"percent with a cap":              {Kind: KindPercent, Value: 20, MaxDiscount: 300},
		"free shipping first, as written": {Kind: KindFreeShipping, Extras: []Part{{Kind: KindPercent, Value: 20}}},
	}
	for name, o := range ok {
		if err := o.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := map[string]Offer{
		"same kind twice":                {Kind: KindPercent, Value: 20, Extras: []Part{{Kind: KindPercent, Value: 30}}},
		"four parts":                     {Kind: KindPercent, Value: 10, Extras: []Part{{Kind: KindAmount, Value: 5}, free, free}},
		"extras on nothing":              {Kind: KindNone, Extras: []Part{free}},
		"extras on a coupon":             {Kind: KindCouponCode, Code: "ABC10", Extras: []Part{free}},
		"coupon code as a part":          {Kind: KindPercent, Value: 10, Extras: []Part{{Kind: KindCouponCode}}},
		"bad extra value":                {Kind: KindPercent, Value: 10, Extras: []Part{{Kind: KindAmount, Value: -5}}},
		"tier on a percentage":           {Kind: KindPercent, Value: 10, StepSpend: 1000},
		"tier with a percentage extra":   {Kind: KindAmount, Value: 100, StepSpend: 1000, Extras: []Part{{Kind: KindPercent, Value: 5}}},
		"discount not below the step":    {Kind: KindAmount, Value: 1000, StepSpend: 1000},
		"cap on a fixed amount":          {Kind: KindAmount, Value: 100, MaxDiscount: 300},
		"cap below one step":             {Kind: KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 50},
		"cap on free shipping":           {Kind: KindFreeShipping, MaxDiscount: 300},
		"cap with percent and amount":    {Kind: KindPercent, Value: 10, MaxDiscount: 300, Extras: []Part{{Kind: KindAmount, Value: 50}}},
		"tier on nothing":                {Kind: KindNone, StepSpend: 1000},
		"negative step":                  {Kind: KindAmount, Value: 10, StepSpend: -1},
	}
	for name, o := range bad {
		if err := o.Validate(); err == nil {
			t.Errorf("%s: %+v should be invalid", name, o)
		}
	}
}

func TestPartOrderDoesNotMatter(t *testing.T) {
	a := Offer{Kind: KindFreeShipping, Extras: []Part{{Kind: KindPercent, Value: 20}}}
	if !a.Equal(combined) {
		t.Fatalf("%+v should equal %+v", a.Normalize(), combined.Normalize())
	}
	if n := a.Normalize(); n.Kind != KindPercent || len(n.Extras) != 1 || n.Extras[0].Kind != KindFreeShipping {
		t.Fatalf("not canonical: %+v", n)
	}
	if combined.Equal(Offer{Kind: KindPercent, Value: 20}) {
		t.Fatal("a missing part must make the offers different")
	}
	if tiered.Equal(Offer{Kind: KindAmount, Value: 100, MinSpend: 1000}) {
		t.Fatal("a tier is not a minimum basket")
	}
}

// Every part and every number of the terms must be in the message.
func TestMentionsNeedsEveryPartAndEveryNumber(t *testing.T) {
	for _, tt := range []struct {
		o    Offer
		text string
		want bool
	}{
		{combined, "%20 indirim + ücretsiz kargo", true},
		{combined, "%20 indirim", false},
		{combined, "ücretsiz kargo", false},
		{tiered, "Her 1000 TL alışverişe 100 TL indirim, toplamda en fazla 300 TL", true},
		{tiered, "100 TL indirim, toplamda en fazla 300 TL", false},
		{tiered, "Her 1000 TL alışverişe 100 TL indirim", false},
		{Offer{Kind: KindPercent, Value: 20, MaxDiscount: 300}, "%20 indirim (en fazla 300 TL)", true},
		{Offer{Kind: KindPercent, Value: 20, MaxDiscount: 300}, "%20 indirim", false},
	} {
		if got := tt.o.Mentions(tt.text); got != tt.want {
			t.Errorf("%s in %q: %v, want %v", tt.o.Describe(), tt.text, got, tt.want)
		}
	}
	// The promised percentage of a combined offer is not a conflict; another one is.
	if c := combined.Conflicts("%20 indirim + ücretsiz kargo"); len(c) != 0 {
		t.Errorf("conflicts: %v", c)
	}
	if c := (Offer{Kind: KindFreeShipping, Extras: []Part{{Kind: KindPercent, Value: 20}}}).Conflicts("%30 indirim"); len(c) != 1 {
		t.Errorf("a different percentage must conflict: %v", c)
	}
}

// The wording in every language states every number, so the language
// consistency check and Mentions work the same way everywhere.
func TestCombinedAndTieredWordingInEveryLanguage(t *testing.T) {
	cases := []struct {
		o    Offer
		want map[string]string
	}{
		{combined, map[string]string{"tr": "%20 indirim + ücretsiz kargo", "en": "20% off + free shipping", "de": "20 % Rabatt + kostenloser Versand"}},
		{tiered, map[string]string{
			"tr": "her 1000 TL alışverişe 100 TL indirim (toplamda en fazla 300 TL)",
			"en": "100 TL off every 1000 TL (up to 300 TL in total)",
			"de": "100 TL Rabatt je 1000 TL Einkaufswert (insgesamt höchstens 300 TL)"}},
		{Offer{Kind: KindPercent, Value: 10, Extras: []Part{{Kind: KindAmount, Value: 50}, free}}, map[string]string{
			"tr": "%10 indirim + ekstra 50 TL indirim + ücretsiz kargo", "en": "10% off + an extra 50 TL off + free shipping", "de": "10 % Rabatt + zusätzlich 50 TL Rabatt + kostenloser Versand"}},
		{Offer{Kind: KindPercent, Value: 20, MaxDiscount: 300}, map[string]string{"tr": "%20 indirim (en fazla 300 TL)", "en": "20% off (up to 300 TL)", "de": "20 % Rabatt (höchstens 300 TL)"}},
	}
	for _, c := range cases {
		for lang, want := range c.want {
			got := c.o.WhatIn(lang)
			if got != want {
				t.Errorf("%s: %q, want %q", lang, got, want)
			}
			if !c.o.Mentions(got) {
				t.Errorf("%s: %q does not state the offer it describes", lang, got)
			}
		}
	}
	if d := tiered.Describe(); !strings.Contains(d, "her 1000 TL") || !strings.Contains(d, "300 TL") {
		t.Errorf("describe: %q", d)
	}
}
