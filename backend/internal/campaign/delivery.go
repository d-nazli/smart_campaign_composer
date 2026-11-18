package campaign

import (
	"context"
	"encoding/json"
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
	// FirstSentAt and LastSentAt bound the messages accepted so far. The list
	// derives the average speed from them (sent / the time in between).
	FirstSentAt *time.Time `json:"first_sent_at,omitempty"`
	LastSentAt  *time.Time `json:"last_sent_at,omitempty"`
}

// DeliveryView is the sending progress of an approved campaign.
type DeliveryView struct {
	SMS   ChannelProgress `json:"sms"`
	Email ChannelProgress `json:"email"`
	// Done is true once nothing is pending in either channel.
	Done bool `json:"done"`
	// Failures groups the failed deliveries by why they failed (largest first).
	Failures []FailureReason `json:"failures,omitempty"`
	// Locales is how many messages (SMS and email together) go to each locale
	// and how many of them were sent: in the locale the message was written in,
	// or (not claimed yet) the person's locale now.
	Locales map[string]LocaleProgress `json:"locales,omitempty"`
}

// LocaleProgress is one locale's share of a campaign's deliveries.
type LocaleProgress struct {
	Total int `json:"total"`
	Sent  int `json:"sent"`
}

// FailureReason is one group of failed deliveries. Reason is a stable code, not
// the raw provider text: "rejected" (the provider refused the recipient or the
// message, so a retry cannot help), "gave_up" (retries ran out), "too_old"
// (waited too long to be worth sending), "too_long" (the personalised text broke
// the 160 / 80 limit and was not sent), "other".
type FailureReason struct {
	Channel string `json:"channel"`
	Reason  string `json:"reason"`
	Count   int    `json:"count"`
}

// Summary is one line of the campaign list (the dashboard).
type Summary struct {
	ID    int64  `json:"id"`
	Brief string `json:"brief"`
	// Name is the campaign's short name ("" for one without; the list shows the brief then).
	Name          string `json:"name,omitempty"`
	Status        string `json:"status"`
	SegmentStatus string `json:"segment_status,omitempty"`
	Matched       *int   `json:"matched,omitempty"`
	// Explanation is the audience in words ("İstanbul'daki, son 30 günde …").
	Explanation string `json:"explanation,omitempty"`
	// AudienceLocales is how many reachable people each locale has (an
	// estimate until the campaign starts).
	AudienceLocales map[string]int `json:"audience_locales,omitempty"`
	Delivery        *DeliveryView  `json:"delivery,omitempty"`
	// ScheduledAt is when a scheduled campaign starts; ScheduleError why it could not.
	ScheduledAt   *time.Time `json:"scheduled_at,omitempty"`
	ScheduleError string     `json:"schedule_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// add folds one (channel, status) group into the view.
func (d *DeliveryView) add(channel, status string, n, retries, limited int, first, last *time.Time) {
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
		p.FirstSentAt, p.LastSentAt = first, last
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
		SELECT campaign_id, channel, status, count(*), coalesce(sum(retries), 0)::int, coalesce(sum(rate_limited), 0)::int,
		       min(sent_at), max(sent_at)
		FROM deliveries WHERE campaign_id = ANY($1) GROUP BY 1, 2, 3`, ids)
	if err != nil {
		return nil, fmt.Errorf("delivery progress: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var channel, status string
		var n, retries, limited int
		var first, last *time.Time
		if err := rows.Scan(&id, &channel, &status, &n, &retries, &limited, &first, &last); err != nil {
			return nil, fmt.Errorf("delivery progress: %w", err)
		}
		out[id].add(channel, status, n, retries, limited, first, last)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("delivery progress: %w", err)
	}
	rows.Close()
	if err := s.addLocales(ctx, ids, out); err != nil {
		return nil, err
	}
	return out, s.addFailures(ctx, ids, out)
}

// addLocales fills DeliveryView.Locales.
func (s *Service) addLocales(ctx context.Context, ids []int64, out map[int64]*DeliveryView) error {
	rows, err := s.pool.Query(ctx, `
		SELECT d.campaign_id, coalesce(d.locale, u.locale), count(*), count(*) FILTER (WHERE d.status = 'sent')
		FROM deliveries d JOIN users u ON u.id = d.user_id
		WHERE d.campaign_id = ANY($1) GROUP BY 1, 2`, ids)
	if err != nil {
		return fmt.Errorf("delivery locales: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var code string
		var p LocaleProgress
		if err := rows.Scan(&id, &code, &p.Total, &p.Sent); err != nil {
			return fmt.Errorf("delivery locales: %w", err)
		}
		if out[id].Locales == nil {
			out[id].Locales = map[string]LocaleProgress{}
		}
		out[id].Locales[code] = p
	}
	return rows.Err()
}

// addFailures fills DeliveryView.Failures. The worker stores the last provider
// answer as free text; it is sorted into a few codes here so the screen can
// explain failures in plain words.
func (s *Service) addFailures(ctx context.Context, ids []int64, out map[int64]*DeliveryView) error {
	rows, err := s.pool.Query(ctx, `
		SELECT campaign_id, channel,
		       CASE WHEN last_error LIKE 'message too long%' THEN 'too_long'
		            WHEN last_error LIKE 'delivery too old%' THEN 'too_old'
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
		SELECT id, brief, name, status, coalesce(segment_status, ''), matched, explanation, locale_counts, scheduled_at, schedule_error, created_at
		FROM campaigns ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()
	out := []Summary{}
	var sending []int64
	for rows.Next() {
		var c Summary
		var counts []byte
		if err := rows.Scan(&c.ID, &c.Brief, &c.Name, &c.Status, &c.SegmentStatus, &c.Matched, &c.Explanation, &counts, &c.ScheduledAt, &c.ScheduleError, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list campaigns: %w", err)
		}
		if len(counts) > 0 {
			_ = json.Unmarshal(counts, &c.AudienceLocales)
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
