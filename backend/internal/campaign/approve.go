package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"smartcampaign/internal/content"
	"smartcampaign/internal/locale"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// Statuses of the approval and sending phase.
const (
	// StatusScheduled: approved for a later time. Nothing is queued yet; the
	// audience is decided when the time comes (see Scheduler).
	StatusScheduled = "scheduled"
	StatusApproved  = "approved"
	StatusSending   = "sending"
	StatusCompleted = "completed"
	// StatusCancelled: a person cancelled a scheduled campaign before it started.
	StatusCancelled = "cancelled"
	// StatusFailed: a scheduled campaign could not be started (see ScheduleError).
	StatusFailed = "failed"
)

// Queued is how many deliveries were enqueued per channel.
type Queued struct {
	SMS   int `json:"sms"`
	Email int `json:"email"`
}

// ApproveResult is what approving a campaign reports.
type ApproveResult struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	// AlreadyApproved is true when this call found the campaign approved (a
	// repeated or concurrent approval); nothing new was enqueued.
	AlreadyApproved bool   `json:"already_approved"`
	Queued          Queued `json:"queued"`
	// ScheduledAt is when a scheduled campaign starts. A scheduled campaign has
	// nothing queued and no audience yet: both are decided at that moment.
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	// Audience is the audience counted at approval time (nil on a repeat and for a
	// scheduled campaign). It can differ from what the screen showed earlier: the
	// data moves on and "the last 30 days" slides. Queued is the truth.
	Audience *segment.Audience `json:"audience,omitempty"`
	// Warnings are the soft issues of the stored source text (D46). They never block.
	Warnings []content.Issue `json:"warnings"`
	// Locales is how many people receive each locale (reachable on at least
	// one channel), counted at approval time.
	Locales map[string]int `json:"locales,omitempty"`
}

// Approve freezes the campaign and enqueues one delivery per reachable
// recipient and channel, all in one transaction (it sends now).
func (s *Service) Approve(ctx context.Context, id int64) (*ApproveResult, error) {
	return s.ApproveAt(ctx, id, nil)
}

// ApproveAt approves the campaign. With at == nil it sends now: the audience is
// counted and the deliveries are queued in this transaction. With a time it
// only records the decision: the campaign becomes "scheduled" and the worker
// builds the audience and the deliveries when the time comes.
//
// The status change is a compare-and-swap (content_ready -> approved/scheduled).
// The loser of a concurrent approval blocks on that row, then finds the
// campaign approved and reports the same state instead of doing it again; the
// UNIQUE(campaign_id, user_id, channel) + ON CONFLICT DO NOTHING is the second
// line of defence.
func (s *Service) ApproveAt(ctx context.Context, id int64, at *time.Time) (*ApproveResult, error) {
	now := time.Now()
	if at != nil {
		if err := checkScheduleTime(*at, now); err != nil {
			return nil, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	status := StatusApproved
	if at != nil {
		status = StatusScheduled
	}
	var ruleJSON []byte
	err = tx.QueryRow(ctx, `
		UPDATE campaigns SET status=$2, scheduled_at=$3, approved_at=now(), updated_at=now()
		WHERE id=$1 AND status='content_ready' RETURNING segment_rule`, id, status, at).Scan(&ruleJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return s.alreadyApprovedOrRefuse(ctx, id)
	}
	if err != nil {
		return nil, fmt.Errorf("approve: %w", err)
	}

	// The stored rule goes through the same strict validation as model output
	// before it can become SQL (it also restores the typed values JSON loses).
	rule, err := segment.Parse(string(ruleJSON))
	if err != nil {
		return nil, fmt.Errorf("stored rule is corrupt: %w", err)
	}

	compiled, err := segment.Compile(rule, now)
	if err != nil {
		return nil, fmt.Errorf("compile stored rule: %w", err)
	}

	// The campaign's locales are those of the audience NOW: a locale that gained
	// a person since the messages were written needs its translation first.
	ls, err := locale.Load(ctx, tx, s.policy)
	if err != nil {
		return nil, err
	}
	counts, err := segment.CountByLocale(ctx, tx, compiled)
	if err != nil {
		return nil, err
	}
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return nil, err
	}

	// The text is validated again, in every locale of the campaign: what is
	// sent must satisfy every limit, every translation must be up to date and
	// say what the source says.
	set, err := checkContents(ctx, tx, s.policy, ls, id, campaignLocales(ls, counts))
	if err != nil {
		var st *StaleTranslationsError
		if errors.As(err, &st) {
			_ = tx.Rollback(ctx)
			// The screen shows the new locales as missing.
			if _, uerr := s.pool.Exec(ctx, `UPDATE campaigns SET locale_counts=$2, updated_at=now() WHERE id=$1`, id, countsJSON); uerr != nil {
				s.log.Error("could not store locale counts", "campaign_id", id, "error", uerr)
			}
		}
		return nil, err
	}

	if at != nil {
		// A message that goes out after the offer has ended would promise something
		// that no longer exists; the person must pick a time inside the offer.
		if err := checkScheduleAgainstOffer(*at, rule.Offer); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE campaigns SET locale_counts=$2 WHERE id=$1`, id, countsJSON); err != nil {
			return nil, fmt.Errorf("store locale counts: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit approval: %w", err)
		}
		return &ApproveResult{ID: id, Status: StatusScheduled, ScheduledAt: at,
			Warnings: s.warningsFor(set, ls, rule.Offer.Normalize())}, nil
	}

	// The audience is counted now, with the same rule, consent and contact
	// filters that decide who gets queued.
	aud, err := segment.CountAudience(ctx, tx, compiled)
	if err != nil {
		return nil, err
	}
	q, err := enqueueAudience(ctx, tx, id, compiled, now)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET locale_counts=$2 WHERE id=$1`, id, countsJSON); err != nil {
		return nil, fmt.Errorf("store locale counts: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit approval: %w", err)
	}

	return &ApproveResult{ID: id, Status: StatusApproved, Queued: q, Audience: &aud, Locales: counts,
		Warnings: s.warningsFor(set, ls, rule.Offer.Normalize())}, nil
}

// enqueueAudience creates one delivery per reachable ADDRESS and channel for
// everyone the rule matches: two customer records with the same phone (or
// email) get one message, sent to the record with the lowest id. Used by
// sending now and by the scheduler, so both queue exactly the same way. now is
// the creation time of the rows (the retry age is counted from it).
func enqueueAudience(ctx context.Context, tx pgx.Tx, id int64, compiled segment.Compiled, now time.Time) (Queued, error) {
	enqueue := func(channel, reachable, address string) (int, error) {
		args := append(append([]any{}, compiled.Args...), id, now)
		n := len(args)
		tag, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO deliveries (campaign_id, user_id, channel, created_at, next_attempt_at, updated_at)
			SELECT DISTINCT ON (%s) $%d, id, '%s', $%d, $%d, $%d FROM users WHERE %s AND %s
			ORDER BY %s, id
			ON CONFLICT (campaign_id, user_id, channel) DO NOTHING`,
			address, n-1, channel, n, n, n, compiled.Where, reachable, address), args...)
		if err != nil {
			return 0, fmt.Errorf("enqueue %s: %w", channel, err)
		}
		return int(tag.RowsAffected()), nil
	}
	var q Queued
	var err error
	if q.SMS, err = enqueue("sms", segment.SMSReachableSQL, segment.SMSAddressSQL); err != nil {
		return Queued{}, err
	}
	if q.Email, err = enqueue("email", segment.EmailReachableSQL, segment.EmailAddressSQL); err != nil {
		return Queued{}, err
	}
	return q, nil
}

// checkContents loads the text of the given locales and checks it as sending
// requires: the source and every translation present, every translation made
// from the current source (else StaleTranslationsError), each within the
// limits and every translation saying what the source says.
func checkContents(ctx context.Context, q queryer, policy content.Policy, ls content.Locales, id int64, required []string) (map[string]content.Content, error) {
	stored, err := loadContents(ctx, q, id)
	if err != nil {
		return nil, err
	}
	srcLocale := ls.Source()
	src, ok := stored[srcLocale.Code]
	if !ok {
		return nil, &StateError{"kaynak metin (" + srcLocale.Code + ") yok; önce mesajlar üretilmeli"}
	}
	if missing, stale := pendingLocales(ls, required, stored); len(missing)+len(stale) > 0 {
		return nil, &StaleTranslationsError{Missing: missing, Stale: stale}
	}
	set := map[string]content.Content{}
	var issues []content.Issue
	for _, code := range required {
		l, _ := ls.Get(code)
		c := stored[code].Content
		set[code] = c
		if _, rep := policy.Validate(c); !rep.OK {
			for _, i := range rep.Issues {
				i.Locale = code
				issues = append(issues, i)
			}
			continue
		}
		if !l.IsSource {
			issues = append(issues, content.ConsistentWith(srcLocale, src.Content, l, c)...)
		}
	}
	if len(issues) > 0 {
		return nil, &InvalidContentError{Report: content.Report{OK: false, Issues: issues, Warnings: []content.Issue{}}}
	}
	return set, nil
}

// warningsFor collects the soft issues of the source text (the Turkish offer
// checks; translations are held to the source by checkContents).
func (s *Service) warningsFor(set map[string]content.Content, ls content.Locales, o offer.Offer) []content.Issue {
	out := []content.Issue{} // never nil: the API sends [] rather than null
	if c, ok := set[ls.Source().Code]; ok {
		for _, i := range s.policy.Quality(c, o) {
			i.Locale = ls.Source().Code
			out = append(out, i)
		}
	}
	return out
}

// alreadyApprovedOrRefuse handles a call that did not win the status change:
// either the campaign was approved before (report its state), or it is not in
// a state that can be approved.
func (s *Service) alreadyApprovedOrRefuse(ctx context.Context, id int64) (*ApproveResult, error) {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	switch row.status {
	case StatusScheduled, StatusApproved, StatusSending, StatusCompleted:
	case StatusCancelled, StatusFailed:
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; yeniden onaylanamaz, yeni bir kampanya oluşturun", row.status)}
	default:
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; onaylamak için önce kitle ve mesajlar hazır olmalı", row.status)}
	}
	var q Queued
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE channel='sms'), count(*) FILTER (WHERE channel='email')
		FROM deliveries WHERE campaign_id=$1`, id).Scan(&q.SMS, &q.Email); err != nil {
		return nil, fmt.Errorf("count deliveries: %w", err)
	}
	warnings := []content.Issue{}
	ls, err := s.loadLocales(ctx)
	if err != nil {
		return nil, err
	}
	if stored, err := loadContents(ctx, s.pool, id); err == nil {
		set := map[string]content.Content{}
		for code, sc := range stored {
			set[code] = sc.Content
		}
		warnings = s.warningsFor(set, ls, rowOffer(row))
	}
	return &ApproveResult{ID: id, Status: row.status, AlreadyApproved: true, Queued: q, ScheduledAt: row.scheduledAt, Warnings: warnings}, nil
}
