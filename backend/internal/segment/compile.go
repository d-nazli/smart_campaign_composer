package segment

import (
	"context"
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
	             count(*) FILTER (WHERE ` + SMSReachableSQL + `),
	             count(*) FILTER (WHERE ` + EmailReachableSQL + `),
	             count(*) FILTER (WHERE ` + SMSReachableSQL + ` AND ` + EmailReachableSQL + `)
	      FROM users WHERE ` + c.Where
	var a Audience
	if err := db.QueryRow(ctx, q, c.Args...).Scan(&a.Matched, &a.SMSReachable, &a.EmailReachable, &a.BothReachable); err != nil {
		return Audience{}, fmt.Errorf("count audience: %w", err)
	}
	return a, nil
}
