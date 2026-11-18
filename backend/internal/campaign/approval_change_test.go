package campaign_test

import (
	"context"
	"testing"
	"time"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/clock"
	"smartcampaign/internal/llm/mock"
)

// The sending screen compares what it showed at approval with the deliveries
// really queued (campaign.View.Approval). These tests pin both numbers to their
// sources: the stored audience at approval, and the rows of the deliveries
// table over every variant and locale.

// A normal approval: what was shown is what was queued, so nothing to say.
func TestANormalApprovalHasNoDifference(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	ctx := context.Background()
	before, _ := svc.Get(ctx, id)
	if _, err := svc.Approve(ctx, id); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(ctx, id)
	sms, email, _ := deliveryCounts(t, pool, id)
	a := v.Approval
	if a == nil || a.Queued.SMS != sms || a.Queued.Email != email || sms == 0 {
		t.Fatalf("approval %+v, rows sms %d email %d", a, sms, email)
	}
	if a.Shown.SMS != before.Segment.Audience.SMSReachable || a.Shown.Email != before.Segment.Audience.EmailReachable || a.Shown != a.Queued {
		t.Fatalf("shown %+v, screen had %+v, queued %+v", a.Shown, before.Segment.Audience, a.Queued)
	}
}

// Consent withdrawn between the count on screen and the approval: fewer SMS
// are queued, and the difference is exactly the rows that are missing.
func TestConsentWithdrawnBeforeApprovalShowsTheRealDifference(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	ctx := context.Background()
	before, _ := svc.Get(ctx, id)
	if _, err := pool.Exec(ctx, `UPDATE users SET sms_consent=false WHERE sms_consent AND id % 2 = 0`); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Approve(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(ctx, id)
	sms, email, _ := deliveryCounts(t, pool, id)
	a := v.Approval
	shown := before.Segment.Audience
	if a == nil || a.Shown.SMS != shown.SMSReachable || a.Queued.SMS != sms || res.Queued.SMS != sms {
		t.Fatalf("approval %+v, result %+v, screen %+v, rows sms %d", a, res.Queued, shown, sms)
	}
	if lost := a.Shown.SMS - a.Queued.SMS; lost <= 0 || lost >= a.Shown.SMS {
		t.Fatalf("withdrawn consent must lose some SMS, not all: %+v", a)
	}
	if a.Shown.Email != a.Queued.Email || a.Queued.Email != email {
		t.Fatalf("e-mail did not change: %+v (rows %d)", a, email)
	}
}

// With Variant B and many locales the queued number is every row: the sum of
// the per-variant, per-locale counts, the same in the approval answer, in the
// campaign's progress and in the comparison with the screen.
func TestQueuedIsEveryVariantAndLocaleTogether(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	ctx := context.Background()
	res, err := svc.Approve(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT d.variant, u.locale, d.channel, count(*) FROM deliveries d JOIN users u ON u.id=d.user_id
		WHERE d.campaign_id=$1 GROUP BY 1, 2, 3`, id)
	if err != nil {
		t.Fatal(err)
	}
	var sum campaign.Queued
	variants, locales := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var variant, locale, channel string
		var n int
		if err := rows.Scan(&variant, &locale, &channel, &n); err != nil {
			t.Fatal(err)
		}
		variants[variant], locales[locale] = true, true
		if channel == "sms" {
			sum.SMS += n
		} else {
			sum.Email += n
		}
	}
	rows.Close()
	if len(variants) != 2 || len(locales) < 3 {
		t.Fatalf("variants %v, locales %v", variants, locales)
	}
	if *res.Queued != sum || res.Variants["A"].SMS+res.Variants["B"].SMS != sum.SMS || res.Variants["A"].Email+res.Variants["B"].Email != sum.Email {
		t.Fatalf("approval says %+v (variants %+v), rows %+v", res.Queued, res.Variants, sum)
	}
	v, _ := svc.Get(ctx, id)
	if v.Approval == nil || v.Approval.Queued != sum || v.Approval.Shown != sum {
		t.Fatalf("approval %+v, rows %+v", v.Approval, sum)
	}
	again, err := svc.Approve(ctx, id)
	if err != nil || !again.AlreadyApproved || *again.Queued != sum {
		t.Fatalf("repeat %v %+v", err, again)
	}
}

// A scheduled campaign queues nothing at approval (no 0 is reported) and keeps
// what the screen showed then; its start queues the rows it is compared with.
func TestAScheduledCampaignIsComparedWhenItStarts(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	at := inTwoHours()
	id := approvableCampaign(t, svc)
	before, _ := svc.Get(ctx, id)
	res, err := svc.ApproveAt(ctx, id, &at)
	if err != nil || res.Queued != nil {
		t.Fatalf("%v %+v", err, res)
	}
	if v, _ := svc.Get(ctx, id); v.Approval != nil {
		t.Fatalf("nothing to compare before the start: %+v", v.Approval)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET sms_consent=false WHERE sms_consent AND id % 3 = 0`); err != nil {
		t.Fatal(err)
	}
	if n, err := newScheduler(t, pool, clock.NewFake(at.Add(time.Minute))).RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("started %d, %v", n, err)
	}
	v, _ := svc.Get(ctx, id)
	sms, email, _ := deliveryCounts(t, pool, id)
	a := v.Approval
	if a == nil || a.Shown.SMS != before.Segment.Audience.SMSReachable || a.Queued.SMS != sms || a.Queued.Email != email || sms == 0 || a.Queued.SMS >= a.Shown.SMS {
		t.Fatalf("approval %+v, screen %+v, rows sms %d email %d", a, before.Segment.Audience, sms, email)
	}
	again, err := svc.Approve(ctx, id)
	if err != nil || again.Queued == nil || again.Queued.SMS != sms {
		t.Fatalf("repeat after the start: %v %+v", err, again)
	}
}
