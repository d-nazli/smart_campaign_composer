package campaign_test

import (
	"context"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/campaign"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
)

// The briefs of the review, end to end with the mock (no API key): the
// audience, the note, the messages in every locale, and the approval, which
// parses the stored rule again.
func TestSpecialDayCombinedAndTieredBriefsGoAllTheWay(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	var everyone int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&everyone); err != nil {
		t.Fatal(err)
	}
	var women int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE gender='female'`).Scan(&women); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		brief   string
		matched int
		note    string
		offer   offer.Offer
	}{
		{"Kadınlar Günü'ne özel %20 indirim. Samimi olsun.", everyone, brief.AudienceDefaultedNote, offer.Offer{Kind: offer.KindPercent, Value: 20}},
		{"Anneler Günü için kadın müşterilere özel %15 indirim", women, "", offer.Offer{Kind: offer.KindPercent, Value: 15}},
		{"Her 1000 TL'lik alışverişe 100 TL indirim, toplamda en fazla 300 TL. Tüm müşterilere.", everyone, "",
			offer.Offer{Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300}},
		{"Babalar Günü'ne özel %20 indirim + ücretsiz kargo, son kullanım 21.06.2027", everyone, brief.AudienceDefaultedNote,
			offer.Offer{Kind: offer.KindPercent, Value: 20, ExpiresAt: "2027-06-21", Extras: []offer.Part{{Kind: offer.KindFreeShipping}}}},
	} {
		t.Run(tt.brief, func(t *testing.T) {
			v, err := svc.Create(ctx, tt.brief)
			if err != nil {
				t.Fatal(err)
			}
			seg := v.Segment
			if seg.Status != "ok" || seg.Audience == nil || seg.Audience.Matched != tt.matched {
				t.Fatalf("segment: %+v (audience %+v), want %d people", seg, seg.Audience, tt.matched)
			}
			if seg.Note != tt.note {
				t.Errorf("note = %q, want %q", seg.Note, tt.note)
			}
			if !seg.Offer.Equal(tt.offer) {
				t.Errorf("offer = %+v, want %+v", seg.Offer, tt.offer)
			}
			if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
				t.Fatal(err)
			}
			got, err := svc.Get(ctx, v.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Content == nil || got.Content.Source != campaign.SourceLLM {
				t.Fatalf("the mock's text must pass every check without the fallback: %+v", got.Content)
			}
			texts := map[string]campaign.ContentView{"tr-TR": *got.Content}
			for code, c := range got.Translations {
				texts[code] = *c
			}
			// The source states the offer; a translation says what the source says (no
			// mismatches below), in its own words ("livraison gratuite").
			for code, c := range texts {
				if !c.Report.OK || len(c.Report.Warnings) != 0 || (code == "tr-TR" && !tt.offer.Mentions(c.SMSBody)) {
					t.Errorf("%s: %+v", code, c)
				}
			}
			if len(got.Mismatches) != 0 || len(got.Pending) != 0 {
				t.Errorf("translations disagree or wait: %+v %v", got.Mismatches, got.Pending)
			}
			res, err := svc.Approve(ctx, v.ID)
			if err != nil {
				t.Fatalf("approve: %v", err)
			}
			if res.Queued.SMS == 0 || res.Queued.Email == 0 {
				t.Fatalf("nothing queued: %+v", res)
			}
		})
	}
}
