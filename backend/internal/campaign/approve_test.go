package campaign_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
)

// approvableCampaign takes a campaign to content_ready (segment + generated messages).
func approvableCampaign(t *testing.T, svc *campaign.Service) int64 {
	t.Helper()
	id := offerCampaign(t, svc)
	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	return id
}

func deliveryCounts(t *testing.T, pool *pgxpool.Pool, id int64) (sms, email, total int) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE channel='sms'), count(*) FILTER (WHERE channel='email'), count(*)
		FROM deliveries WHERE campaign_id=$1`, id).Scan(&sms, &email, &total)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func campaignStatus(t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM campaigns WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// What was shown on the screen as "reachable" is exactly what gets queued, and
// nobody without consent or contact data is ever queued.
func TestApproveQueuesExactlyTheReachableAudience(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	before, err := svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Approve(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	aud := before.Segment.Audience
	if res.Queued.SMS != aud.SMSReachable || res.Queued.Email != aud.EmailReachable {
		t.Fatalf("queued %+v, screen promised sms=%d email=%d", res.Queued, aud.SMSReachable, aud.EmailReachable)
	}
	if res.AlreadyApproved || res.Status != "approved" || res.Audience == nil || res.Audience.Matched != aud.Matched {
		t.Fatalf("result: %+v", res)
	}
	sms, email, total := deliveryCounts(t, pool, id)
	if sms != res.Queued.SMS || email != res.Queued.Email || total != sms+email || total == 0 {
		t.Fatalf("rows sms=%d email=%d total=%d, result %+v", sms, email, total, res.Queued)
	}
	if got := campaignStatus(t, pool, id); got != "approved" {
		t.Fatalf("status = %s", got)
	}

	var notReachable, notPending int
	err = pool.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE (d.channel='sms'   AND NOT (u.sms_consent   AND u.phone IS NOT NULL))
		                           OR (d.channel='email' AND NOT (u.email_consent AND u.email IS NOT NULL))),
		       count(*) FILTER (WHERE d.status <> 'pending' OR d.attempts <> 0)
		FROM deliveries d JOIN users u ON u.id = d.user_id WHERE d.campaign_id=$1`, id).Scan(&notReachable, &notPending)
	if err != nil {
		t.Fatal(err)
	}
	if notReachable != 0 || notPending != 0 {
		t.Fatalf("queued %d unreachable recipients and %d rows that are not fresh", notReachable, notPending)
	}
}

func TestApprovingTwiceDoesNotQueueTwice(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	first, err := svc.Approve(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	_, _, total := deliveryCounts(t, pool, id)

	again, err := svc.Approve(context.Background(), id)
	if err != nil {
		t.Fatalf("a repeated approval is not an error: %v", err)
	}
	if !again.AlreadyApproved || again.Queued != first.Queued {
		t.Fatalf("repeat = %+v, first = %+v", again, first)
	}
	if _, _, after := deliveryCounts(t, pool, id); after != total {
		t.Fatalf("rows %d -> %d", total, after)
	}
}

func TestConcurrentApprovalsQueueOnceAndExactlyOneWins(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)

	const n = 8
	results := make([]*campaign.ApproveResult, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = svc.Approve(context.Background(), id)
		}()
	}
	wg.Wait()

	winners := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("approval %d: %v", i, errs[i])
		}
		if !results[i].AlreadyApproved {
			winners++
		}
		if results[i].Queued != results[0].Queued {
			t.Errorf("approval %d saw %+v, approval 0 saw %+v", i, results[i].Queued, results[0].Queued)
		}
	}
	if winners != 1 {
		t.Fatalf("%d approvals did the enqueueing, want exactly 1", winners)
	}
	sms, email, total := deliveryCounts(t, pool, id)
	if sms != results[0].Queued.SMS || email != results[0].Queued.Email || total != sms+email {
		t.Fatalf("rows sms=%d email=%d, result %+v", sms, email, results[0].Queued)
	}
}

func TestApproveRefusesCampaignsThatAreNotReady(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()

	if _, err := svc.Approve(ctx, 999999); !errors.Is(err, campaign.ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}

	// Segment is ready but there is no content to send yet.
	noContent := offerCampaign(t, svc)
	var se *campaign.StateError
	if _, err := svc.Approve(ctx, noContent); !errors.As(err, &se) {
		t.Errorf("campaign without content: %v", err)
	}
	if _, _, total := deliveryCounts(t, pool, noContent); total != 0 {
		t.Errorf("queued %d rows for a campaign that was refused", total)
	}

	// A brief that was never understood has no audience at all.
	unsupported, err := svc.Create(ctx, "Adana'daki 35 yaş üstü müşterilere %10 indirim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, unsupported.ID); !errors.As(err, &se) {
		t.Errorf("unsupported brief: %v", err)
	}
}

// The text is re-validated at approval; the stored content could have been
// damaged or the limits changed since it was saved.
func TestApproveRejectsContentThatBreaksTheLimitsAndChangesNothing(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := pool.Exec(context.Background(), `UPDATE campaign_contents SET sms_body=$2 WHERE campaign_id=$1`, id, string(long)); err != nil {
		t.Fatal(err)
	}

	var ic *campaign.InvalidContentError
	if _, err := svc.Approve(context.Background(), id); !errors.As(err, &ic) {
		t.Fatalf("want InvalidContentError, got %v", err)
	}
	if got := campaignStatus(t, pool, id); got != "content_ready" {
		t.Errorf("status = %s; a refused approval must not change the campaign", got)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Errorf("queued %d rows for a refused approval", total)
	}
}

// Warnings (D46) never block: the person has the last word, but sees them again.
func TestApproveWithAnEditedMessageThatLostTheOfferStillWorksAndWarns(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	id := offerCampaign(t, svc)
	_, err := svc.SaveContents(context.Background(), id, allLanguages(content.Content{
		SMSBody: "{{first_name}}, sepetin seni bekliyor!", EmailSubject: "Sepetin seni bekliyor", EmailBody: "Merhaba {{first_name}}, sepetin hâlâ burada.",
	}))
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Approve(context.Background(), id)
	if err != nil {
		t.Fatalf("warnings must not block approval: %v", err)
	}
	if len(res.Warnings) < 2 || res.Warnings[0].Code != content.CodeOfferMissing {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
}

// What was approved is what is sent: the text and the audience are frozen.
func TestAfterApprovalContentCannotChange(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	if _, err := svc.Approve(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	var se *campaign.StateError
	_, err := svc.SaveContent(context.Background(), id, content.Content{SMSBody: "x", EmailSubject: "y", EmailBody: "z"})
	if !errors.As(err, &se) {
		t.Errorf("edit after approval: %v", err)
	}
	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); !errors.As(err, &se) {
		t.Errorf("regenerate after approval: %v", err)
	}
}
