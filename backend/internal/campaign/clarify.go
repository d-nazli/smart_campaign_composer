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

	var (
		status, brief, question string
		segmentStatus           *string
		history                 []Clarification
	)
	err = tx.QueryRow(ctx, `
		SELECT status, segment_status, brief, question, clarifications
		FROM campaigns WHERE id=$1 FOR NO KEY UPDATE`, id).
		Scan(&status, &segmentStatus, &brief, &question, &history)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock campaign: %w", err)
	}
	if status != StatusDraft || segmentStatus == nil || *segmentStatus != segment.StatusNeedsClarification {
		seg := ""
		if segmentStatus != nil {
			seg = *segmentStatus
		}
		return nil, &StateError{fmt.Sprintf("kampanya %q / %q durumunda; yalnızca soru bekleyen kampanyaya cevap verilebilir", status, seg)}
	}
	if len(history) >= maxClarifications {
		return nil, &StateError{fmt.Sprintf("bu kampanyaya %d cevap verildi; brief'i daha net yazıp yeni kampanya başlatın", maxClarifications)}
	}

	history = append(history, Clarification{Question: question, Answer: answer, AnsweredAt: time.Now().UTC()})
	genCtx, cancel := context.WithTimeout(ctx, segmentTimeout)
	defer cancel()
	rule, err := briefs.ResolveSegmentWith(genCtx, s.llm, brief, llm.Hooks{OnLog: s.recorder(id, "segment", "", VariantA)}, s.segmentMode, llmClarifications(history)...)
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
	switch status {
	case StatusDraft, StatusSegmentReady, StatusContentReady:
	default:
		return &StateError{fmt.Sprintf("kampanya %q durumunda; yalnızca onaylanmamış kampanya silinebilir", status)}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM campaigns WHERE id=$1`, id); err != nil {
		return fmt.Errorf("delete campaign: %w", err)
	}
	return tx.Commit(ctx)
}
