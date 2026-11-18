package campaign_test

import (
	"context"
	"testing"
	"time"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/llm/mock"
)

// The busiest second is a sliding window over every call start: 101 calls
// inside one second read as 101 (the limit is broken), the same calls spread
// over two seconds read lower; the calls of each channel are counted apart.
func TestPeakPerSecondCountsCallsInAnySlidingSecond(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO campaigns (brief, status) VALUES ('b', 'completed') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 18, 12, 0, 0, 300_000_000, time.UTC) // starts mid-second: a fixed bucket would split it
	var burst, spread []time.Time
	for i := 0; i < 101; i++ {
		burst = append(burst, base.Add(time.Duration(i)*9*time.Millisecond)) // 101 calls in 0.9 s
		spread = append(spread, base.Add(time.Duration(i)*20*time.Millisecond))
	}
	for i, ch := range []string{"sms", "email"} {
		calls := burst
		if ch == "email" {
			calls = spread
		}
		// one delivery holds every call (retries are calls of one delivery too)
		if _, err := pool.Exec(ctx, `INSERT INTO deliveries (campaign_id, user_id, channel, status, call_starts)
			SELECT $1, id, $2, 'sent', $3::timestamptz[] FROM users ORDER BY id OFFSET $4 LIMIT 1`, id, ch, calls, i); err != nil {
			t.Fatal(err)
		}
	}
	peaks, err := campaign.PeakPerSecond(ctx, pool, id)
	if err != nil {
		t.Fatal(err)
	}
	if peaks["sms"] != 101 || peaks["email"] != 50 {
		t.Fatalf("peaks %v", peaks)
	}
	v, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Delivery.SMS.PeakPerSecond != 101 || v.Delivery.Email.PeakPerSecond != 50 {
		t.Fatalf("view %+v %+v", v.Delivery.SMS, v.Delivery.Email)
	}
}
