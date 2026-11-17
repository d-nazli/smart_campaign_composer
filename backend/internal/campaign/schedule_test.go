package campaign_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/clock"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
)

var trt = time.FixedZone("TRT", 3*3600)

func newScheduler(t *testing.T, pool *pgxpool.Pool, clk clock.Clock) *campaign.Scheduler {
	t.Helper()
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	return campaign.NewScheduler(pool, policy, clk, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// scheduled makes a campaign (messages ready) and approves it for later.
func scheduled(t *testing.T, svc *campaign.Service, at time.Time) int64 {
	t.Helper()
	id := approvableCampaign(t, svc)
	res, err := svc.ApproveAt(context.Background(), id, &at)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != campaign.StatusScheduled {
		t.Fatalf("status %s", res.Status)
	}
	return id
}

func inTwoHours() time.Time { return time.Now().Add(2 * time.Hour).Truncate(time.Second) }

// ---- approving for later -------------------------------------------------------

func TestApprovingForLaterOnlyRecordsTheDecision(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	ctx := context.Background()
	at := inTwoHours()

	res, err := svc.ApproveAt(ctx, id, &at)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != campaign.StatusScheduled || res.ScheduledAt == nil || !res.ScheduledAt.Equal(at) ||
		res.AlreadyApproved || res.Audience != nil || res.Queued != (campaign.Queued{}) {
		t.Fatalf("result: %+v", res)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Fatalf("a scheduled campaign has no deliveries yet, got %d", total)
	}

	v, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusScheduled || v.ScheduledAt == nil || !v.ScheduledAt.Equal(at) || v.Delivery != nil {
		t.Fatalf("view: status=%s scheduled=%v delivery=%v", v.Status, v.ScheduledAt, v.Delivery)
	}

	// The text and the rule are frozen from the moment of approval, as before.
	var se *campaign.StateError
	if _, err := svc.SaveContent(ctx, id, content.Content{SMSBody: "x", EmailSubject: "y", EmailBody: "z"}); !errors.As(err, &se) {
		t.Errorf("edit of a scheduled campaign: %v", err)
	}
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); !errors.As(err, &se) {
		t.Errorf("regenerate: %v", err)
	}

	// Approving again (even with another time) reports the first decision.
	later := at.Add(time.Hour)
	again, err := svc.ApproveAt(ctx, id, &later)
	if err != nil || !again.AlreadyApproved || again.ScheduledAt == nil || !again.ScheduledAt.Equal(at) {
		t.Fatalf("second approval: %v %+v", err, again)
	}
	if now, err := svc.Approve(ctx, id); err != nil || !now.AlreadyApproved || now.Status != campaign.StatusScheduled {
		t.Fatalf("send now after scheduling must not send: %v %+v", err, now)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Fatalf("%d deliveries after repeated approvals", total)
	}
}

func TestScheduleTimeMustBeInAReasonableWindow(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	ctx := context.Background()
	for name, at := range map[string]time.Time{
		"in the past":            time.Now().Add(-time.Hour),
		"a day ago":              time.Now().Add(-24 * time.Hour),
		"within the minute":      time.Now().Add(20 * time.Second),
		"just after 30 days":     time.Now().Add(30*24*time.Hour + 5*time.Minute),
		"after 31 days":          time.Now().Add(31 * 24 * time.Hour),
		"a year ahead (a typo?)": time.Now().AddDate(1, 0, 0),
	} {
		at := at
		_, err := svc.ApproveAt(ctx, id, &at)
		var sched *campaign.ScheduleError
		if !errors.As(err, &sched) {
			t.Errorf("%s: want ScheduleError, got %v", name, err)
		}
	}
	if got := campaignStatus(t, pool, id); got != "content_ready" {
		t.Fatalf("a refused schedule must leave the campaign waiting for approval, got %s", got)
	}
	ok := time.Now().Add(2 * time.Minute)
	if _, err := svc.ApproveAt(ctx, id, &ok); err != nil {
		t.Fatalf("two minutes ahead is fine: %v", err)
	}

	// The last allowed moment: just under 30 days ahead is still accepted.
	other := approvableCampaign(t, svc)
	edge := time.Now().Add(30*24*time.Hour - 5*time.Minute)
	if res, err := svc.ApproveAt(ctx, other, &edge); err != nil || res.Status != campaign.StatusScheduled {
		t.Fatalf("just under 30 days ahead is fine: %v %+v", err, res)
	}
}

// ---- the offer's last day ------------------------------------------------------

func datedBrief(day time.Time) string {
	return fmt.Sprintf("anneler gününe özel 1000 tl üzeri alışverişe 100tl indirim kullandır son kullanım %s\n\nTüm müşterilere gönder.", day.Format("02.01.2006"))
}

func dated(t *testing.T, svc *campaign.Service, day time.Time) int64 {
	t.Helper()
	v := createBrief(t, svc, datedBrief(day))
	if v.Segment == nil || v.Segment.Status != "ok" || v.Segment.Offer.ExpiresAt != day.Format("2006-01-02") {
		t.Fatalf("segment: %+v", v.Segment)
	}
	if err := svc.GenerateMessages(context.Background(), v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	return v.ID
}

func TestAScheduleAfterTheOffersLastDayIsRefused(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	lastDay := time.Now().In(trt).AddDate(0, 0, 5)
	id := dated(t, svc, lastDay)
	endOfLastDay := time.Date(lastDay.Year(), lastDay.Month(), lastDay.Day(), 23, 59, 59, 0, trt)

	after := endOfLastDay.Add(time.Hour)
	_, err := svc.ApproveAt(ctx, id, &after)
	var sched *campaign.ScheduleError
	if !errors.As(err, &sched) || !strings.Contains(err.Error(), lastDay.Format("02.01.2006")) {
		t.Fatalf("a time after the last day: %v", err)
	}
	if got := campaignStatus(t, pool, id); got != "content_ready" {
		t.Fatalf("status %s", got)
	}

	// Later on the last day itself is still inside the offer.
	onTheDay := endOfLastDay.Add(-time.Hour)
	res, err := svc.ApproveAt(ctx, id, &onTheDay)
	if err != nil || res.Status != campaign.StatusScheduled {
		t.Fatalf("a time on the last day: %v %+v", err, res)
	}
}

// An offer that ended while the campaign waited (a worker that was down for
// days) is never announced.
func TestAnOfferThatHasEndedIsNotSent(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	lastDay := time.Now().In(trt).AddDate(0, 0, 3)
	id := dated(t, svc, lastDay)
	at := time.Now().Add(3 * time.Hour)
	if _, err := svc.ApproveAt(ctx, id, &at); err != nil {
		t.Fatal(err)
	}

	clk := clock.NewFake(time.Date(lastDay.Year(), lastDay.Month(), lastDay.Day(), 12, 0, 0, 0, trt).AddDate(0, 0, 2)) // two days after the last day
	n, err := newScheduler(t, pool, clk).RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	v, _ := svc.Get(ctx, id)
	if v.Status != campaign.StatusFailed || !strings.Contains(v.ScheduleError, lastDay.Format("02.01.2006")) {
		t.Fatalf("status %s, error %q", v.Status, v.ScheduleError)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Fatalf("%d deliveries of an ended offer", total)
	}
}

// ---- starting -------------------------------------------------------------------

func TestNothingStartsBeforeItsTimeAndEverythingAtIt(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	at := inTwoHours()
	id := scheduled(t, svc, at)

	clk := clock.NewFake(at.Add(-time.Second))
	s := newScheduler(t, pool, clk)
	if n, err := s.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("before the time: n=%d err=%v", n, err)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Fatal("deliveries before the time")
	}

	clk.Advance(time.Second)
	if n, err := s.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("at the time: n=%d err=%v", n, err)
	}
	sms, email, _ := deliveryCounts(t, pool, id)
	if sms == 0 || email == 0 {
		t.Fatalf("sms=%d email=%d", sms, email)
	}
	if got := campaignStatus(t, pool, id); got != "approved" {
		t.Fatalf("status %s", got)
	}
	v, _ := svc.Get(ctx, id)
	if v.Delivery == nil || v.Delivery.SMS.Total != sms || v.Delivery.Email.Total != email {
		t.Fatalf("progress: %+v", v.Delivery)
	}
	var started *time.Time
	if err := pool.QueryRow(ctx, `SELECT materialized_at FROM campaigns WHERE id=$1`, id).Scan(&started); err != nil || started == nil || !started.Equal(clk.Now()) {
		t.Fatalf("materialized_at = %v (%v)", started, err)
	}

	// Run again: it is not started a second time.
	if n, err := s.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("second run: n=%d err=%v", n, err)
	}
	if s2, e2, _ := deliveryCounts(t, pool, id); s2 != sms || e2 != email {
		t.Fatal("the deliveries changed")
	}
}

// The audience is the one of the moment the campaign starts, not of the moment
// it was approved: new people are in, people who opted out or bought are out.
func TestTheAudienceIsDecidedWhenTheCampaignStarts(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	at := inTwoHours()
	id := scheduled(t, svc, at) // "last 30 days: put something in the cart, did not buy"

	before, _ := svc.Get(ctx, id)
	estimate := before.Segment.Audience.Matched

	// Between approval and start: a new matching customer, someone who withdraws
	// SMS consent, someone who buys.
	var newcomer int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (first_name, last_name, email, phone, city, sms_consent, email_consent, last_cart_add_at, created_at, language)
		VALUES ('Yeni', 'Gelen', 'yeni.gelen@example.com', '+905559990001', 'Ankara', true, true, now() - interval '1 day', now(), 'de')
		RETURNING id`).Scan(&newcomer); err != nil {
		t.Fatal(err)
	}
	pick := func(extra string) int64 {
		var u int64
		err := pool.QueryRow(ctx, `
			SELECT id FROM users u WHERE id <> $1 AND sms_consent AND email_consent
			  AND phone ~ '^\+90[0-9]{10}$' AND email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$'
			  AND last_cart_add_at > now() - interval '25 days'
			  AND (last_order_at IS NULL OR last_order_at < last_cart_add_at) `+extra+` ORDER BY id LIMIT 1`, newcomer).Scan(&u)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	optedOut := pick("")
	buyer := pick("AND id <> " + fmt.Sprint(optedOut))
	if _, err := pool.Exec(ctx, `UPDATE users SET sms_consent=false WHERE id=$1`, optedOut); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET last_order_at=now(), order_count=order_count+1 WHERE id=$1`, buyer); err != nil {
		t.Fatal(err)
	}

	clk := clock.NewFake(at.Add(time.Minute))
	if n, err := newScheduler(t, pool, clk).RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	has := func(user int64, channel string) bool {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND user_id=$2 AND channel=$3`, id, user, channel).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if !has(newcomer, "sms") || !has(newcomer, "email") {
		t.Error("someone who joined the audience after approval must get the message")
	}
	if has(optedOut, "sms") || !has(optedOut, "email") {
		t.Errorf("someone who withdrew SMS consent must get no SMS (but still the email): sms=%v email=%v", has(optedOut, "sms"), has(optedOut, "email"))
	}
	if has(buyer, "sms") || has(buyer, "email") {
		t.Error("someone who bought in the meantime is no longer in the audience")
	}

	// The numbers on the campaign are the ones of the start, and they are what was queued.
	after, _ := svc.Get(ctx, id)
	sms, email, _ := deliveryCounts(t, pool, id)
	if after.Segment.Audience.SMSReachable != sms || after.Segment.Audience.EmailReachable != email {
		t.Errorf("the audience shown after the start (%d SMS, %d email) must be what was queued (%d, %d); approval estimated %d people",
			after.Segment.Audience.SMSReachable, after.Segment.Audience.EmailReachable, sms, email, estimate)
	}
	if after.Segment.Languages["de"] == 0 {
		t.Errorf("the language counts are recounted at the start: %+v", after.Segment.Languages)
	}
}

// What can never be sent is marked failed with a reason; nothing is queued.
func TestACampaignThatCannotStartFailsWithAReason(t *testing.T) {
	cases := map[string]struct {
		break_ string
		want   string
	}{
		"corrupt rule":     {`UPDATE campaigns SET segment_rule = '{"status":"ok","conditions":[]}' WHERE id=$1`, "kural"},
		"missing language": {`DELETE FROM campaign_contents WHERE campaign_id=$1 AND language='en'`, "gönderime uygun değil"},
		"disagreeing text": {`UPDATE campaign_contents SET sms_body = replace(sms_body, '15%', '99%') WHERE campaign_id=$1 AND language='en'`, "gönderime uygun değil"},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			svc, pool := newService(t, mock.New(0))
			ctx := context.Background()
			at := inTwoHours()
			id := scheduled(t, svc, at)
			if _, err := pool.Exec(ctx, tt.break_, id); err != nil {
				t.Fatal(err)
			}
			n, err := newScheduler(t, pool, clock.NewFake(at.Add(time.Minute))).RunOnce(ctx)
			if err != nil || n != 1 {
				t.Fatalf("n=%d err=%v", n, err)
			}
			v, _ := svc.Get(ctx, id)
			if v.Status != campaign.StatusFailed || !strings.Contains(v.ScheduleError, tt.want) {
				t.Fatalf("status %s, error %q", v.Status, v.ScheduleError)
			}
			if _, _, total := deliveryCounts(t, pool, id); total != 0 {
				t.Fatalf("%d deliveries of a failed campaign", total)
			}
			// A failed campaign is not tried again and cannot be approved again.
			if n, _ := newScheduler(t, pool, clock.NewFake(at.Add(time.Hour))).RunOnce(ctx); n != 0 {
				t.Fatal("a failed campaign was picked up again")
			}
			var se *campaign.StateError
			if _, err := svc.Approve(ctx, id); !errors.As(err, &se) {
				t.Fatalf("approve after failure: %v", err)
			}
		})
	}
}

// ---- once and only once -----------------------------------------------------------

func TestManySchedulersStartEachCampaignExactlyOnce(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	at := inTwoHours()
	const campaigns = 4
	ids := make([]int64, campaigns)
	for i := range ids {
		ids[i] = scheduled(t, svc, at)
	}

	clk := clock.NewFake(at.Add(time.Minute))
	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	handled := make([]int, workers)
	errs := make([]error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := newScheduler(t, pool, clk)
			<-start
			handled[w], errs[w] = s.RunOnce(ctx)
		}()
	}
	close(start)
	wg.Wait()

	total := 0
	for w := range handled {
		if errs[w] != nil {
			t.Fatalf("scheduler %d: %v", w, errs[w])
		}
		total += handled[w]
	}
	if total != campaigns {
		t.Fatalf("%d campaigns were started %d times in all: %v", campaigns, total, handled)
	}
	for _, id := range ids {
		if got := campaignStatus(t, pool, id); got != "approved" {
			t.Errorf("campaign %d: %s", id, got)
		}
		var rows, distinct int
		if err := pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT (user_id, channel)) FROM deliveries WHERE campaign_id=$1`, id).Scan(&rows, &distinct); err != nil {
			t.Fatal(err)
		}
		if rows == 0 || rows != distinct {
			t.Errorf("campaign %d: %d rows, %d distinct", id, rows, distinct)
		}
	}
}

// ---- cancelling -------------------------------------------------------------------

func TestCancellingAScheduledCampaignStopsItForGood(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	at := inTwoHours()
	id := scheduled(t, svc, at)

	v, err := svc.CancelScheduled(ctx, id)
	if err != nil || v.Status != campaign.StatusCancelled {
		t.Fatalf("cancel: %v %+v", err, v)
	}
	if again, err := svc.CancelScheduled(ctx, id); err != nil || again.Status != campaign.StatusCancelled {
		t.Fatalf("cancelling twice must not fail: %v", err)
	}
	var cancelledAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT cancelled_at FROM campaigns WHERE id=$1`, id).Scan(&cancelledAt); err != nil || cancelledAt == nil {
		t.Fatalf("cancelled_at: %v %v", cancelledAt, err)
	}

	if n, err := newScheduler(t, pool, clock.NewFake(at.Add(time.Hour))).RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("a cancelled campaign was started: n=%d err=%v", n, err)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Fatalf("%d deliveries of a cancelled campaign", total)
	}
	var se *campaign.StateError
	if _, err := svc.Approve(ctx, id); !errors.As(err, &se) {
		t.Fatalf("a cancelled campaign cannot be approved again: %v", err)
	}
}

func TestOnlyAScheduledCampaignCanBeCancelled(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	ctx := context.Background()
	var se *campaign.StateError

	ready := approvableCampaign(t, svc)
	if _, err := svc.CancelScheduled(ctx, ready); !errors.As(err, &se) {
		t.Errorf("waiting for approval: %v", err)
	}
	sent := approvableCampaign(t, svc)
	if _, err := svc.Approve(ctx, sent); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelScheduled(ctx, sent); !errors.As(err, &se) || !strings.Contains(err.Error(), "başladığı") {
		t.Errorf("already sending: %v", err)
	}
	if _, err := svc.CancelScheduled(ctx, 987654); !errors.Is(err, campaign.ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}
}

// Cancelling while the scheduler is starting the same campaign: exactly one of
// them wins. Either the cancel succeeds and nothing was queued, or the campaign
// had started and the cancel is refused. Never a cancelled campaign with
// deliveries, and never a started one that was reported cancelled.
func TestCancelAndStartNeverBothHappen(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	cancelledWins, startWins := 0, 0
	for i := 0; i < 20; i++ {
		at := inTwoHours()
		id := scheduled(t, svc, at)
		clk := clock.NewFake(at.Add(time.Minute))
		s := newScheduler(t, pool, clk)

		gate := make(chan struct{})
		var wg sync.WaitGroup
		var cancelErr, runErr error
		wg.Add(2)
		go func() { defer wg.Done(); <-gate; _, cancelErr = svc.CancelScheduled(ctx, id) }()
		go func() { defer wg.Done(); <-gate; _, runErr = s.RunOnce(ctx) }()
		close(gate)
		wg.Wait()
		if runErr != nil {
			t.Fatal(runErr)
		}

		status := campaignStatus(t, pool, id)
		_, _, deliveries := deliveryCounts(t, pool, id)
		var se *campaign.StateError
		switch {
		case cancelErr == nil:
			cancelledWins++
			if status != "cancelled" || deliveries != 0 {
				t.Fatalf("round %d: cancel succeeded but status=%s deliveries=%d", i, status, deliveries)
			}
		case errors.As(cancelErr, &se):
			startWins++
			if status != "approved" || deliveries == 0 {
				t.Fatalf("round %d: cancel was refused but status=%s deliveries=%d", i, status, deliveries)
			}
		default:
			t.Fatalf("round %d: %v", i, cancelErr)
		}
	}
	t.Logf("cancel won %d times, start won %d times", cancelledWins, startWins)
}
