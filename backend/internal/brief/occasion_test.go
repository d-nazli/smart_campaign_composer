package brief_test

import (
	"context"
	"strings"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// A special-day brief that names nobody goes to everyone, and the audience step
// says why. A brief that says "everyone" itself gets no note.
func TestASpecialDayWithoutAnAudienceGoesToEveryoneWithANote(t *testing.T) {
	for _, tt := range []struct {
		brief, note string
		occasion    string
	}{
		{"Kadınlar Günü'ne özel %20 indirim", brief.AudienceDefaultedNote, brief.OccasionWomensDay},
		{"Anneler Günü'ne özel ücretsiz kargo", brief.AudienceDefaultedNote, brief.OccasionMothersDay},
		{"Babalar Günü'ne özel %10 indirim, tüm müşterilere", "", brief.OccasionFathersDay},
		{"Kadınlar Günü'ne özel kadın müşterilere %20 indirim", "", brief.OccasionWomensDay},
	} {
		rule, err := brief.ResolveSegment(context.Background(), mock.New(0), tt.brief, llm.Hooks{})
		if err != nil || rule.Status != segment.StatusOK {
			t.Fatalf("%q: %v %+v", tt.brief, err, rule)
		}
		if rule.Note != tt.note {
			t.Errorf("%q: note = %q, want %q", tt.brief, rule.Note, tt.note)
		}
		if in := brief.Analyze(tt.brief); in.Occasion == nil || in.Occasion.ID != tt.occasion {
			t.Errorf("%q: occasion = %+v, want %s", tt.brief, in.Occasion, tt.occasion)
		}
	}
}

// The note comes from code: a model cannot add one, nor drop it.
func TestTheNoteIsDecidedByCode(t *testing.T) {
	invented := okRule(pct(10), c("city", "eq", "İstanbul"))
	invented.Note = "Model yazdı"
	m := mock.NewScripted([]mock.Step{rawRule(t, invented)}, nil)
	rule, err := brief.ResolveSegment(context.Background(), m, "İstanbul'daki müşterilere %10 indirim", llm.Hooks{})
	if err != nil || rule.Note != "" {
		t.Fatalf("note = %q, err %v", rule.Note, err)
	}
}

// What a real model gets wrong about special days, combined and tiered offers
// is caught by code and sent back with a precise hint; the repaired answer is
// the right one. These run the same Verify the real Claude answer goes through.
func TestBackendCatchesSpecialDayAndOfferMistakes(t *testing.T) {
	everyoneCond := c("all_customers", "eq", true)
	female := c("gender", "eq", "female")
	tests := []struct {
		name  string
		brief string
		wrong segment.Rule
		right offer.Offer
		conds string
		say   []string // words the repair feedback must contain
	}{
		{"special day became a gender condition", "Kadınlar Günü'ne özel %20 indirim",
			okRule(pct(20), female), pct(20), everyone, []string{"ozel gun", "gender"}},
		{"special day became a question about the audience", "Anneler Günü'ne özel %15 indirim",
			okRule(pct(15), c("city", "eq", "İstanbul")), pct(15), everyone, []string{"all_customers"}},
		{"explicit gender was dropped", "Kadınlar Günü'ne özel, kadın müşterilere %20 indirim",
			okRule(pct(20), everyoneCond), pct(20), "gender eq female", []string{"gender eq female"}},
		{"a part of a combined offer was dropped", "Herkese %20 indirim ve ücretsiz kargo",
			okRule(pct(20), everyoneCond), combo(pct(20), freePart), everyone, []string{"extras"}},
		{"the tier became a minimum basket", "Her 1000 TL'lik alışverişe 100 TL indirim, toplamda en fazla 300 TL. Tüm müşterilere.",
			okRule(withTerms(amountOf(100), 1000, ""), everyoneCond), tier(100, 1000, 300), everyone, []string{"step_spend", "max_discount"}},
		{"the cap was dropped", "Herkese %20 indirim, en fazla 300 TL",
			okRule(pct(20), everyoneCond), capped(pct(20), 300), everyone, []string{"max_discount"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mock.NewScripted([]mock.Step{rawRule(t, tt.wrong), {Kind: mock.Smart}}, nil)
			rule, err := brief.ResolveSegment(context.Background(), m, tt.brief, llm.Hooks{})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(conds(rule), "|"); rule.Status != segment.StatusOK || got != tt.conds || !rule.Offer.Equal(tt.right) {
				t.Fatalf("repaired rule: %s %v %+v", rule.Status, got, rule.Offer)
			}
			fb := feedback(t, m)
			for _, w := range tt.say {
				if !strings.Contains(fb, w) {
					t.Errorf("the repair feedback should mention %q: %s", w, fb)
				}
			}
		})
	}
}

// The order of the parts does not matter: a model that writes free shipping as
// the main part and the percentage as an extra promises the same offer.
func TestTheOrderOfTheOfferPartsDoesNotMatter(t *testing.T) {
	reversed := offer.Offer{Kind: offer.KindFreeShipping, Extras: []offer.Part{{Kind: offer.KindPercent, Value: 20}}}
	m := mock.NewScripted([]mock.Step{rawRule(t, okRule(reversed, c("all_customers", "eq", true)))}, nil)
	rule, err := brief.ResolveSegment(context.Background(), m, "Herkese %20 indirim ve ücretsiz kargo", llm.Hooks{})
	if err != nil || rule.Status != segment.StatusOK || len(m.Calls()) != 1 {
		t.Fatalf("%+v %v calls=%d", rule, err, len(m.Calls()))
	}
	if rule.Offer.Kind != offer.KindPercent || len(rule.Offer.Extras) != 1 || rule.Offer.Extras[0].Kind != offer.KindFreeShipping {
		t.Fatalf("not normalized: %+v", rule.Offer)
	}
}
