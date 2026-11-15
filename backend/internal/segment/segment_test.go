package segment

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

const exampleRule = `{
  "status": "ok", "match": "all",
  "conditions": [
    {"field": "city", "op": "eq", "value": "İstanbul"},
    {"field": "days_since_last_cart_add", "op": "lte", "value": 30},
    {"field": "purchased_since_last_cart_add", "op": "eq", "value": false}
  ],
  "explanation": "İstanbul'da son 30 günde sepete ekleyip satın almayanlar", "question": ""
}`

func TestParseExampleRule(t *testing.T) {
	r, err := Parse(exampleRule)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusOK || r.Match != MatchAll || len(r.Conditions) != 3 {
		t.Fatalf("rule: %+v", r)
	}
	if r.Conditions[0].Value != "İstanbul" || r.Conditions[1].Value != int64(30) || r.Conditions[2].Value != false {
		t.Fatalf("values: %+v", r.Conditions)
	}
}

func TestCompileExampleRuleGolden(t *testing.T) {
	r, err := Parse(exampleRule)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Compile(r, now)
	if err != nil {
		t.Fatal(err)
	}
	wantWhere := "((city = $1) AND (last_cart_add_at >= $2) AND " +
		"((last_cart_add_at IS NOT NULL AND (last_order_at IS NULL OR last_order_at < last_cart_add_at))))"
	if c.Where != wantWhere {
		t.Fatalf("where:\n got %s\nwant %s", c.Where, wantWhere)
	}
	want := []any{"İstanbul", now.Add(-30 * 24 * time.Hour)}
	if !reflect.DeepEqual(c.Args, want) {
		t.Fatalf("args: got %#v want %#v", c.Args, want)
	}
}

func TestCompileMoreShapes(t *testing.T) {
	tests := []struct {
		name  string
		rule  string
		where string
		nargs int
	}{
		{"any", `{"status":"ok","match":"any","conditions":[{"field":"order_count","op":"gte","value":3},{"field":"total_spent","op":"gt","value":1500.5}],"explanation":"x","question":""}`,
			"((order_count >= $1) OR (total_spent > $2))", 2},
		{"city in", `{"status":"ok","match":"all","conditions":[{"field":"city","op":"in","value":["Istanbul","ankara"]}],"explanation":"x","question":""}`,
			"((city = ANY($1)))", 1},
		{"city not_in", `{"status":"ok","match":"all","conditions":[{"field":"city","op":"not_in","value":["İzmir"]}],"explanation":"x","question":""}`,
			"((NOT (city = ANY($1))))", 1},
		{"purchased true", `{"status":"ok","match":"all","conditions":[{"field":"purchased_since_last_cart_add","op":"eq","value":true}],"explanation":"x","question":""}`,
			"(((last_cart_add_at IS NOT NULL AND last_order_at IS NOT NULL AND last_order_at >= last_cart_add_at)))", 0},
		{"days eq uses two args", `{"status":"ok","match":"all","conditions":[{"field":"days_since_last_order","op":"eq","value":7}],"explanation":"x","question":""}`,
			"(((last_order_at <= $1 AND last_order_at > $2)))", 2},
		{"days gte", `{"status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"gte","value":0}],"explanation":"x","question":""}`,
			"((last_cart_add_at <= $1))", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Parse(tt.rule)
			if err != nil {
				t.Fatal(err)
			}
			c, err := Compile(r, now)
			if err != nil {
				t.Fatal(err)
			}
			if c.Where != tt.where {
				t.Fatalf("where:\n got %s\nwant %s", c.Where, tt.where)
			}
			if len(c.Args) != tt.nargs {
				t.Fatalf("args = %d, want %d", len(c.Args), tt.nargs)
			}
		})
	}
}

func TestCityCanonicalization(t *testing.T) {
	for in, want := range map[string]string{
		"istanbul": "İstanbul", "Istanbul": "İstanbul", "İSTANBUL": "İstanbul", " izmir ": "İzmir",
		"eskisehir": "Eskişehir", "DIYARBAKIR": "Diyarbakır", "mugla": "Muğla", "Tekirdag": "Tekirdağ",
	} {
		got, ok := CanonicalCity(in)
		if !ok || got != want {
			t.Errorf("CanonicalCity(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := CanonicalCity("Atlantis"); ok {
		t.Error("unknown city must not canonicalize")
	}
}

func rule(conds string) string {
	return `{"status":"ok","match":"all","conditions":[` + conds + `],"explanation":"x","question":""}`
}

func TestParseRejectsBadRules(t *testing.T) {
	many := strings.Repeat(`{"field":"order_count","op":"gt","value":1},`, 9)
	tests := []struct {
		name string
		raw  string
		want string // substring expected in the problems
	}{
		{"not json", `hello`, "JSON"},
		{"empty", ``, "JSON"},
		{"array", `[]`, "JSON"},
		{"trailing garbage", exampleRule + ` {"x":1}`, "fazladan"},
		{"unknown top-level field", `{"status":"ok","match":"all","conditions":[],"explanation":"","question":"","sql":"1=1"}`, "sql"},
		{"unknown condition field name", rule(`{"field":"password","op":"eq","value":"x"}`), "bilinmiyor"},
		{"sql in field name", rule(`{"field":"city; DROP TABLE users","op":"eq","value":"x"}`), "bilinmiyor"},
		{"sql in field name 2", rule(`{"field":"1=1 --","op":"eq","value":"x"}`), "bilinmiyor"},
		{"unknown op", rule(`{"field":"city","op":"like","value":"%"}`), "geçersiz"},
		{"sql in op", rule(`{"field":"order_count","op":"> 0 OR 1=1 --","value":1}`), "geçersiz"},
		{"op not allowed for field", rule(`{"field":"total_spent","op":"eq","value":10}`), "geçersiz"},
		{"neq on int", rule(`{"field":"order_count","op":"neq","value":1}`), "geçersiz"},
		{"unknown city", rule(`{"field":"city","op":"eq","value":"Atlantis"}`), "bilinmeyen şehir"},
		{"sql in city value", rule(`{"field":"city","op":"eq","value":"x' OR '1'='1"}`), "bilinmeyen şehir"},
		{"city list contains unknown", rule(`{"field":"city","op":"in","value":["Ankara","'; DROP TABLE users;--"]}`), "bilinmeyen şehir"},
		{"city in with scalar", rule(`{"field":"city","op":"in","value":"Ankara"}`), "liste"},
		{"city eq with list", rule(`{"field":"city","op":"eq","value":["Ankara"]}`), "metin"},
		{"empty city list", rule(`{"field":"city","op":"in","value":[]}`), "liste"},
		{"huge city list", rule(`{"field":"city","op":"in","value":[` + strings.Repeat(`"Ankara",`, 20) + `"Ankara"]}`), "liste"},
		{"int as string", rule(`{"field":"order_count","op":"gt","value":"5"}`), "tam sayı"},
		{"int as float", rule(`{"field":"days_since_last_cart_add","op":"lte","value":30.5}`), "tam sayı"},
		{"int as exponent", rule(`{"field":"order_count","op":"gt","value":1e2}`), "tam sayı"},
		{"int negative", rule(`{"field":"days_since_last_cart_add","op":"lte","value":-1}`), "arasında"},
		{"int too big", rule(`{"field":"days_since_last_cart_add","op":"lte","value":999999}`), "arasında"},
		{"int overflow", rule(`{"field":"order_count","op":"gt","value":99999999999999999999}`), "tam sayı"},
		{"number as string", rule(`{"field":"total_spent","op":"gt","value":"100"}`), "sayı"},
		{"number too big", rule(`{"field":"total_spent","op":"gt","value":1e12}`), "arasında"},
		{"bool as string", rule(`{"field":"purchased_since_last_cart_add","op":"eq","value":"false"}`), "true veya false"},
		{"bool as number", rule(`{"field":"purchased_since_last_cart_add","op":"eq","value":0}`), "true veya false"},
		{"null value", rule(`{"field":"order_count","op":"gt","value":null}`), "boş"},
		{"missing value", rule(`{"field":"order_count","op":"gt"}`), "boş"},
		{"no conditions", rule(``), "en az bir koşul"},
		{"too many conditions", rule(many + `{"field":"order_count","op":"gt","value":1}`), "en fazla"},
		{"bad match", strings.Replace(rule(`{"field":"order_count","op":"gt","value":1}`), `"all"`, `"none"`, 1), "match"},
		{"bad status", `{"status":"maybe","match":"all","conditions":[],"explanation":"","question":""}`, "status"},
		{"ok with question", strings.Replace(rule(`{"field":"order_count","op":"gt","value":1}`), `"question":""`, `"question":"?"`, 1), "question"},
		{"clarification without question", `{"status":"needs_clarification","match":"all","conditions":[],"explanation":"","question":""}`, "question"},
		{"clarification with conditions", `{"status":"needs_clarification","match":"all","conditions":[{"field":"order_count","op":"gt","value":1}],"explanation":"","question":"?"}`, "conditions"},
		{"unsupported without explanation", `{"status":"unsupported","match":"all","conditions":[],"explanation":"","question":""}`, "explanation"},
		{"explanation too long", `{"status":"unsupported","match":"all","conditions":[],"explanation":"` + strings.Repeat("a", 501) + `","question":""}`, "explanation"},
		{"oversized payload", `{"status":"ok","pad":"` + strings.Repeat("a", MaxRawBytes) + `"}`, "büyük"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.raw)
			if err == nil {
				t.Fatal("expected a validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

func TestParseReportsAllProblemsAtOnce(t *testing.T) {
	_, err := Parse(rule(`{"field":"nope","op":"eq","value":1},{"field":"city","op":"eq","value":"Atlantis"}`))
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("want *ParseError, got %T", err)
	}
	if len(pe.Problems) != 2 {
		t.Fatalf("problems: %v", pe.Problems)
	}
}

func TestNonOKRulesParseButDoNotCompile(t *testing.T) {
	r, err := Parse(`{"status":"needs_clarification","match":"all","conditions":[],"explanation":"","question":"Hangi şehir?"}`)
	if err != nil || r.Question != "Hangi şehir?" {
		t.Fatalf("%v %+v", err, r)
	}
	if _, err := Compile(r, now); err == nil {
		t.Fatal("a rule that is not ok must not compile")
	}
	if _, err := Compile(Rule{Status: StatusOK, Match: MatchAll}, now); err == nil {
		t.Fatal("a rule without conditions must not compile")
	}
}

// No value from the rule may ever appear in the SQL text.
func TestCompiledSQLContainsNoRuleValues(t *testing.T) {
	r, err := Parse(rule(`{"field":"city","op":"in","value":["Ankara","İzmir"]},{"field":"order_count","op":"gt","value":42424},{"field":"total_spent","op":"lt","value":987654.5}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Compile(r, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Ankara", "İzmir", "42424", "987654"} {
		if strings.Contains(c.Where, leak) {
			t.Fatalf("value %q leaked into SQL text: %s", leak, c.Where)
		}
	}
}

func TestSchemaIsFlatAndMatchesWhitelist(t *testing.T) {
	b, err := json.Marshal(JSONSchema())
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "$ref") || strings.Contains(s, "minLength") || strings.Contains(s, "maxLength") ||
		strings.Contains(s, "minimum") || strings.Contains(s, "maximum") {
		t.Fatalf("schema uses constructs structured outputs do not support: %s", s)
	}
	for _, f := range Fields() {
		if !strings.Contains(s, `"`+f.Name+`"`) {
			t.Errorf("field %s missing from schema", f.Name)
		}
	}
	if !strings.Contains(s, `"additionalProperties":false`) {
		t.Error("objects must set additionalProperties:false")
	}
}
