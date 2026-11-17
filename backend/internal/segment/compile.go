package segment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Compiled is a parameterized WHERE clause. Where is built only from constants
// in fields.go; Args hold every value from the rule.
type Compiled struct {
	Where string
	Args  []any
}

// Compile turns a validated rule into SQL. now is passed in so tests are exact.
func Compile(r Rule, now time.Time) (Compiled, error) {
	if r.Status != StatusOK || len(r.Conditions) == 0 {
		return Compiled{}, errors.New("only rules with status ok and at least one condition can be compiled")
	}
	joiner := " AND "
	if r.Match == MatchAny {
		joiner = " OR "
	}
	a := &args{}
	parts := make([]string, 0, len(r.Conditions))
	for _, c := range r.Conditions {
		f, ok := fieldByName(c.Field)
		if !ok {
			return Compiled{}, fmt.Errorf("unknown field %q", c.Field)
		}
		frag, err := f.sql(c.Op, c.Value, now, a)
		if err != nil {
			return Compiled{}, fmt.Errorf("%s: %w", c.Field, err)
		}
		parts = append(parts, "("+frag+")")
	}
	return Compiled{Where: "(" + strings.Join(parts, joiner) + ")", Args: a.values}, nil
}

// Audience is how many people a rule reaches, after consent and contact data.
// SMSReachable and EmailReachable count distinct addresses (see SMSAddressSQL):
// that is how many messages each channel sends. Matched and BothReachable
// count customer records.
type Audience struct {
	Matched        int `json:"matched"`
	SMSReachable   int `json:"sms_reachable"`
	EmailReachable int `json:"email_reachable"`
	BothReachable  int `json:"both_reachable"`
}

// Querier is satisfied by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CountAudience counts matches and, separately, the people who can actually be
// reached on each channel (consent given AND a phone/email on file).
func CountAudience(ctx context.Context, db Querier, c Compiled) (Audience, error) {
	q := `SELECT count(*),
	             count(DISTINCT ` + SMSAddressSQL + `) FILTER (WHERE ` + SMSReachableSQL + `),
	             count(DISTINCT ` + EmailAddressSQL + `) FILTER (WHERE ` + EmailReachableSQL + `),
	             count(*) FILTER (WHERE ` + SMSReachableSQL + ` AND ` + EmailReachableSQL + `)
	      FROM users WHERE ` + c.Where
	var a Audience
	if err := db.QueryRow(ctx, q, c.Args...).Scan(&a.Matched, &a.SMSReachable, &a.EmailReachable, &a.BothReachable); err != nil {
		return Audience{}, fmt.Errorf("count audience: %w", err)
	}
	return a, nil
}

// CountByLanguage counts the people the rule reaches on at least one channel,
// per language (for example {"tr": 120, "en": 45}). Languages nobody speaks are
// absent. It uses the same rule, consent and contact filters as the delivery
// queue, so it adds up to what will actually be queued.
func CountByLanguage(ctx context.Context, db Querier, c Compiled) (map[string]int, error) {
	q := `SELECT coalesce(jsonb_object_agg(language, n), '{}'::jsonb)
	      FROM (SELECT language, count(*) AS n FROM users
	            WHERE ` + c.Where + ` AND ((` + SMSReachableSQL + `) OR (` + EmailReachableSQL + `))
	            GROUP BY language) t`
	var raw []byte
	if err := db.QueryRow(ctx, q, c.Args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("count by language: %w", err)
	}
	out := map[string]int{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("count by language: %w", err)
	}
	return out, nil
}
