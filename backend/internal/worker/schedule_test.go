package worker_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/clock"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/queue"
	"smartcampaign/internal/testdb"
)

// A scheduled campaign, end to end: the audience is built when the time comes,
// the person's locale is read when their message is sent, the provider gets
// exactly one message per person and channel.

const (
	targetPhone = "+905559990042"
	targetEmail = "hedef.kisi@example.com"
)

// scheduledForTarget approves a campaign for later and returns it with its start
// time. The audience rule matches anyone who put something in the cart recently
// and did not buy, which is how the target (created here) qualifies.
func scheduledForTarget(t *testing.T, pool *pgxpool.Pool) (id int64, target int64, at time.Time) {
	t.Helper()
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := campaign.NewService(pool, mock.New(0), policy, log)
	v, err := svc.Create(ctx, "Son 7 günde sepete ürün ekleyip almayanlara %15 indirim hatırlatması gönder")
	if err != nil || v.Segment.Status != "ok" {
		t.Fatalf("create: %v %+v", err, v)
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	at = time.Now().Add(3 * time.Hour).Truncate(time.Second)
	if _, err := svc.ApproveAt(ctx, v.ID, &at); err != nil {
		t.Fatal(err)
	}
	// The person joins after the campaign was scheduled, in the source locale.
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (first_name, last_name, email, phone, city, sms_consent, email_consent, last_cart_add_at, created_at, locale)
		VALUES ('Hedef', 'Kişi', $1, $2, 'Ankara', true, true, now() - interval '1 day', now(), 'tr-TR')
		RETURNING id`, targetEmail, targetPhone).Scan(&target); err != nil {
		t.Fatal(err)
	}
	return v.ID, target, at
}

func messagesTo(tp *testProvider, address string) []request {
	var out []request
	for _, r := range tp.requests() {
		if r.To == address {
			out = append(out, r)
		}
	}
	return out
}

func startScheduled(t *testing.T, pool *pgxpool.Pool, clk *clock.Fake) {
	t.Helper()
	policy, _ := content.NewPolicy(nil)
	n, err := campaign.NewScheduler(pool, policy, clk, log).RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("scheduler: n=%d err=%v", n, err)
	}
}

// The person changes their locale AFTER the campaign was scheduled (and before
// it starts): the message comes once, in the new locale.
func TestALocaleChangedAfterSchedulingGetsOneMessageInTheNewLocale(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	_, target, at := scheduledForTarget(t, pool)

	if _, err := pool.Exec(ctx, `UPDATE users SET locale='de-DE' WHERE id=$1`, target); err != nil {
		t.Fatal(err)
	}

	clk := clock.NewFake(at.Add(time.Minute))
	startScheduled(t, pool, clk)
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	sms, email := messagesTo(tp, targetPhone), messagesTo(tp, targetEmail)
	if len(sms) != 1 || len(email) != 1 {
		t.Fatalf("one SMS and one email expected, got %d and %d", len(sms), len(email))
	}
	for _, r := range []request{sms[0], email[0]} {
		if !strings.Contains(r.Body, "15 % Rabatt") || strings.Contains(r.Body, "indirim") {
			t.Fatalf("%s must be German: %q", r.Channel, r.Body)
		}
		if strings.Contains(r.Body, "{{") {
			t.Fatalf("unrendered template: %q", r.Body)
		}
	}
	if !strings.Contains(sms[0].Body, "Hedef") {
		t.Fatalf("the name is filled in: %q", sms[0].Body)
	}
}

// ...and also when the change comes after the deliveries were created but
// before they were sent: the locale is read when the message goes out.
func TestALocaleChangedAfterTheStartIsStillRespected(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	_, target, at := scheduledForTarget(t, pool)

	clk := clock.NewFake(at.Add(time.Minute))
	startScheduled(t, pool, clk)
	if got := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE user_id=$1`, target); got != 2 {
		t.Fatalf("the target should have an SMS and an email queued, got %d", got)
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET locale='en-GB' WHERE id=$1`, target); err != nil {
		t.Fatal(err)
	}
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	sms, email := messagesTo(tp, targetPhone), messagesTo(tp, targetEmail)
	if len(sms) != 1 || len(email) != 1 {
		t.Fatalf("one SMS and one email expected, got %d and %d", len(sms), len(email))
	}
	for _, r := range []request{sms[0], email[0]} {
		if !strings.Contains(r.Body, "15% off") || strings.Contains(r.Body, "indirim") || strings.Contains(r.Body, "Rabatt") {
			t.Fatalf("%s must be English: %q", r.Channel, r.Body)
		}
	}
}

// A locale with nobody in the audience at approval has no text. Someone who
// moves to it before the start gets nothing (nobody approved a text for them),
// and everybody else is sent as usual.
func TestALocaleWithoutAnApprovedTextGetsNothing(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id, target, at := scheduledForTarget(t, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO locales (code, name, native_name, fallbacks) VALUES
		('fr-CA', 'French (Canada)', 'Français (Canada)', '{"first_name": "cher client", "city": "votre région"}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET locale='fr-CA' WHERE id=$1`, target); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFake(at.Add(time.Minute))
	startScheduled(t, pool, clk)
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	if got := len(messagesTo(tp, targetPhone)) + len(messagesTo(tp, targetEmail)); got != 0 {
		t.Fatalf("%d messages reached a person whose locale has no text", got)
	}
	if n := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE user_id=$1 AND status='skipped'`, target); n != 2 {
		t.Fatalf("both deliveries should be skipped, got %d", n)
	}
	if n := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND status='sent'`, id); n == 0 {
		t.Fatal("the others are still sent")
	}
}

// Nothing is sent before the time, and cancelling means nothing is ever sent.
func TestACancelledScheduledCampaignSendsNothing(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id, target, at := scheduledForTarget(t, pool)
	policy, _ := content.NewPolicy(nil)
	svc := campaign.NewService(pool, mock.New(0), policy, log)

	clk := clock.NewFake(at.Add(-time.Minute))
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	if n, _ := w.RunOnce(ctx); n != 0 || len(tp.requests()) != 0 {
		t.Fatal("something was sent before the time")
	}
	if _, err := svc.CancelScheduled(ctx, id); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Hour)
	sched := campaign.NewScheduler(pool, policy, clk, log)
	if n, err := sched.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	drain(t, w, pool, clk)
	if got := len(tp.requests()); got != 0 {
		t.Fatalf("%d messages were sent for a cancelled campaign", got)
	}
	if got := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE user_id=$1`, target); got != 0 {
		t.Fatalf("%d deliveries", got)
	}
}

// The whole thing for the audience of the seed: everyone reachable gets exactly
// one message once the campaign has started, nobody before.
func TestAScheduledCampaignDeliversToItsAudienceOnceStarted(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id, _, at := scheduledForTarget(t, pool)

	clk := clock.NewFake(at.Add(time.Minute))
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	if n, err := w.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("the worker has nothing before the start: n=%d err=%v", n, err)
	}
	startScheduled(t, pool, clk)
	drain(t, w, pool, clk)

	c := counts(t, pool, id)
	if c["sms/sent"] == 0 || c["email/sent"] == 0 {
		t.Fatalf("%v", c)
	}
	for _, ch := range []string{"sms", "email"} {
		if got := tp.stats(ch).UniqueDeliveries; got != c[ch+"/sent"] {
			t.Errorf("%s: provider holds %d messages, %d deliveries are sent", ch, got, c[ch+"/sent"])
		}
	}
	if got := scalar(t, pool, `SELECT count(*) FROM campaigns WHERE id=$1 AND status='completed'`, id); got != 1 {
		t.Fatal("the campaign was not marked completed")
	}
}
