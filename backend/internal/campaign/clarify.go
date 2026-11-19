package campaign

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	briefs "smartcampaign/internal/brief"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/segment"
)

const (
	maxAnswerRunes    = 500
	maxClarifications = 5
)

// Clarification is one question the model asked about the brief and the
// person's answer to it.
type Clarification struct {
	Question   string    `json:"question"`
	Answer     string    `json:"answer"`
	AnsweredAt time.Time `json:"answered_at"`
}

func llmClarifications(cs []Clarification) []llm.Clarification {
	out := make([]llm.Clarification, len(cs))
	for i, c := range cs {
		out[i] = llm.Clarification{Question: c.Question, Answer: c.Answer}
	}
	return out
}

// Clarify answers the question of a campaign that waits for one: the audience
// is resolved again from the brief and every answer so far, on the same
// campaign. The model may build the audience (segment_ready), ask again, or
// say it cannot; the answer is kept in the campaign's history either way. A
// campaign that does not wait for an answer is a StateError (409).
//
// The row is locked for the whole step, model call included, so two answers to
// the same question cannot both be applied: the second waits, then finds the
// question answered. FOR NO KEY UPDATE, not FOR UPDATE: the LLM attempts are
// written to llm_generations from another connection while the lock is held,
// and their foreign-key check takes FOR KEY SHARE on this row, which FOR UPDATE
// would block (the step would wait on itself). NO KEY UPDATE still excludes a
// second answer, an edit and an approval.
func (s *Service) Clarify(ctx context.Context, id int64, answer string) (*View, error) {
	answer = strings.TrimSpace(answer)
	if n := utf8.RuneCountInString(answer); n == 0 || n > maxAnswerRunes {
		return nil, &InputError{fmt.Sprintf("cevap 1 ile %d karakter arasında olmalı", maxAnswerRunes)}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck // a no-op after Commit

	c, err := lockForSegment(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if c.status != StatusDraft || c.segmentStatus != segment.StatusNeedsClarification {
		return nil, &StateError{fmt.Sprintf("kampanya %q / %q durumunda; yalnızca soru bekleyen kampanyaya cevap verilebilir", c.status, c.segmentStatus)}
	}
	history := c.history
	if len(history) >= maxClarifications {
		return nil, &StateError{fmt.Sprintf("bu kampanyaya %d cevap verildi; brief'i düzenleyip yeniden gönderin", maxClarifications)}
	}

	history = append(history, Clarification{Question: c.question, Answer: answer, AnsweredAt: time.Now().UTC()})
	genCtx, cancel := context.WithTimeout(ctx, segmentTimeout)
	defer cancel()
	rule, err := briefs.ResolveSegmentWith(genCtx, s.llm, c.brief, llm.Hooks{OnLog: s.recorder(id, "segment", "", VariantA)}, s.segmentMode, llmClarifications(history)...)
	if err != nil {
		// Nothing is stored: the campaign still waits for the answer, which can be sent again.
		return nil, s.generationError(err, "Kitle kuralı cevapla birlikte üretilemedi")
	}
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET clarifications=$2 WHERE id=$1`, id, history); err != nil {
		return nil, fmt.Errorf("store answer: %w", err)
	}
	if err := s.storeRule(ctx, tx, id, rule); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit answer: %w", err)
	}
	return s.Get(ctx, id)
}

// UpdateBrief replaces the brief of a campaign that was never approved and
// resolves its audience again on the SAME campaign, the way Create does for a
// new one: "Desteklenmiyor" or "Soru bekliyor" after an edit becomes
// segment_ready (or asks again) without a second campaign. What belonged to
// the old brief goes: the texts of both variants, the answers to earlier
// questions and the variant counts. An approved, scheduled, sent or cancelled
// campaign keeps its brief: a StateError (409). The lock is the one Clarify
// takes (see there); nothing is stored when the model step fails.
func (s *Service) UpdateBrief(ctx context.Context, id int64, brief string) (*View, error) {
	brief = strings.TrimSpace(brief)
	if n := utf8.RuneCountInString(brief); n < minBriefRunes || n > maxBriefRunes {
		return nil, &InputError{fmt.Sprintf("brief %d ile %d karakter arasında olmalı", minBriefRunes, maxBriefRunes)}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck // a no-op after Commit

	c, err := lockForSegment(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if !editableBrief(c.status) {
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; yalnızca onaylanmamış kampanyanın brief'i değiştirilebilir", c.status)}
	}

	genCtx, cancel := context.WithTimeout(ctx, segmentTimeout)
	defer cancel()
	rule, err := briefs.ResolveSegmentWith(genCtx, s.llm, brief, llm.Hooks{OnLog: s.recorder(id, "segment", "", VariantA)}, s.segmentMode)
	if err != nil {
		return nil, s.generationError(err, "Kitle kuralı üretilemedi")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM campaign_contents WHERE campaign_id=$1`, id); err != nil {
		return nil, fmt.Errorf("drop old texts: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET brief=$2, clarifications='[]'::jsonb, variant_counts=NULL WHERE id=$1`, id, brief); err != nil {
		return nil, fmt.Errorf("store brief: %w", err)
	}
	if err := s.storeRule(ctx, tx, id, rule); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit brief: %w", err)
	}
	return s.Get(ctx, id)
}

// editableBrief: the brief can change, and the campaign be deleted, only
// before the approval.
func editableBrief(status string) bool {
	switch status {
	case StatusDraft, StatusSegmentReady, StatusContentReady:
		return true
	}
	return false
}

// lockedCampaign is what a same-campaign segment step reads under its lock.
type lockedCampaign struct {
	status, segmentStatus, brief, question string
	history                                []Clarification
}

// lockForSegment locks the campaign for a step that resolves its audience
// again (Clarify, UpdateBrief). FOR NO KEY UPDATE, not FOR UPDATE: the model
// step runs under the lock and its attempts are written to llm_generations
// from another connection; their foreign-key check takes FOR KEY SHARE on this
// row, which FOR UPDATE would block. NO KEY UPDATE still serializes these
// steps with each other, with edits and with the approval.
func lockForSegment(ctx context.Context, tx pgx.Tx, id int64) (lockedCampaign, error) {
	var c lockedCampaign
	var segmentStatus *string
	err := tx.QueryRow(ctx, `
		SELECT status, segment_status, brief, question, clarifications
		FROM campaigns WHERE id=$1 FOR NO KEY UPDATE`, id).
		Scan(&c.status, &segmentStatus, &c.brief, &c.question, &c.history)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("lock campaign: %w", err)
	}
	if segmentStatus != nil {
		c.segmentStatus = *segmentStatus
	}
	return c, nil
}

// Delete removes a campaign that was never approved (a draft, a campaign
// waiting for an answer, one with an audience or with texts). Its texts and LLM
// records go with it (ON DELETE CASCADE); it has no deliveries yet. Anything
// approved, scheduled, sent or cancelled stays: a StateError (409).
func (s *Service) Delete(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck // a no-op after Commit

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM campaigns WHERE id=$1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock campaign: %w", err)
	}
	if !editableBrief(status) {
		return &StateError{fmt.Sprintf("kampanya %q durumunda; yalnızca onaylanmamış kampanya silinebilir", status)}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM campaigns WHERE id=$1`, id); err != nil {
		return fmt.Errorf("delete campaign: %w", err)
	}
	return tx.Commit(ctx)
}
