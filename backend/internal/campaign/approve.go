package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"smartcampaign/internal/content"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// Statuses of the sending phase.
const (
	StatusApproved  = "approved"
	StatusSending   = "sending"
	StatusCompleted = "completed"
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
	// Audience is the audience counted at approval time (nil on a repeat). It can
	// differ from what the screen showed earlier: the data moves on and "the last
	// 30 days" slides. Queued is the truth.
	Audience *segment.Audience `json:"audience,omitempty"`
	// Warnings are the soft issues of the stored text (D46), each marked with
	// its language. They never block.
	Warnings []content.Issue `json:"warnings"`
	// Languages is how many people receive each language (reachable on at
	// least one channel), counted at approval time.
	Languages map[string]int `json:"languages,omitempty"`
}

// Approve freezes the campaign and enqueues one delivery per reachable
// recipient and channel, all in one transaction.
//
// The status change is a compare-and-swap (content_ready -> approved). The
// loser of a concurrent approval blocks on that row, then finds the campaign
// approved and reports the same queue instead of enqueueing again; the
// UNIQUE(campaign_id, user_id, channel) + ON CONFLICT DO NOTHING is the
// second line of defence.
func (s *Service) Approve(ctx context.Context, id int64) (*ApproveResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var ruleJSON []byte
	err = tx.QueryRow(ctx, `
		UPDATE campaigns SET status='approved', approved_at=now(), updated_at=now()
		WHERE id=$1 AND status='content_ready' RETURNING segment_rule`, id).Scan(&ruleJSON)
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

	// The text is validated again, in every language: what is sent must satisfy
	// every limit, and the languages must say the same thing.
	set, err := s.approvableContents(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	// The audience is counted now, with the same rule, consent and contact
	// filters that decide who gets queued.
	compiled, err := segment.Compile(rule, time.Now())
	if err != nil {
		return nil, fmt.Errorf("compile stored rule: %w", err)
	}
	aud, err := segment.CountAudience(ctx, tx, compiled)
	if err != nil {
		return nil, err
	}

	enqueue := func(channel, reachable string) (int, error) {
		args := append(append([]any{}, compiled.Args...), id)
		tag, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO deliveries (campaign_id, user_id, channel)
			SELECT $%d, id, '%s' FROM users WHERE %s AND %s
			ON CONFLICT (campaign_id, user_id, channel) DO NOTHING`,
			len(args), channel, compiled.Where, reachable), args...)
		if err != nil {
			return 0, fmt.Errorf("enqueue %s: %w", channel, err)
		}
		return int(tag.RowsAffected()), nil
	}
	var q Queued
	if q.SMS, err = enqueue("sms", segment.SMSReachableSQL); err != nil {
		return nil, err
	}
	if q.Email, err = enqueue("email", segment.EmailReachableSQL); err != nil {
		return nil, err
	}
	langs, err := segment.CountByLanguage(ctx, tx, compiled)
	if err != nil {
		return nil, err
	}
	langJSON, err := json.Marshal(langs)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET language_counts=$2 WHERE id=$1`, id, langJSON); err != nil {
		return nil, fmt.Errorf("store language counts: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit approval: %w", err)
	}

	return &ApproveResult{ID: id, Status: StatusApproved, Queued: q, Audience: &aud, Languages: langs,
		Warnings: s.warningsFor(set, rule.Offer.Normalize())}, nil
}

// approvableContents loads the text of every language and checks it as sending
// requires: all languages present, each within the limits, all saying the same.
func (s *Service) approvableContents(ctx context.Context, q queryer, id int64) (map[string]content.Content, error) {
	stored, err := loadContents(ctx, q, id)
	if err != nil {
		return nil, err
	}
	set := map[string]content.Content{}
	for code, sc := range stored {
		set[code] = sc.Content
	}
	if err := requireAllLanguages(set); err != nil {
		return nil, err
	}
	var issues []content.Issue
	for _, l := range content.Languages {
		if _, rep := s.policy.Validate(set[l.Code]); !rep.OK {
			for _, i := range rep.Issues {
				i.Language = l.Code
				issues = append(issues, i)
			}
		}
	}
	issues = append(issues, content.Consistency(set)...)
	if len(issues) > 0 {
		return nil, &InvalidContentError{Report: content.Report{OK: false, Issues: issues, Warnings: []content.Issue{}}}
	}
	return set, nil
}

// warningsFor collects the soft issues of every language, marked with it.
func (s *Service) warningsFor(set map[string]content.Content, o offer.Offer) []content.Issue {
	out := []content.Issue{} // never nil: the API sends [] rather than null
	for _, l := range content.Languages {
		c, ok := set[l.Code]
		if !ok {
			continue
		}
		for _, i := range s.policy.Quality(c, o) {
			i.Language = l.Code
			out = append(out, i)
		}
	}
	return out
}

// alreadyApprovedOrRefuse handles a call that did not win the status change:
// either the campaign was approved before (report its queue), or it is not in
// a state that can be approved.
func (s *Service) alreadyApprovedOrRefuse(ctx context.Context, id int64) (*ApproveResult, error) {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	switch row.status {
	case StatusApproved, StatusSending, StatusCompleted:
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
	if stored, err := loadContents(ctx, s.pool, id); err == nil {
		set := map[string]content.Content{}
		for code, sc := range stored {
			set[code] = sc.Content
		}
		warnings = s.warningsFor(set, rowOffer(row))
	}
	return &ApproveResult{ID: id, Status: row.status, AlreadyApproved: true, Queued: q, Warnings: warnings}, nil
}
