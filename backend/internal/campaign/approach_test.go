package campaign_test

import (
	"context"
	"strings"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
)

// sourceRetries records the problems sent back to the model for the source text.
func sourceRetries(out *[][]string) campaign.Events {
	return campaign.Events{Retry: func(code string, _ int, p []string) {
		if code == "tr-TR" {
			*out = append(*out, p)
		}
	}}
}

func containsProblem(retries [][]string, part string) bool {
	for _, r := range retries {
		for _, p := range r {
			if strings.Contains(p, part) {
				return true
			}
		}
	}
	return false
}

// Each variant takes an approach from the list, chosen for the brief, and B's
// is not A's (the mock reads a cart brief as "merak" first).
func TestEachVariantTakesItsOwnApproachFromTheList(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	v, err := svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	a, b := v.Content.Approach, v.VariantB.Content.Approach
	if a != content.ApproachCuriosity || !content.IsApproach(b) || a == b {
		t.Fatalf("approaches A %q B %q", a, b)
	}
	// A translation has no approach of its own.
	for code, tr := range v.Translations {
		if tr.Approach != "" {
			t.Fatalf("%s translation has approach %q", code, tr.Approach)
		}
	}
}

// An approach outside the list is a broken answer: the model is told and asked again.
func TestAnApproachOutsideTheListIsRejected(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.NewScripted(nil, []mock.Step{{Kind: mock.UnknownApproach}, {Kind: mock.Smart}}))
	id := offerCampaign(t, svc)
	var retries [][]string
	if err := svc.GenerateMessages(context.Background(), id, sourceRetries(&retries)); err != nil {
		t.Fatal(err)
	}
	if !containsProblem(retries, `approach "`+mock.UnknownApproachValue+`" listede yok`) {
		t.Fatalf("retries %v", retries)
	}
	if v, _ := svc.Get(context.Background(), id); !content.IsApproach(v.Content.Approach) || v.Content.Source != campaign.SourceLLM {
		t.Fatalf("stored %+v", v.Content)
	}
}

// A message without an approach is rejected like a missing field.
func TestAMessageWithoutAnApproachIsRejected(t *testing.T) {
	t.Parallel()
	noApproach := `{"sms_body":"{{first_name}}, %15 indirim seni bekliyor!","email_subject":"%15 indirim","email_body":"Merhaba {{first_name}}, %15 indirim var."}`
	svc, _ := newService(t, mock.NewScripted(nil, []mock.Step{raw(noApproach), {Kind: mock.Smart}}))
	id := offerCampaign(t, svc)
	var retries [][]string
	if err := svc.GenerateMessages(context.Background(), id, sourceRetries(&retries)); err != nil {
		t.Fatal(err)
	}
	if !containsProblem(retries, "approach alanı eksik") {
		t.Fatalf("retries %v", retries)
	}
}

// Variant B in Variant A's approach is rejected; the next answer takes another one.
func TestVariantBInTheSameApproachAsAIsRejected(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.NewScripted(nil, []mock.Step{{Kind: mock.Smart}, {Kind: mock.SameApproach}, {Kind: mock.Smart}}))
	id := offerCampaign(t, svc)
	ctx := context.Background()
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	var retries [][]string
	if err := svc.GenerateVariant(ctx, id, campaign.VariantB, sourceRetries(&retries)); err != nil {
		t.Fatal(err)
	}
	if len(retries) != 1 || !containsProblem(retries, "diğer varyantın yaklaşımıyla aynı") {
		t.Fatalf("retries %v", retries)
	}
	v, _ := svc.Get(ctx, id)
	if v.VariantB.Content.Approach == v.Content.Approach {
		t.Fatalf("both %q", v.Content.Approach)
	}
}

// A brief without a last day or a duration allows no urgency: neither the
// approach nor a sentence that hurries the reader.
func TestUrgencyWithoutADeadlineIsRejected(t *testing.T) {
	t.Parallel()
	urgent := msgs("{{first_name}}, %15 indirim seni bekliyor!", "%15 indirim", "Merhaba {{first_name}}, %15 indirim var.")
	urgent = strings.Replace(urgent, `"approach":"fayda"`, `"approach":"aciliyet"`, 1)
	hurried := msgs("{{first_name}}, son gün! %15 indirim seni bekliyor.", "%15 indirim", "Merhaba {{first_name}}, %15 indirim var. Acele et!")
	svc, _ := newService(t, mock.NewScripted(nil, []mock.Step{raw(urgent), raw(hurried), {Kind: mock.Smart}}))
	id := offerCampaign(t, svc) // "Son 30 günde ..." is an audience window, not a deadline
	var retries [][]string
	if err := svc.GenerateMessages(context.Background(), id, sourceRetries(&retries)); err != nil {
		t.Fatal(err)
	}
	if len(retries) != 2 || !containsProblem(retries[:1], `"aciliyet" seçilemez`) ||
		!containsProblem(retries[1:], `"son gün"`) || !containsProblem(retries[1:], `"acele et"`) {
		t.Fatalf("retries %v", retries)
	}
	if v, _ := svc.Get(context.Background(), id); v.Content.Approach == content.ApproachUrgency {
		t.Fatal("urgency stored without a deadline")
	}
}

// With a last day in the brief urgency is honest, and the mock picks it.
func TestADeadlineAllowsUrgency(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	v := createBrief(t, svc, "Tüm müşterilere 1000 TL üzeri alışverişe 100 TL indirim gönder, son kullanım 31.12.2099.")
	if err := svc.GenerateMessages(context.Background(), v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(context.Background(), v.ID)
	if got.Content.Approach != content.ApproachUrgency || got.Content.Source != campaign.SourceLLM {
		t.Fatalf("approach %q source %s", got.Content.Approach, got.Content.Source)
	}
}

// The two variants promise the same: B with another discount is rejected.
func TestVariantBMustPromiseWhatAPromises(t *testing.T) {
	t.Parallel()
	other := msgs("{{first_name}}, %20 indirim seni bekliyor!", "%20 indirim", "Merhaba {{first_name}}, %20 indirim var.")
	other = strings.Replace(other, `"approach":"fayda"`, `"approach":"kisisel"`, 1)
	svc, _ := newService(t, mock.NewScripted(nil, []mock.Step{{Kind: mock.Smart}, raw(other), {Kind: mock.Smart}}))
	id := offerCampaign(t, svc)
	ctx := context.Background()
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	var retries [][]string
	if err := svc.GenerateVariant(ctx, id, campaign.VariantB, sourceRetries(&retries)); err != nil {
		t.Fatal(err)
	}
	if !containsProblem(retries, "iki varyant aynı teklifi söylemeli") {
		t.Fatalf("retries %v", retries)
	}
}
