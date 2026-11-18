package content

import (
	"strings"
	"testing"
)

func TestApproachProblem(t *testing.T) {
	for _, c := range []struct {
		approach, other string
		deadline        bool
		want            string
	}{
		{"fayda", "", false, ""},
		{"korku", "", true, "listede yok"},
		{"", "", true, "listede yok"},
		{"merak", "merak", true, "aynı"},
		{"aciliyet", "", false, "seçilemez"},
		{"aciliyet", "fayda", true, ""},
	} {
		got := ApproachProblem(c.approach, c.other, c.deadline)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%+v: %q", c, got)
		}
	}
	if s := MessagesJSONSchema(); len(s["required"].([]any)) != 4 {
		t.Fatalf("schema %v", s)
	}
	if len(JSONSchema()["required"].([]any)) != 3 {
		t.Fatal("the translation schema changed")
	}
}

func TestUnsupportedClaims(t *testing.T) {
	c := Content{SMSBody: "Son gün! %10 indirim", EmailSubject: "Stoklar tükeniyor", EmailBody: "Acele edin."}
	if n := len(UnsupportedClaims(c, false, false)); n < 4 {
		t.Fatalf("found %d", n)
	}
	if n := len(UnsupportedClaims(c, true, true)); n != 0 {
		t.Fatalf("backed claims flagged: %d", n)
	}
	if n := len(UnsupportedClaims(Content{SMSBody: "Bugün sana güzel bir haberimiz var", EmailSubject: "Fırsatı kaçırma", EmailBody: "x"}, false, false)); n != 0 {
		t.Fatalf("plain words flagged: %d", n)
	}
}

func TestVariantFactsComparesThePromiseNotTheWording(t *testing.T) {
	l := Locale{Code: "tr-TR", FreeShippingTerms: []string{"ücretsiz kargo"}}
	a := Content{SMSBody: "{{first_name}}, %30 indirim!", EmailSubject: "%30 indirim", EmailBody: "Merhaba {{first_name}}"}
	b := Content{SMSBody: "Sepetin seni bekliyor: %30 indirim", EmailSubject: "{{city}} için", EmailBody: "%30 indirim var"}
	if issues := VariantFacts(l, a, b); len(issues) != 0 {
		t.Fatalf("%v", issues)
	}
	b.SMSBody = "%20 indirim"
	if issues := VariantFacts(l, a, b); len(issues) != 1 || issues[0].Code != CodeVariantMismatch {
		t.Fatalf("%v", issues)
	}
}
