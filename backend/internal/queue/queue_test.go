package queue_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/queue"
	"smartcampaign/internal/testdb"
)

var ctx = context.Background()

// fixture: an approved campaign with nSMS sms deliveries and nEmail email
// deliveries, all pending and due. now is safely after their next_attempt_at.
type fixture struct {
	q        *queue.PG
	pool     *pgxpool.Pool
	campaign int64
	now      time.Time
}

func setup(t *testing.T, nSMS, nEmail int) fixture {
	t.Helper()
	pool := testdb.NewSeeded(t)
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO campaigns (brief, status) VALUES ('b', 'approved') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campaign_contents (campaign_id, locale, sms_body, email_subject, email_body, source) VALUES ($1,'tr-TR','sms {{first_name}}','konu','govde','llm')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deliveries (campaign_id, user_id, channel)
		SELECT $1, id, 'sms' FROM users WHERE sms_consent AND phone IS NOT NULL ORDER BY id LIMIT $2`, id, nSMS); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deliveries (campaign_id, user_id, channel)
		SELECT $1, id, 'email' FROM users WHERE email_consent AND email IS NOT NULL ORDER BY id LIMIT $2`, id, nEmail); err != nil {
		t.Fatal(err)
	}
	return fixture{q: queue.NewPG(pool), pool: pool, campaign: id, now: time.Now().Add(time.Minute)}
}

type row struct {
	Status                       string
	Attempts, Retries, RateLimit int
	LastError, Provider          string
	NextAttempt                  time.Time
	Locked                       *time.Time
	SentAt                       *time.Time
}

func (f fixture) row(t *testing.T, id int64) row {
	t.Helper()
	var r row
	err := f.pool.QueryRow(ctx, `
		SELECT status, attempts, retries, rate_limited, last_error, coalesce(provider_message_id,''), next_attempt_at, locked_until, sent_at
		FROM deliveries WHERE id=$1`, id).
		Scan(&r.Status, &r.Attempts, &r.Retries, &r.RateLimit, &r.LastError, &r.Provider, &r.NextAttempt, &r.Locked, &r.SentAt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f fixture) campaignStatus(t *testing.T) string {
	t.Helper()
	var s string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM campaigns WHERE id=$1`, f.campaign).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func ids(ds []queue.Delivery) []int64 {
	out := make([]int64, len(ds))
	for i, d := range ds {
		out[i] = d.ID
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func claim(t *testing.T, f fixture, channel string, limit int, at time.Time) []queue.Delivery {
	t.Helper()
	ds, err := f.q.Claim(ctx, channel, limit, 30*time.Second, at)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestClaimTakesDueRowsOfOneChannelAndLocksThem(t *testing.T) {
	f := setup(t, 10, 5)
	got := claim(t, f, "sms", 4, f.now)
	if len(got) != 4 {
		t.Fatalf("claimed %d, want 4", len(got))
	}
	for _, d := range got {
		if d.Channel != "sms" || d.CampaignID != f.campaign || d.FirstName == "" || d.City == "" || d.Phone == "" || !d.SMSConsent || d.Attempts != 0 {
			t.Fatalf("delivery: %+v", d)
		}
		r := f.row(t, d.ID)
		if r.Status != "sending" || r.Locked == nil || !r.Locked.Equal(f.now.Add(30*time.Second).Truncate(time.Microsecond)) {
			t.Fatalf("row after claim: %+v (want locked until %v)", r, f.now.Add(30*time.Second))
		}
	}
	if rest := claim(t, f, "sms", 100, f.now); len(rest) != 6 {
		t.Fatalf("second claim got %d, want the remaining 6", len(rest))
	}
	if email := claim(t, f, "email", 100, f.now); len(email) != 5 || email[0].Email == "" || !email[0].EmailConsent {
		t.Fatalf("email claim: %+v", email)
	}
}

func TestClaimedRowsAreNeverHandedOutTwice(t *testing.T) {
	f := setup(t, 40, 0)
	var mu sync.Mutex
	seen := map[int64]int{}
	var wg sync.WaitGroup
	for w := 0; w < 6; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				ds, err := f.q.Claim(ctx, "sms", 3, 30*time.Second, f.now)
				if err != nil {
					t.Error(err)
					return
				}
				if len(ds) == 0 {
					return
				}
				mu.Lock()
				for _, d := range ds {
					seen[d.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != 40 {
		t.Fatalf("claimed %d distinct rows, want 40", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("delivery %d was handed out %d times", id, n)
		}
	}
}

// A worker that crashes leaves a 'sending' row; it comes back once the lease ends.
func TestExpiredLeaseIsReclaimedButLiveLeaseIsNot(t *testing.T) {
	f := setup(t, 1, 0)
	first := claim(t, f, "sms", 1, f.now)
	if len(first) != 1 {
		t.Fatal("no first claim")
	}
	if got := claim(t, f, "sms", 1, f.now.Add(29*time.Second)); len(got) != 0 {
		t.Fatalf("a live lease was stolen: %+v", got)
	}
	again := claim(t, f, "sms", 1, f.now.Add(31*time.Second))
	if len(again) != 1 || again[0].ID != first[0].ID {
		t.Fatalf("expired lease not reclaimed: %+v", again)
	}
}

func TestRowsNotYetDueAreNotClaimed(t *testing.T) {
	f := setup(t, 1, 0)
	d := claim(t, f, "sms", 1, f.now)[0]
	at := f.now.Add(7 * time.Second)
	if err := f.q.Retry(ctx, d.ID, at, queue.RetryTransient, "boom", f.now); err != nil {
		t.Fatal(err)
	}
	if got := claim(t, f, "sms", 1, at.Add(-time.Second)); len(got) != 0 {
		t.Fatalf("claimed %d rows before they were due", len(got))
	}
	if got := claim(t, f, "sms", 1, at); len(got) != 1 {
		t.Fatalf("not claimed at the due time")
	}
}

func TestMarkSent(t *testing.T) {
	f := setup(t, 1, 0)
	d := claim(t, f, "sms", 1, f.now)[0]
	if err := f.q.MarkSent(ctx, d.ID, "pm_123", f.now); err != nil {
		t.Fatal(err)
	}
	r := f.row(t, d.ID)
	if r.Status != "sent" || r.Provider != "pm_123" || r.SentAt == nil || r.Locked != nil {
		t.Fatalf("row: %+v", r)
	}
	if got := claim(t, f, "sms", 10, f.now.Add(time.Hour)); len(got) != 0 {
		t.Fatal("a sent row was claimed again")
	}
}

func TestTransientRetryUsesAnAttemptButRateLimitDoesNot(t *testing.T) {
	f := setup(t, 2, 0)
	ds := claim(t, f, "sms", 2, f.now)
	at := f.now.Add(5 * time.Second)

	if err := f.q.Retry(ctx, ds[0].ID, at, queue.RetryTransient, "http 503", f.now); err != nil {
		t.Fatal(err)
	}
	if err := f.q.Retry(ctx, ds[1].ID, at, queue.RetryRateLimited, "http 429", f.now); err != nil {
		t.Fatal(err)
	}
	a, b := f.row(t, ds[0].ID), f.row(t, ds[1].ID)
	if a.Status != "pending" || a.Attempts != 1 || a.Retries != 1 || a.RateLimit != 0 || a.LastError != "http 503" || a.Locked != nil || !a.NextAttempt.Equal(at.Truncate(time.Microsecond)) {
		t.Fatalf("transient: %+v", a)
	}
	if b.Status != "pending" || b.Attempts != 0 || b.Retries != 1 || b.RateLimit != 1 || b.LastError != "http 429" {
		t.Fatalf("rate limited: %+v", b)
	}
}

func TestFailAndSkipAreFinal(t *testing.T) {
	f := setup(t, 2, 0)
	ds := claim(t, f, "sms", 2, f.now)
	if err := f.q.Fail(ctx, ds[0].ID, "invalid_recipient", f.now); err != nil {
		t.Fatal(err)
	}
	if err := f.q.Skip(ctx, ds[1].ID, "consent withdrawn", f.now); err != nil {
		t.Fatal(err)
	}
	if r := f.row(t, ds[0].ID); r.Status != "failed" || r.LastError != "invalid_recipient" || r.Locked != nil {
		t.Fatalf("failed: %+v", r)
	}
	if r := f.row(t, ds[1].ID); r.Status != "skipped" || r.LastError != "consent withdrawn" {
		t.Fatalf("skipped: %+v", r)
	}
	if got := claim(t, f, "sms", 10, f.now.Add(time.Hour)); len(got) != 0 {
		t.Fatal("final rows were claimed again")
	}
}

// Only a row this worker holds can be changed: a late result from a worker whose
// lease was taken over must not overwrite a final state.
func TestResultsOnlyApplyToRowsThatAreBeingSent(t *testing.T) {
	f := setup(t, 1, 0)
	d := claim(t, f, "sms", 1, f.now)[0]
	if err := f.q.MarkSent(ctx, d.ID, "pm_1", f.now); err != nil {
		t.Fatal(err)
	}
	if err := f.q.Fail(ctx, d.ID, "late", f.now); err != nil {
		t.Fatal(err)
	}
	if err := f.q.Retry(ctx, d.ID, f.now, queue.RetryTransient, "late", f.now); err != nil {
		t.Fatal(err)
	}
	if r := f.row(t, d.ID); r.Status != "sent" || r.LastError != "" || r.Attempts != 0 {
		t.Fatalf("a sent row was overwritten: %+v", r)
	}
}

func TestClaimMovesTheCampaignToSending(t *testing.T) {
	f := setup(t, 1, 0)
	if s := f.campaignStatus(t); s != "approved" {
		t.Fatalf("start: %s", s)
	}
	claim(t, f, "sms", 1, f.now)
	if s := f.campaignStatus(t); s != "sending" {
		t.Fatalf("after claim: %s", s)
	}
}

func TestFinishCompletesOnlyWhenNothingIsLeft(t *testing.T) {
	f := setup(t, 2, 1)
	sms := claim(t, f, "sms", 2, f.now)
	email := claim(t, f, "email", 1, f.now)

	finish := func() int {
		n, err := f.q.Finish(ctx, f.now)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	_ = f.q.MarkSent(ctx, sms[0].ID, "a", f.now)
	_ = f.q.Fail(ctx, sms[1].ID, "x", f.now)
	if finish() != 0 || f.campaignStatus(t) != "sending" {
		t.Fatal("completed while an email was still being sent")
	}
	_ = f.q.Retry(ctx, email[0].ID, f.now.Add(time.Second), queue.RetryTransient, "e", f.now)
	if finish() != 0 || f.campaignStatus(t) != "sending" {
		t.Fatal("completed while a retry was pending")
	}
	again := claim(t, f, "email", 1, f.now.Add(time.Second))
	_ = f.q.Skip(ctx, again[0].ID, "gone", f.now)
	if finish() != 1 || f.campaignStatus(t) != "completed" {
		t.Fatalf("not completed: %s", f.campaignStatus(t))
	}
	if finish() != 0 {
		t.Fatal("completed twice")
	}
}

// A campaign whose audience turned out empty must not stay 'approved' forever.
func TestFinishCompletesACampaignWithNoRecipients(t *testing.T) {
	f := setup(t, 0, 0)
	if n, err := f.q.Finish(ctx, f.now); err != nil || n != 1 || f.campaignStatus(t) != "completed" {
		t.Fatalf("n=%d err=%v status=%s", n, err, f.campaignStatus(t))
	}
}

func TestContentReturnsTheFrozenTemplatesWithTheLocalesWords(t *testing.T) {
	f := setup(t, 0, 0)
	tmpl, err := f.q.Content(ctx, f.campaign, "tr-TR")
	c := tmpl.Content
	if err != nil || c.SMSBody != "sms {{first_name}}" || c.EmailSubject != "konu" || c.EmailBody != "govde" {
		t.Fatalf("%+v %v", c, err)
	}
	if tmpl.Fallbacks["first_name"] != "değerli müşterimiz" {
		t.Fatalf("fallbacks = %v", tmpl.Fallbacks)
	}
	if _, err := f.q.Content(ctx, 424242, "tr-TR"); !errors.Is(err, queue.ErrNoContent) {
		t.Fatalf("unknown campaign: %v", err)
	}
	if _, err := f.q.Content(ctx, f.campaign, "fr-FR"); !errors.Is(err, queue.ErrNoContent) {
		t.Fatalf("a locale without a text: %v", err)
	}
}

// The locale a message is written in is read at claim time and kept on the row.
func TestClaimReadsTheLocaleNowAndKeepsItOnTheDelivery(t *testing.T) {
	f := setup(t, 1, 0)
	if _, err := f.pool.Exec(ctx, `UPDATE users SET locale='ar-SA'`); err != nil {
		t.Fatal(err)
	}
	got := claim(t, f, "sms", 1, f.now)
	if len(got) != 1 || got[0].Locale != "ar-SA" {
		t.Fatalf("%+v", got)
	}
	var stored string
	if err := f.pool.QueryRow(ctx, `SELECT locale FROM deliveries WHERE id=$1`, got[0].ID).Scan(&stored); err != nil || stored != "ar-SA" {
		t.Fatalf("stored locale %q %v", stored, err)
	}
}

var _ = ids

func TestObserveAddsCallsAndTimeAcrossAttempts(t *testing.T) {
	f := setup(t, 2, 0)
	ds := claim(t, f, "sms", 2, f.now)
	for _, l := range []time.Duration{120 * time.Millisecond, 80 * time.Millisecond} {
		if err := f.q.Observe(ctx, ds[0].ID, l); err != nil {
			t.Fatal(err)
		}
	}
	var calls int
	var micros int64
	if err := f.pool.QueryRow(ctx, `SELECT provider_calls, provider_micros FROM deliveries WHERE id=$1`, ds[0].ID).Scan(&calls, &micros); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || micros != 200_000 {
		t.Fatalf("calls=%d micros=%d, want 2 and 200000", calls, micros)
	}
	if err := f.pool.QueryRow(ctx, `SELECT provider_calls FROM deliveries WHERE id=$1`, ds[1].ID).Scan(&calls); err != nil || calls != 0 {
		t.Fatalf("another delivery was touched: %d %v", calls, err)
	}
}
