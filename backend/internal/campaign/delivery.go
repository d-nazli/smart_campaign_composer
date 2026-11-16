package campaign

import (
	"context"
	"fmt"
	"time"
)

// ChannelProgress is how far one channel has come. Pending includes the rows
// being sent right now and the ones waiting for a retry.
type ChannelProgress struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	// Retries counts how often deliveries went back to the queue; RateLimited
	// how many of those were HTTP 429s.
	Retries     int `json:"retries"`
	RateLimited int `json:"rate_limited"`
}

// DeliveryView is the sending progress of an approved campaign.
type DeliveryView struct {
	SMS   ChannelProgress `json:"sms"`
	Email ChannelProgress `json:"email"`
	// Done is true once nothing is pending in either channel.
	Done bool `json:"done"`
	// Failures groups the failed deliveries by why they failed (largest first).
	Failures []FailureReason `json:"failures,omitempty"`
}

// FailureReason is one group of failed deliveries. Reason is a stable code, not
// the raw provider text: "rejected" (the provider refused the recipient or the
// message, so a retry cannot help), "gave_up" (retries ran out), "too_old"
// (waited too long to be worth sending), "other".
type FailureReason struct {
	Channel string `json:"channel"`
	Reason  string `json:"reason"`
	Count   int    `json:"count"`
}

// Summary is one line of the campaign list (the dashboard).
type Summary struct {
	ID            int64         `json:"id"`
	Brief         string        `json:"brief"`
	Status        string        `json:"status"`
	SegmentStatus string        `json:"segment_status,omitempty"`
	Matched       *int          `json:"matched,omitempty"`
	Delivery      *DeliveryView `json:"delivery,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
}

// add folds one (channel, status) group into the view.
func (d *DeliveryView) add(channel, status string, n, retries, limited int) {
	p := &d.SMS
	if channel == "email" {
		p = &d.Email
	}
	p.Total += n
	p.Retries += retries
	p.RateLimited += limited
	switch status {
	case "sent":
		p.Sent += n
	case "failed":
		p.Failed += n
	case "skipped":
		p.Skipped += n
	default: // pending, sending
		p.Pending += n
	}
}

func (d *DeliveryView) finish() *DeliveryView {
	d.Done = d.SMS.Pending == 0 && d.Email.Pending == 0
	return d
}

// deliveryProgress counts one campaign's deliveries per channel and state.
func (s *Service) deliveryProgress(ctx context.Context, id int64) (*DeliveryView, error) {
	m, err := s.deliveryProgressFor(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	return m[id].finish(), nil
}

// deliveryProgressFor does the same for many campaigns in one query. Every id
// gets a view, even one without deliveries.
func (s *Service) deliveryProgressFor(ctx context.Context, ids []int64) (map[int64]*DeliveryView, error) {
	out := make(map[int64]*DeliveryView, len(ids))
	for _, id := range ids {
		out[id] = &DeliveryView{}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT campaign_id, channel, status, count(*), coalesce(sum(retries), 0)::int, coalesce(sum(rate_limited), 0)::int
		FROM deliveries WHERE campaign_id = ANY($1) GROUP BY 1, 2, 3`, ids)
	if err != nil {
		return nil, fmt.Errorf("delivery progress: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var channel, status string
		var n, retries, limited int
		if err := rows.Scan(&id, &channel, &status, &n, &retries, &limited); err != nil {
			return nil, fmt.Errorf("delivery progress: %w", err)
		}
		out[id].add(channel, status, n, retries, limited)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("delivery progress: %w", err)
	}
	rows.Close()
	return out, s.addFailures(ctx, ids, out)
}

// addFailures fills DeliveryView.Failures. The worker stores the last provider
// answer as free text; it is sorted into a few codes here so the screen can
// explain failures in plain words.
func (s *Service) addFailures(ctx context.Context, ids []int64, out map[int64]*DeliveryView) error {
	rows, err := s.pool.Query(ctx, `
		SELECT campaign_id, channel,
		       CASE WHEN last_error LIKE 'delivery too old%' THEN 'too_old'
		            WHEN last_error LIKE 'gave up after%' THEN 'gave_up'
		            WHEN last_error ~ 'http 4[0-9][0-9]' THEN 'rejected'
		            ELSE 'other' END AS reason,
		       count(*)
		FROM deliveries WHERE campaign_id = ANY($1) AND status = 'failed'
		GROUP BY 1, 2, 3 ORDER BY 4 DESC, 2, 3`, ids)
	if err != nil {
		return fmt.Errorf("delivery failures: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var f FailureReason
		if err := rows.Scan(&id, &f.Channel, &f.Reason, &f.Count); err != nil {
			return fmt.Errorf("delivery failures: %w", err)
		}
		out[id].Failures = append(out[id].Failures, f)
	}
	return rows.Err()
}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// List returns the newest campaigns first, each with its sending progress once
// it has been approved. A limit outside 1..200 means the default (50) / the maximum.
func (s *Service) List(ctx context.Context, limit int) ([]Summary, error) {
	switch {
	case limit <= 0:
		limit = defaultListLimit
	case limit > maxListLimit:
		limit = maxListLimit
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, brief, status, coalesce(segment_status, ''), matched, created_at
		FROM campaigns ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()
	out := []Summary{}
	var sending []int64
	for rows.Next() {
		var c Summary
		if err := rows.Scan(&c.ID, &c.Brief, &c.Status, &c.SegmentStatus, &c.Matched, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list campaigns: %w", err)
		}
		switch c.Status {
		case StatusApproved, StatusSending, StatusCompleted:
			sending = append(sending, c.ID)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	rows.Close()
	if len(sending) == 0 {
		return out, nil
	}
	progress, err := s.deliveryProgressFor(ctx, sending)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if v, ok := progress[out[i].ID]; ok {
			out[i].Delivery = v.finish()
		}
	}
	return out, nil
}
