package segment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Rule statuses.
const (
	StatusOK                 = "ok"
	StatusNeedsClarification = "needs_clarification"
	StatusUnsupported        = "unsupported"
)

// Match modes.
const (
	MatchAll = "all"
	MatchAny = "any"
)

// Limits that keep a rule small and cheap.
const (
	MaxConditions  = 8
	MaxListItems   = 20
	MaxExplanation = 500
	MaxQuestion    = 300
	MaxRawBytes    = 16 << 10
)

// rawRule is exactly what the LLM returns (the structured-output schema).
type rawRule struct {
	Status      string         `json:"status"`
	Match       string         `json:"match"`
	Conditions  []rawCondition `json:"conditions"`
	Explanation string         `json:"explanation"`
	Question    string         `json:"question"`
}

type rawCondition struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

// Condition is a validated, canonicalized condition. Value is a string,
// int64, float64, bool or []string depending on the field.
type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// Rule is a validated rule. Only values of this type can be compiled to SQL.
type Rule struct {
	Status      string      `json:"status"`
	Match       string      `json:"match,omitempty"`
	Conditions  []Condition `json:"conditions"`
	Explanation string      `json:"explanation"`
	Question    string      `json:"question,omitempty"`
}

// ParseError lists every problem found, so it can be fed back to the LLM.
type ParseError struct{ Problems []string }

func (e *ParseError) Error() string { return "invalid segment rule: " + strings.Join(e.Problems, "; ") }

// Parse strictly decodes and validates raw LLM output. It never returns a Rule
// unless every check passed.
func Parse(raw string) (Rule, error) {
	if len(raw) > MaxRawBytes {
		return Rule{}, &ParseError{[]string{fmt.Sprintf("çıktı çok büyük (%d bayt)", len(raw))}}
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var rr rawRule
	if err := dec.Decode(&rr); err != nil {
		return Rule{}, &ParseError{[]string{"geçerli bir JSON nesnesi değil veya şemaya uymuyor: " + err.Error()}}
	}
	if dec.More() {
		return Rule{}, &ParseError{[]string{"JSON nesnesinden sonra fazladan içerik var"}}
	}
	return validate(rr)
}

func validate(rr rawRule) (Rule, error) {
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	out := Rule{Status: rr.Status, Conditions: []Condition{}}

	if utf8.RuneCountInString(rr.Explanation) > MaxExplanation {
		bad("explanation en fazla %d karakter olmalı", MaxExplanation)
	}
	if utf8.RuneCountInString(rr.Question) > MaxQuestion {
		bad("question en fazla %d karakter olmalı", MaxQuestion)
	}
	out.Explanation = strings.TrimSpace(rr.Explanation)
	out.Question = strings.TrimSpace(rr.Question)

	switch rr.Status {
	case StatusOK:
		if rr.Match != MatchAll && rr.Match != MatchAny {
			bad("match %q geçersiz; %q veya %q olmalı", rr.Match, MatchAll, MatchAny)
		}
		out.Match = rr.Match
		if n := len(rr.Conditions); n == 0 {
			bad("status \"ok\" için en az bir koşul gerekli (boş kural herkesi seçerdi)")
		} else if n > MaxConditions {
			bad("en fazla %d koşul olabilir, %d verildi", MaxConditions, n)
		}
		if out.Question != "" {
			bad("status \"ok\" iken question boş olmalı")
		}
		for i, rc := range rr.Conditions {
			if i >= MaxConditions {
				break
			}
			c, errs := validateCondition(i, rc)
			problems = append(problems, errs...)
			if len(errs) == 0 {
				out.Conditions = append(out.Conditions, c)
			}
		}
	case StatusNeedsClarification:
		if out.Question == "" {
			bad("status \"needs_clarification\" için question dolu olmalı")
		}
		if len(rr.Conditions) != 0 {
			bad("status \"needs_clarification\" iken conditions boş olmalı")
		}
	case StatusUnsupported:
		if out.Explanation == "" {
			bad("status \"unsupported\" için explanation dolu olmalı")
		}
		if len(rr.Conditions) != 0 {
			bad("status \"unsupported\" iken conditions boş olmalı")
		}
	default:
		bad("status %q geçersiz; %q, %q veya %q olmalı", rr.Status, StatusOK, StatusNeedsClarification, StatusUnsupported)
	}

	if len(problems) > 0 {
		return Rule{}, &ParseError{problems}
	}
	return out, nil
}

func validateCondition(i int, rc rawCondition) (Condition, []string) {
	path := fmt.Sprintf("conditions[%d]", i)
	f, ok := fieldByName(rc.Field)
	if !ok {
		return Condition{}, []string{fmt.Sprintf("%s.field %q bilinmiyor; izin verilenler: %s", path, truncate(rc.Field, 40), fieldNames())}
	}
	if !contains(f.Ops, rc.Op) {
		return Condition{}, []string{fmt.Sprintf("%s.op %q, %s alanı için geçersiz; izin verilenler: %s", path, truncate(rc.Op, 20), f.Name, strings.Join(f.Ops, ", "))}
	}
	v, err := decodeValue(f, rc.Op, rc.Value)
	if err != nil {
		return Condition{}, []string{fmt.Sprintf("%s.value (%s %s): %v", path, f.Name, rc.Op, err)}
	}
	return Condition{Field: f.Name, Op: rc.Op, Value: v}, nil
}

func decodeValue(f fieldDef, op string, raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("değer boş olamaz")
	}
	switch f.kind {
	case kindCity:
		if op == OpIn || op == OpNotIn {
			var list []string
			if err := json.Unmarshal(raw, &list); err != nil {
				return nil, errors.New("şehir listesi (metin dizisi) olmalı")
			}
			if len(list) == 0 || len(list) > MaxListItems {
				return nil, fmt.Errorf("liste 1..%d öğe içermeli", MaxListItems)
			}
			out := make([]string, 0, len(list))
			for _, s := range list {
				c, err := canonicalCity(s)
				if err != nil {
					return nil, err
				}
				out = append(out, c)
			}
			return out, nil
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, errors.New("şehir adı (metin) olmalı")
		}
		return canonicalCity(s)

	case kindInt:
		n, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 64)
		if err != nil {
			return nil, errors.New("tam sayı olmalı")
		}
		if float64(n) < f.Min || float64(n) > f.Max {
			return nil, fmt.Errorf("%v ile %v arasında olmalı", f.Min, f.Max)
		}
		return n, nil

	case kindNumber:
		x, err := strconv.ParseFloat(string(bytes.TrimSpace(raw)), 64)
		if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, errors.New("sayı olmalı")
		}
		if x < f.Min || x > f.Max {
			return nil, fmt.Errorf("%v ile %v arasında olmalı", f.Min, f.Max)
		}
		return x, nil

	case kindBool:
		switch string(bytes.TrimSpace(raw)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, errors.New("true veya false olmalı")
	}
	return nil, errors.New("desteklenmeyen alan tipi")
}

func canonicalCity(s string) (string, error) {
	if utf8.RuneCountInString(s) > 50 {
		return "", errors.New("şehir adı çok uzun")
	}
	c, ok := CanonicalCity(s)
	if !ok {
		return "", fmt.Errorf("bilinmeyen şehir %q; geçerli şehirler: %s", truncate(s, 40), strings.Join(KnownCities, ", "))
	}
	return c, nil
}

func fieldNames() string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	return strings.Join(names, ", ")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
