// Package segment turns an LLM-produced audience rule into SQL safely.
//
// The LLM never writes SQL. It produces a small JSON rule: a list of
// {field, op, value} conditions. This package (1) strictly validates that rule
// against a whitelist and (2) compiles it into a parameterized WHERE clause
// whose SQL text comes only from constants in this file; every value travels as
// a bind parameter.
package segment

import (
	"fmt"
	"time"
)

type kind int

const (
	kindCity kind = iota // string, canonicalized against KnownCities
	kindInt
	kindNumber
	kindBool
	kindGender // "female" or "male"
)

// Genders a gender condition may name (users.gender also allows "other" and
// NULL = not stated; neither is ever selected by a gender condition).
var Genders = []string{"female", "male"}

// Operators.
const (
	OpEq    = "eq"
	OpNeq   = "neq"
	OpLt    = "lt"
	OpLte   = "lte"
	OpGt    = "gt"
	OpGte   = "gte"
	OpIn    = "in"
	OpNotIn = "not_in"
)

// FieldInfo is the public description of a field (used in the LLM prompt and
// JSON schema, so the whitelist exists in exactly one place).
type FieldInfo struct {
	Name        string
	Type        string // "city", "integer", "number", "boolean"
	Ops         []string
	Min, Max    float64
	Description string
}

type fieldDef struct {
	FieldInfo
	kind kind
	// sql builds the WHERE fragment. Values are appended to args and referenced
	// as $n; the returned text contains only constants from this file.
	sql func(op string, v any, now time.Time, a *args) (string, error)
}

// args collects bind parameters and hands out their $n placeholders.
type args struct{ values []any }

func (a *args) add(v any) string {
	a.values = append(a.values, v)
	return fmt.Sprintf("$%d", len(a.values))
}

var (
	cmpOps  = []string{OpLt, OpLte, OpGt, OpGte, OpEq}
	sqlCmp  = map[string]string{OpLt: "<", OpLte: "<=", OpGt: ">", OpGte: ">=", OpEq: "="}
	dayOps  = cmpOps
	numOps  = []string{OpLt, OpLte, OpGt, OpGte}
	cityOps = []string{OpEq, OpNeq, OpIn, OpNotIn}
)

// daysSince compiles "days since <timestamp column> <op> N". Comparisons are on
// the timestamp itself (N days = N*24h before now), and NULL timestamps (the
// event never happened) never match.
func daysSince(col string) func(op string, v any, now time.Time, a *args) (string, error) {
	return func(op string, v any, now time.Time, a *args) (string, error) {
		days := v.(int64)
		t := now.Add(-time.Duration(days) * 24 * time.Hour)
		switch op {
		case OpLte: // at most N days ago
			return fmt.Sprintf("%s >= %s", col, a.add(t)), nil
		case OpLt:
			return fmt.Sprintf("%s > %s", col, a.add(t)), nil
		case OpGte: // at least N days ago
			return fmt.Sprintf("%s <= %s", col, a.add(t)), nil
		case OpGt:
			return fmt.Sprintf("%s < %s", col, a.add(t)), nil
		case OpEq: // exactly N full days ago: [N, N+1) days
			older := now.Add(-time.Duration(days+1) * 24 * time.Hour)
			return fmt.Sprintf("(%s <= %s AND %s > %s)", col, a.add(t), col, a.add(older)), nil
		}
		return "", fmt.Errorf("operator %q not supported", op)
	}
}

// daysSinceOrNever is the NULL-inclusive complement of daysSince(...) lte/lt:
// "no event in the last N days", where having NEVER had the event counts. It
// only supports the "longer ago" operators; for lte/lt/eq a NULL cannot match.
func daysSinceOrNever(col string) func(op string, v any, now time.Time, a *args) (string, error) {
	return func(op string, v any, now time.Time, a *args) (string, error) {
		t := now.Add(-time.Duration(v.(int64)) * 24 * time.Hour)
		switch op {
		case OpGt: // not within the last N days (older than N days, or never)
			return fmt.Sprintf("(%s IS NULL OR %s < %s)", col, col, a.add(t)), nil
		case OpGte:
			return fmt.Sprintf("(%s IS NULL OR %s <= %s)", col, col, a.add(t)), nil
		}
		return "", fmt.Errorf("operator %q not supported", op)
	}
}

func numeric(col string) func(op string, v any, now time.Time, a *args) (string, error) {
	return func(op string, v any, _ time.Time, a *args) (string, error) {
		sym, ok := sqlCmp[op]
		if !ok {
			return "", fmt.Errorf("operator %q not supported", op)
		}
		return fmt.Sprintf("%s %s %s", col, sym, a.add(v)), nil
	}
}

// The whitelist. Add a field by adding an entry here; the validator, compiler,
// JSON schema and LLM prompt all follow.
var fields = []fieldDef{
	{
		FieldInfo: FieldInfo{Name: "all_customers", Type: "boolean", Ops: []string{OpEq},
			Description: "Every customer who may be contacted (consent is applied automatically). Value is always true, and it must be the ONLY condition. Use it only when the brief explicitly says everyone: \"herkese\", \"tüm müşterilere\", \"bütün kullanıcılara\". NEVER use it because the brief names no audience: then return needs_clarification instead."},
		kind: kindBool,
		sql: func(op string, v any, _ time.Time, _ *args) (string, error) {
			if op != OpEq || v != true {
				return "", fmt.Errorf("all_customers only supports eq true")
			}
			return "TRUE", nil
		},
	},
	{
		FieldInfo: FieldInfo{Name: "city", Type: "city", Ops: cityOps,
			Description: "Customer's city. Value is one of the allowed city names (or a list of them for in / not_in)."},
		kind: kindCity,
		sql: func(op string, v any, _ time.Time, a *args) (string, error) {
			switch op {
			case OpEq:
				return "city = " + a.add(v), nil
			case OpNeq:
				return "city <> " + a.add(v), nil
			case OpIn:
				return "city = ANY(" + a.add(v) + ")", nil
			case OpNotIn:
				return "NOT (city = ANY(" + a.add(v) + "))", nil
			}
			return "", fmt.Errorf("operator %q not supported", op)
		},
	},
	{
		FieldInfo: FieldInfo{Name: "gender", Type: "string", Ops: []string{OpEq},
			Description: "Customer's stated gender: \"female\" or \"male\". Use it ONLY when the brief explicitly restricts WHO receives the campaign by gender (\"kadın müşterilere özel\", \"erkek müşterilerimize\", \"kadınlara\"). A special day (Kadınlar Günü, Anneler Günü, Babalar Günü, Sevgililer Günü ...) is the THEME of the message, never an audience: do not add gender for it. Customers who did not state a gender never match."},
		kind: kindGender,
		sql: func(op string, v any, _ time.Time, a *args) (string, error) {
			if op != OpEq {
				return "", fmt.Errorf("operator %q not supported", op)
			}
			return "gender = " + a.add(v), nil
		},
	},
	{
		FieldInfo: FieldInfo{Name: "days_since_last_cart_add", Type: "integer", Ops: dayOps, Min: 0, Max: 3650,
			Description: "Whole days since the customer last added a product to the cart. Customers who never added anything never match. 'In the last 30 days' = lte 30. 'Has added to cart at some point' = gte 0."},
		kind: kindInt,
		sql:  daysSince("last_cart_add_at"),
	},
	{
		FieldInfo: FieldInfo{Name: "days_since_last_order", Type: "integer", Ops: dayOps, Min: 0, Max: 3650,
			Description: "Whole days since the customer's last order. Customers with no orders never match (use order_count eq 0 for them)."},
		kind: kindInt,
		sql:  daysSince("last_order_at"),
	},
	{
		FieldInfo: FieldInfo{Name: "days_since_last_order_or_never", Type: "integer", Ops: []string{OpGt, OpGte}, Min: 0, Max: 3650,
			Description: "Customers who have NOT ordered in the last N days, INCLUDING customers who never ordered at all. 'Has not ordered in the last 30 days' = gt 30. Use this, not days_since_last_order gt N, for 'did not order in the last N days' (that one silently drops everyone who never ordered)."},
		kind: kindInt,
		sql:  daysSinceOrNever("last_order_at"),
	},
	{
		FieldInfo: FieldInfo{Name: "order_count", Type: "integer", Ops: cmpOps, Min: 0, Max: 100000,
			Description: "Total number of orders the customer has placed (0 = never bought)."},
		kind: kindInt,
		sql:  numeric("order_count"),
	},
	{
		FieldInfo: FieldInfo{Name: "total_spent", Type: "number", Ops: numOps, Min: 0, Max: 1e9,
			Description: "Total amount the customer has spent, in TRY."},
		kind: kindNumber,
		sql:  numeric("total_spent"),
	},
	{
		FieldInfo: FieldInfo{Name: "purchased_since_last_cart_add", Type: "boolean", Ops: []string{OpEq},
			Description: "Only for customers who have added to the cart. true = placed an order after their last cart add; false = has NOT bought since their last cart add (abandoned cart). 'Added to cart but did not buy' = eq false."},
		kind: kindBool,
		sql: func(op string, v any, _ time.Time, _ *args) (string, error) {
			if op != OpEq {
				return "", fmt.Errorf("operator %q not supported", op)
			}
			if v.(bool) {
				return "(last_cart_add_at IS NOT NULL AND last_order_at IS NOT NULL AND last_order_at >= last_cart_add_at)", nil
			}
			return "(last_cart_add_at IS NOT NULL AND (last_order_at IS NULL OR last_order_at < last_cart_add_at))", nil
		},
	},
}

func fieldByName(name string) (fieldDef, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return fieldDef{}, false
}

// Fields lists the whitelist (for prompts and schemas).
func Fields() []FieldInfo {
	out := make([]FieldInfo, len(fields))
	for i, f := range fields {
		out[i] = f.FieldInfo
	}
	return out
}

// Consent and contact rules. The system ALWAYS applies these when counting or
// enqueueing; they are not part of the LLM rule, so the LLM cannot widen the
// audience beyond people who agreed to be contacted on that channel.
const (
	SMSReachableSQL   = "(sms_consent AND phone IS NOT NULL)"
	EmailReachableSQL = "(email_consent AND email IS NOT NULL)"
)

// The address a message actually goes to, normalized. Two customer records with
// the same phone (or the same email, ignoring case and surrounding spaces) are
// one recipient on that channel: a campaign sends the address one message, not
// one per record. Counting and enqueueing both use these expressions, so the
// number on the screen is the number queued.
const (
	SMSAddressSQL   = "btrim(phone)"
	EmailAddressSQL = "lower(btrim(email))"
)
