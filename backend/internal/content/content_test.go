package content

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"smartcampaign/internal/offer"
	"smartcampaign/internal/seed"
)

func mustPolicy(t *testing.T, overrides map[string]int) Policy {
	t.Helper()
	p, err := NewPolicy(overrides)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func ok() Content {
	return Content{SMSBody: "Merhaba {{first_name}}, %30 indirim seni bekliyor!", EmailSubject: "{{first_name}}, sepetin seni bekliyor", EmailBody: "Merhaba {{first_name}},\n\nSepetindeki ürünlerde %30 indirim var."}
}

func codes(r Report) map[string]bool {
	m := map[string]bool{}
	for _, i := range r.Issues {
		m[i.Field+":"+i.Code] = true
	}
	return m
}

func TestValidContentPasses(t *testing.T) {
	_, rep := mustPolicy(t, nil).Validate(ok())
	if !rep.OK {
		t.Fatalf("issues: %+v", rep.Issues)
	}
}

func TestEffectiveLengthUsesFixedBudgets(t *testing.T) {
	p := mustPolicy(t, nil)
	// "ab " (3) + 20 (first_name) + " " (1) + 15 (city) = 39
	if got := p.EffectiveLength("ab {{first_name}} {{city}}"); got != 39 {
		t.Fatalf("got %d, want 39", got)
	}
	if got := p.EffectiveLength("ab {{ first_name }}"); got != 23 {
		t.Fatalf("spaced placeholder: got %d, want 23", got)
	}
}

func TestSMSLimitIsExactlyOneSixty(t *testing.T) {
	p := mustPolicy(t, nil)
	c := ok()
	c.SMSBody = strings.Repeat("a", 160)
	if _, rep := p.Validate(c); !rep.OK {
		t.Fatalf("160 characters must pass: %+v", rep.Issues)
	}
	c.SMSBody = strings.Repeat("a", 161)
	if _, rep := p.Validate(c); !codes(rep)["sms_body:too_long"] {
		t.Fatalf("161 characters must fail: %+v", rep.Issues)
	}
	// With a variable: 140 + 20 = 160 passes, 141 + 20 fails.
	c.SMSBody = strings.Repeat("a", 140) + "{{first_name}}"
	if _, rep := p.Validate(c); !rep.OK {
		t.Fatalf("140+20 must pass: %+v", rep.Issues)
	}
	c.SMSBody = strings.Repeat("a", 141) + "{{first_name}}"
	if _, rep := p.Validate(c); !codes(rep)["sms_body:too_long"] {
		t.Fatalf("141+20 must fail: %+v", rep.Issues)
	}
}

func TestSubjectLimitIsExactlyEighty(t *testing.T) {
	p := mustPolicy(t, nil)
	c := ok()
	c.EmailSubject = strings.Repeat("b", 80)
	if _, rep := p.Validate(c); !rep.OK {
		t.Fatalf("80 characters must pass: %+v", rep.Issues)
	}
	c.EmailSubject = strings.Repeat("b", 81)
	if _, rep := p.Validate(c); !codes(rep)["email_subject:too_long"] {
		t.Fatalf("81 characters must fail: %+v", rep.Issues)
	}
}

func TestLengthCountsCharactersNotBytes(t *testing.T) {
	p := mustPolicy(t, nil)
	c := ok()
	c.SMSBody = strings.Repeat("ş", 160) // 320 bytes
	if _, rep := p.Validate(c); !rep.OK {
		t.Fatalf("160 Turkish characters must pass: %+v", rep.Issues)
	}
	c.SMSBody = strings.Repeat("İ", 161)
	if _, rep := p.Validate(c); !codes(rep)["sms_body:too_long"] {
		t.Fatal("161 Turkish characters must fail")
	}
}

func TestSubjectMustBeSingleLine(t *testing.T) {
	p := mustPolicy(t, nil)
	for _, subj := range []string{"a\nb", "a\r\nb", "a\rb", "a\u2028b", "a\u2029b", "a\u0085b"} {
		c := ok()
		c.EmailSubject = subj
		if _, rep := p.Validate(c); !codes(rep)["email_subject:newline_in_subject"] {
			t.Errorf("subject %q must be rejected: %+v", subj, rep.Issues)
		}
	}
	// A trailing newline is trimmed, not an error.
	c := ok()
	c.EmailSubject = "Merhaba\n"
	if norm, rep := p.Validate(c); !rep.OK || norm.EmailSubject != "Merhaba" {
		t.Fatalf("trailing newline: %+v %q", rep.Issues, norm.EmailSubject)
	}
}

func TestRequiredFields(t *testing.T) {
	_, rep := mustPolicy(t, nil).Validate(Content{SMSBody: "  ", EmailSubject: "", EmailBody: "\n"})
	for _, k := range []string{"sms_body:required", "email_subject:required", "email_body:required"} {
		if !codes(rep)[k] {
			t.Errorf("missing %s: %+v", k, rep.Issues)
		}
	}
}

func TestVariableValidation(t *testing.T) {
	p := mustPolicy(t, nil)
	tests := []struct {
		name string
		sms  string
		want string // issue code, "" = valid
	}{
		{"known", "Hi {{first_name}} from {{city}}", ""},
		{"spaced", "Hi {{ first_name }}", ""},
		{"removed last_name", "Hi {{last_name}}", "sms_body:unknown_variable"},
		{"unknown", "Hi {{email}}", "sms_body:unknown_variable"},
		{"empty", "Hi {{}}", "sms_body:unknown_variable"},
		{"single braces", "Hi {first_name}", "sms_body:malformed_placeholder"},
		{"unclosed", "Hi {{first_name}", "sms_body:malformed_placeholder"},
		{"stray close", "Hi first_name}}", "sms_body:malformed_placeholder"},
		{"case matters", "Hi {{First_Name}}", "sms_body:unknown_variable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ok()
			c.SMSBody = tt.sms
			norm, rep := p.Validate(c)
			if tt.want == "" {
				if !rep.OK {
					t.Fatalf("unexpected issues: %+v", rep.Issues)
				}
				if strings.Contains(norm.SMSBody, "{{ ") {
					t.Fatalf("placeholder not normalized: %q", norm.SMSBody)
				}
				return
			}
			if !codes(rep)[tt.want] {
				t.Fatalf("want %s, got %+v", tt.want, rep.Issues)
			}
		})
	}
}

func TestControlCharactersRejected(t *testing.T) {
	c := ok()
	c.SMSBody = "hi\x00there"
	if _, rep := mustPolicy(t, nil).Validate(c); !codes(rep)["sms_body:control_character"] {
		t.Fatalf("NUL must be rejected: %+v", rep.Issues)
	}
	c = ok()
	c.EmailBody = "line1\r\nline2\tx" // CRLF and tab are fine in a body
	if _, rep := mustPolicy(t, nil).Validate(c); !rep.OK {
		t.Fatalf("CRLF/tab in body: %+v", rep.Issues)
	}
}

func TestBudgetOverridesFromEnv(t *testing.T) {
	env := func(k string) string {
		if k == "VARIABLE_BUDGET_FIRST_NAME" {
			return "30"
		}
		return ""
	}
	p, err := PolicyFromEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	if p.Budget("first_name") != 30 || p.Budget("city") != 15 {
		t.Fatalf("budgets: first_name=%d city=%d", p.Budget("first_name"), p.Budget("city"))
	}
	for name, overrides := range map[string]map[string]int{
		"unknown variable":       {"nope": 5},
		"too small for fallback": {"first_name": 3},
		"above sane maximum":     {"city": 500},
	} {
		if _, err := NewPolicy(overrides); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := PolicyFromEnv(func(k string) string {
		if k == "VARIABLE_BUDGET_CITY" {
			return "abc"
		}
		return ""
	}); err == nil {
		t.Error("non-numeric env must fail")
	}
}

func TestEveryVariableFallbackFitsItsBudget(t *testing.T) {
	p := mustPolicy(t, nil)
	for _, v := range Variables {
		if n := utf8.RuneCountInString(v.Fallback); n > p.Budget(v.Name) {
			t.Errorf("%s fallback has %d characters, budget %d", v.Name, n, p.Budget(v.Name))
		}
	}
}

func TestRenderUsesValueOrFallback(t *testing.T) {
	p := mustPolicy(t, nil)
	tpl := "Merhaba {{first_name}} ({{city}})"
	if got := p.Render(tpl, Recipient{FirstName: "Ayşe", City: "İzmir"}); got != "Merhaba Ayşe (İzmir)" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("x", 21) // one over the budget
	if got := p.Render(tpl, Recipient{FirstName: long, City: "İzmir"}); got != "Merhaba değerli müşterimiz (İzmir)" {
		t.Fatalf("over-budget name: got %q", got)
	}
	if got := p.Render(tpl, Recipient{FirstName: strings.Repeat("x", 20), City: ""}); !strings.Contains(got, "bölgeniz") || !strings.Contains(got, strings.Repeat("x", 20)) {
		t.Fatalf("exact-budget name kept, empty city falls back: %q", got)
	}
	if got := p.Render("{{ first_name }}", Recipient{FirstName: "a\nb"}); got != "a b" {
		t.Fatalf("newlines in values must become spaces: %q", got)
	}
}

func TestFallbackContentIsValid(t *testing.T) {
	p := mustPolicy(t, nil)
	if _, rep := p.Validate(p.Fallback(offer.Offer{})); !rep.OK {
		t.Fatalf("default fallback invalid: %+v", rep.Issues)
	}
	// Absurdly large budgets would break the personalized fallback; the
	// variable-free one must still be returned and be valid.
	big := mustPolicy(t, map[string]int{"first_name": 100, "city": 100})
	f := big.Fallback(offer.Offer{})
	if _, rep := big.Validate(f); !rep.OK {
		t.Fatalf("fallback with large budgets invalid: %+v", rep.Issues)
	}
	if strings.Contains(f.SMSBody, "{{") {
		t.Fatalf("expected the variable-free fallback, got %q", f.SMSBody)
	}
}

// The guarantee that matters: a template that passes Validate renders within
// the limits for EVERY recipient in the seed, including the 30 users with
// 30..45 character first names.
func TestRenderedMessagesNeverExceedLimitsForAnySeedUser(t *testing.T) {
	p := mustPolicy(t, nil)
	ds, err := seed.Generate(42, time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), seed.DefaultUsers)
	if err != nil {
		t.Fatal(err)
	}

	// Worst legal templates: exactly at the limit with the most expensive variables.
	templates := []Content{
		{SMSBody: strings.Repeat("a", 160-20-15) + "{{first_name}}{{city}}", EmailSubject: strings.Repeat("b", 80-20-15) + "{{first_name}}{{city}}", EmailBody: "x"},
		{SMSBody: strings.Repeat("ş", 120) + "{{first_name}}{{first_name}}", EmailSubject: "{{first_name}} {{first_name}} {{first_name}}", EmailBody: "x"},
		p.Fallback(offer.Offer{}),
	}
	longNames := 0
	for _, tpl := range templates {
		norm, rep := p.Validate(tpl)
		if !rep.OK {
			t.Fatalf("template must be valid: %+v", rep.Issues)
		}
		for _, u := range ds.Users {
			if utf8.RuneCountInString(u.FirstName) > 20 {
				longNames++
			}
			r := p.RenderContent(norm, Recipient{FirstName: u.FirstName, City: u.City})
			if n := utf8.RuneCountInString(r.SMSBody); n > SMSMaxRunes {
				t.Fatalf("user %d: rendered SMS has %d characters: %q", u.ID, n, r.SMSBody)
			}
			if n := utf8.RuneCountInString(r.EmailSubject); n > SubjectMaxRunes {
				t.Fatalf("user %d: rendered subject has %d characters: %q", u.ID, n, r.EmailSubject)
			}
			if strings.ContainsAny(r.EmailSubject, "\n\r") {
				t.Fatalf("user %d: rendered subject has a line break", u.ID)
			}
		}
	}
	if longNames == 0 {
		t.Fatal("the seed must contain users whose first name exceeds the budget")
	}
}

// The last check before sending: exactly at the limit passes, one over fails
// with a reason the dashboard can group.
func TestRenderedProblemIsTheLastLineOfDefence(t *testing.T) {
	for _, tc := range []struct {
		name, channel string
		c             Content
		bad           bool
	}{
		{"sms at 160", "sms", Content{SMSBody: strings.Repeat("ş", 160)}, false},
		{"sms at 161", "sms", Content{SMSBody: strings.Repeat("ş", 161)}, true},
		{"subject at 80", "email", Content{EmailSubject: strings.Repeat("ğ", 80), EmailBody: "x"}, false},
		{"subject at 81", "email", Content{EmailSubject: strings.Repeat("ğ", 81), EmailBody: "x"}, true},
		{"subject with a line break", "email", Content{EmailSubject: "a\nb", EmailBody: "x"}, true},
		{"sms limit does not apply to email", "email", Content{SMSBody: strings.Repeat("a", 500), EmailSubject: "a", EmailBody: "x"}, false},
	} {
		got := RenderedProblem(tc.channel, tc.c)
		if (got != "") != tc.bad {
			t.Errorf("%s: problem %q, want bad=%v", tc.name, got, tc.bad)
		}
		if got != "" && !strings.HasPrefix(got, TooLongPrefix) {
			t.Errorf("%s: reason %q must start with %q", tc.name, got, TooLongPrefix)
		}
	}
}
