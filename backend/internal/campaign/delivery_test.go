package campaign_test

import (
	"context"
	"testing"

	"smartcampaign/internal/llm/mock"
)

func TestDeliveryProgressAppearsOnlyAfterApprovalAndFollowsTheQueue(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	id := approvableCampaign(t, svc)

	v, err := svc.Get(ctx, id)
	if err != nil || v.Delivery != nil {
		t.Fatalf("before approval: %+v %v", v.Delivery, err)
	}

	res, err := svc.Approve(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	v, _ = svc.Get(ctx, id)
	d := v.Delivery
	if d == nil || d.Done {
		t.Fatalf("after approval: %+v", d)
	}
	if d.SMS.Total != res.Queued.SMS || d.SMS.Pending != res.Queued.SMS || d.Email.Total != res.Queued.Email || d.Email.Pending != res.Queued.Email {
		t.Fatalf("progress %+v does not match the queue %+v", d, res.Queued)
	}

	// Pretend the worker made progress: 3 sent, 2 failed, 1 skipped, 1 in flight.
	for status, n := range map[string]int{"sent": 3, "failed": 2, "skipped": 1, "sending": 1} {
		if _, err := pool.Exec(ctx, `UPDATE deliveries SET status=$2 WHERE id IN
			(SELECT id FROM deliveries WHERE campaign_id=$1 AND channel='sms' AND status='pending' ORDER BY id LIMIT $3)`, id, status, n); err != nil {
			t.Fatal(err)
		}
	}
	v, _ = svc.Get(ctx, id)
	s := v.Delivery.SMS
	if s.Sent != 3 || s.Failed != 2 || s.Skipped != 1 || s.Pending != res.Queued.SMS-6 || s.Total != res.Queued.SMS {
		t.Fatalf("sms progress: %+v", s)
	}
	if v.Delivery.Done {
		t.Fatal("done while rows are pending")
	}

	// Everything final: done.
	if _, err := pool.Exec(ctx, `UPDATE deliveries SET status='sent' WHERE campaign_id=$1 AND status IN ('pending','sending')`, id); err != nil {
		t.Fatal(err)
	}
	v, _ = svc.Get(ctx, id)
	if !v.Delivery.Done || v.Delivery.SMS.Pending != 0 || v.Delivery.Email.Pending != 0 {
		t.Fatalf("finished: %+v", v.Delivery)
	}
}

// A campaign whose audience was empty is approved with nothing queued; its
// progress must say "done", not wait forever.
func TestDeliveryProgressOfAnEmptyApprovalIsDone(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	id := approvableCampaign(t, svc)
	if _, err := svc.Approve(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM deliveries WHERE campaign_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(ctx, id)
	if v.Delivery == nil || !v.Delivery.Done || v.Delivery.SMS.Total != 0 {
		t.Fatalf("%+v", v.Delivery)
	}
}
