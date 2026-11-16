package campaign_test

import (
	"context"
	"testing"

	"smartcampaign/internal/llm/mock"
)

func TestListShowsEveryCampaignNewestFirstWithItsProgress(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()

	unsupported, err := svc.Create(ctx, "Adana'daki 35 yaş üstü müşterilere %10 indirim")
	if err != nil {
		t.Fatal(err)
	}
	sent := approvableCampaign(t, svc)
	res, err := svc.Approve(ctx, sent)
	if err != nil {
		t.Fatal(err)
	}
	ready := offerCampaign(t, svc)

	// Pretend the worker has been at work on the approved campaign.
	for _, q := range []string{
		`UPDATE deliveries SET status='sent', retries=2, rate_limited=1 WHERE id IN (SELECT id FROM deliveries WHERE campaign_id=$1 AND channel='sms' ORDER BY id LIMIT 4)`,
		`UPDATE deliveries SET status='failed', retries=1 WHERE id IN (SELECT id FROM deliveries WHERE campaign_id=$1 AND channel='sms' AND status='pending' ORDER BY id LIMIT 2)`,
		`UPDATE deliveries SET status='sent', rate_limited=1, retries=1 WHERE id IN (SELECT id FROM deliveries WHERE campaign_id=$1 AND channel='email' ORDER BY id LIMIT 3)`,
	} {
		if _, err := pool.Exec(ctx, q, sent); err != nil {
			t.Fatal(err)
		}
	}

	list, err := svc.List(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].ID != ready || list[1].ID != sent || list[2].ID != unsupported.ID {
		t.Fatalf("order/length: %+v", list)
	}

	// Never approved: no delivery block.
	if list[0].Delivery != nil || list[0].Status != "segment_ready" || list[0].SegmentStatus != "ok" || list[0].Matched == nil {
		t.Fatalf("segment_ready: %+v", list[0])
	}
	if list[2].Delivery != nil || list[2].SegmentStatus != "unsupported" || list[2].Matched != nil || list[2].Brief == "" {
		t.Fatalf("unsupported: %+v", list[2])
	}

	d := list[1].Delivery
	if d == nil || list[1].Status != "approved" {
		t.Fatalf("approved: %+v", list[1])
	}
	if d.SMS.Total != res.Queued.SMS || d.SMS.Sent != 4 || d.SMS.Failed != 2 || d.SMS.Pending != res.Queued.SMS-6 || d.SMS.Retries != 4*2+2 || d.SMS.RateLimited != 4 {
		t.Fatalf("sms: %+v", d.SMS)
	}
	if d.Email.Total != res.Queued.Email || d.Email.Sent != 3 || d.Email.Retries != 3 || d.Email.RateLimited != 3 || d.Done {
		t.Fatalf("email: %+v", d.Email)
	}

	// The detail view carries the same numbers.
	v, _ := svc.Get(ctx, sent)
	if v.Delivery.SMS.Retries != d.SMS.Retries || v.Delivery.Email.RateLimited != d.Email.RateLimited {
		t.Fatalf("detail %+v vs list %+v", v.Delivery, d)
	}
}

func TestListHonoursAndClampsItsLimit(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		offerCampaign(t, svc)
	}
	for limit, want := range map[int]int{1: 1, 2: 2, 100: 3, 0: 3, -5: 3} {
		list, err := svc.List(ctx, limit)
		if err != nil || len(list) != want {
			t.Errorf("limit %d: %d rows, err %v; want %d", limit, len(list), err, want)
		}
	}
}

func TestListOfNothingIsAnEmptyListNotNull(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	list, err := svc.List(context.Background(), 10)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("%#v %v", list, err)
	}
}

// Per language: how many messages go to each language and how many were sent.
// The numbers add up to the campaign's own totals and agree with the queue.
func TestDeliveryProgressCountsEachLanguage(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	id := approvableCampaign(t, svc)
	if _, err := svc.Approve(ctx, id); err != nil {
		t.Fatal(err)
	}
	// Some of every language are sent, the rest still wait.
	if _, err := pool.Exec(ctx, `
		UPDATE deliveries SET status='sent' WHERE id IN (
			SELECT d.id FROM deliveries d JOIN users u ON u.id=d.user_id WHERE d.campaign_id=$1 AND d.id % 3 = 0)`, id); err != nil {
		t.Fatal(err)
	}

	v, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	langs := v.Delivery.Languages
	if len(langs) != 3 {
		t.Fatalf("every language of the seeded audience is present: %+v", langs)
	}
	total, sent := 0, 0
	for code, p := range langs {
		var wantTotal, wantSent int
		if err := pool.QueryRow(ctx, `
			SELECT count(*), count(*) FILTER (WHERE d.status='sent')
			FROM deliveries d JOIN users u ON u.id=d.user_id WHERE d.campaign_id=$1 AND u.language=$2`, id, code).Scan(&wantTotal, &wantSent); err != nil {
			t.Fatal(err)
		}
		if p.Total != wantTotal || p.Sent != wantSent || p.Sent == 0 || p.Sent >= p.Total {
			t.Errorf("%s: %+v, want total %d sent %d (some but not all sent)", code, p, wantTotal, wantSent)
		}
		total, sent = total+p.Total, sent+p.Sent
	}
	if total != v.Delivery.SMS.Total+v.Delivery.Email.Total || sent != v.Delivery.SMS.Sent+v.Delivery.Email.Sent {
		t.Fatalf("languages add up to %d/%d, channels say %d/%d", sent, total,
			v.Delivery.SMS.Sent+v.Delivery.Email.Sent, v.Delivery.SMS.Total+v.Delivery.Email.Total)
	}

	list, err := svc.List(ctx, 10)
	if err != nil || len(list) != 1 || list[0].Delivery.Languages["tr"] != langs["tr"] {
		t.Fatalf("the list carries the same numbers: %v %+v", err, list)
	}
}

// A card shows who the campaign is for (in words) and how many people speak each language.
func TestListSummaryNamesTheAudience(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	ctx := context.Background()
	id := offerCampaign(t, svc)
	list, err := svc.List(ctx, 10)
	if err != nil || len(list) != 1 || list[0].ID != id {
		t.Fatalf("%v %+v", err, list)
	}
	if list[0].Explanation == "" {
		t.Fatal("the summary must say who the audience is")
	}
	sum := 0
	for _, n := range list[0].AudienceLanguages {
		sum += n
	}
	if len(list[0].AudienceLanguages) != 3 || sum == 0 {
		t.Fatalf("audience languages: %v", list[0].AudienceLanguages)
	}
}

// A scheduled campaign keeps its time in the list, also after it has started.
func TestListKeepsTheScheduledTimeAfterTheStart(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	at := inTwoHours()
	id := scheduled(t, svc, at)
	list, _ := svc.List(ctx, 10)
	if list[0].Status != "scheduled" || list[0].ScheduledAt == nil || !list[0].ScheduledAt.Equal(at) {
		t.Fatalf("%+v", list[0])
	}
	if _, err := pool.Exec(ctx, `UPDATE campaigns SET status='approved' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.List(ctx, 10)
	if list[0].ScheduledAt == nil || !list[0].ScheduledAt.Equal(at) {
		t.Fatalf("the planned time must stay on the card: %+v", list[0])
	}
}
