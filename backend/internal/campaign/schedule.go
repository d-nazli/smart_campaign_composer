package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/clock"
	"smartcampaign/internal/content"
	"smartcampaign/internal/locale"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// Scheduled sending.
//
// Approving "for later" only records the decision (status scheduled, scheduled_at).
// No audience is counted and no delivery is created: the people who match "the
// last 30 days, in Istanbul, ..." tomorrow are not the people who match today.
// When the time comes a worker runs the stored rule again, takes the audience of
// THAT moment and queues the deliveries; from there on it is an ordinary
// approved campaign. Nobody has to be there.

const (
	// A time that is too close would pass before anyone could look at the
	// campaign again; one that is too far is probably a typo (a wrong year).
	minScheduleLead  = time.Minute
	maxScheduleAhead = 30 * 24 * time.Hour
)

// businessZone is the time zone an offer's last day is read in: Turkey, UTC+3
// all year (it dropped daylight saving time in 2016). A fixed zone needs no
// timezone database in the container.
var businessZone = time.FixedZone("TRT", 3*60*60)

// offerLastMoment is the end of the offer's last day, in Turkish time.
func offerLastMoment(expiresAt string) (time.Time, bool) {
	day, err := time.ParseInLocation("2006-01-02", expiresAt, businessZone)
	if err != nil {
		return time.Time{}, false
	}
	return day.AddDate(0, 0, 1).Add(-time.Nanosecond), true
}

// checkScheduleTime refuses times that are too close or too far.
func checkScheduleTime(at, now time.Time) error {
	switch {
	case at.Before(now.Add(minScheduleLead)):
		return &ScheduleError{"gönderim zamanı en az 1 dakika sonrası olmalı"}
	case at.After(now.Add(maxScheduleAhead)):
		return &ScheduleError{"gönderim zamanı en fazla 30 gün sonrası olabilir"}
	}
	return nil
}

// checkScheduleAgainstOffer refuses a time after the offer's last day: that
// message would announce something that has ended. The last day itself is fine.
func checkScheduleAgainstOffer(at time.Time, o offer.Offer) error {
	o = o.Normalize()
	last, ok := offerLastMoment(o.ExpiresAt)
	if !ok || !at.After(last) {
		return nil
	}
	return &ScheduleError{fmt.Sprintf(
		"gönderim zamanı teklifin son kullanma tarihinden (%s) sonra olamaz; en geç o günün sonuna kadar bir zaman seçin",
		offer.FormatDate(o.ExpiresAt))}
}

// ---- cancel --------------------------------------------------------------------

// CancelScheduled cancels a campaign that has not started. It works only on a
// "scheduled" campaign. The scheduler locks the campaign row while it builds the
// audience, and this UPDATE waits for that lock, so exactly one of them wins:
// either the campaign is cancelled and nothing was queued, or it had already
// started and the cancel is refused. Never both.
func (s *Service) CancelScheduled(ctx context.Context, id int64) (*View, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE campaigns SET status='cancelled', cancelled_at=now(), updated_at=now()
		WHERE id=$1 AND status='scheduled'`, id)
	if err != nil {
		return nil, fmt.Errorf("cancel: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return s.Get(ctx, id)
	}
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	switch row.status {
	case StatusCancelled:
		return s.Get(ctx, id) // cancelling twice is not an error
	case StatusApproved, StatusSending, StatusCompleted:
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; gönderim başladığı için iptal edilemez", row.status)}
	}
	return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; yalnızca zamanlanmış kampanya iptal edilebilir", row.status)}
}

// ---- scheduler -----------------------------------------------------------------

// Scheduler starts the scheduled campaigns that are due. Several of them (in
// several workers) can run at once: a campaign is taken with FOR UPDATE SKIP
// LOCKED, so each one is started by exactly one of them.
type Scheduler struct {
	pool   *pgxpool.Pool
	policy content.Policy
	clk    clock.Clock
	log    *slog.Logger
	poll   time.Duration
}

func NewScheduler(pool *pgxpool.Pool, policy content.Policy, clk clock.Clock, log *slog.Logger) *Scheduler {
	return &Scheduler{pool: pool, policy: policy, clk: clk, log: log, poll: time.Second}
}

// Run starts due campaigns until ctx ends.
func (s *Scheduler) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		if n, err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("scheduler", "error", err)
		} else if n > 0 {
			s.log.Info("scheduled campaigns handled", "count", n)
		}
		_ = s.clk.Sleep(ctx, s.poll)
	}
	return ctx.Err()
}

// RunOnce starts every campaign that is due and returns how many it handled
// (started or failed). A campaign it cannot handle because of a database error
// stays scheduled and is tried again; the error is returned.
func (s *Scheduler) RunOnce(ctx context.Context) (int, error) {
	handled := 0
	for ctx.Err() == nil {
		ok, err := s.startNext(ctx)
		if err != nil {
			return handled, err
		}
		if !ok {
			return handled, nil
		}
		handled++
	}
	return handled, ctx.Err()
}

// startNext takes ONE due campaign and starts it. It returns false when none is due.
//
// Everything is one transaction: the audience, the deliveries and the status
// change commit together or not at all, so a crash in between leaves the
// campaign scheduled and it is simply tried again. After the commit the status
// is approved, so nobody picks it up a second time.
func (s *Scheduler) startNext(ctx context.Context) (bool, error) {
	now := s.clk.Now()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var id int64
	var ruleJSON []byte
	err = tx.QueryRow(ctx, `
		SELECT id, segment_rule FROM campaigns
		WHERE status='scheduled' AND scheduled_at <= $1
		ORDER BY scheduled_at, id LIMIT 1
		FOR UPDATE SKIP LOCKED`, now).Scan(&id, &ruleJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find due campaign: %w", err)
	}

	// What can never work is a permanent failure; the person sees the reason.
	reason, err := s.cannotStart(ctx, tx, id, ruleJSON, now)
	if err != nil {
		return false, err
	}
	if reason != "" {
		if _, err := tx.Exec(ctx, `UPDATE campaigns SET status='failed', schedule_error=$2, updated_at=$3 WHERE id=$1`, id, reason, now); err != nil {
			return false, fmt.Errorf("mark failed: %w", err)
		}
		s.log.Warn("scheduled campaign failed", "campaign", id, "reason", reason)
		return true, tx.Commit(ctx)
	}

	rule, _ := segment.Parse(string(ruleJSON)) // validated by cannotStart
	compiled, _ := segment.Compile(rule, now)
	aud, err := segment.CountAudience(ctx, tx, compiled)
	if err != nil {
		return false, err
	}
	counts, err := segment.CountByLocale(ctx, tx, compiled)
	if err != nil {
		return false, err
	}
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return false, err
	}
	q, err := enqueueAudience(ctx, tx, id, compiled, now)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE campaigns SET status='approved', matched=$2, sms_reachable=$3, email_reachable=$4, both_reachable=$5,
		       locale_counts=$6, materialized_at=$7, updated_at=$7
		WHERE id=$1`, id, aud.Matched, aud.SMSReachable, aud.EmailReachable, aud.BothReachable, countsJSON, now); err != nil {
		return false, fmt.Errorf("start campaign: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit start: %w", err)
	}
	s.log.Info("scheduled campaign started", "campaign", id, "matched", aud.Matched, "sms", q.SMS, "email", q.Email)
	return true, nil
}

// cannotStart returns why a due campaign can never be sent ("" when it can). A
// database error is returned as an error instead: that is worth retrying.
func (s *Scheduler) cannotStart(ctx context.Context, tx pgx.Tx, id int64, ruleJSON []byte, now time.Time) (string, error) {
	rule, err := segment.Parse(string(ruleJSON))
	if err != nil {
		return "kayıtlı kitle kuralı geçersiz: " + err.Error(), nil
	}
	if _, err := segment.Compile(rule, now); err != nil {
		return "kayıtlı kitle kuralı derlenemedi: " + err.Error(), nil
	}
	// An offer that ended while the campaign waited (or the worker was down) is
	// not announced.
	if last, ok := offerLastMoment(rule.Offer.Normalize().ExpiresAt); ok && now.After(last) {
		return fmt.Sprintf("teklifin son kullanma tarihi (%s) geçti; süresi dolmuş teklif gönderilmedi", offer.FormatDate(rule.Offer.Normalize().ExpiresAt)), nil
	}
	// The texts approved are checked again. A locale that gained a person after
	// the approval has no text: nobody is there to approve one, so those people
	// get nothing (exact locale match only, no other locale instead).
	ls, err := locale.Load(ctx, tx, s.policy)
	if err != nil {
		return "", err
	}
	stored, err := loadContents(ctx, tx, id)
	if err != nil {
		return "", err
	}
	have := map[string]bool{ls.Source().Code: true}
	if src, ok := stored[ls.Source().Code]; ok {
		for code, tr := range stored {
			if !isStale(src, tr) {
				have[code] = true // a stale text outside the approved audience is never sent
			}
		}
	}
	if _, err := checkContents(ctx, tx, s.policy, ls, id, ls.Ordered(have)); err != nil {
		var inv *InvalidContentError
		var st *StateError
		var stale *StaleTranslationsError
		if errors.As(err, &inv) || errors.As(err, &st) || errors.As(err, &stale) {
			return "kayıtlı mesajlar gönderime uygun değil: " + err.Error(), nil
		}
		return "", err
	}
	return "", nil
}
