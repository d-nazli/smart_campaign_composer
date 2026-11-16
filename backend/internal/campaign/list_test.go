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
