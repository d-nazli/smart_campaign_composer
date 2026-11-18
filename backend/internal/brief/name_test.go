package brief_test

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/segment"
)

const exampleBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

// The mock names a campaign the way a model would: the audience or the
// special day, the topic and the offer, always within 40 characters.
func TestTheMockNamesTheCampaign(t *testing.T) {
	for text, want := range map[string]string{
		exampleBrief: "İstanbul sepet hatırlatma · %30",
		"Kadınlar Günü'ne özel tüm müşterilere %20 indirim gönder":                                 "Kadınlar Günü · %20",
		"Herkese ücretsiz kargo kampanyası duyur":                                                  "Kampanya · ücretsiz kargo",
		"Ankara'daki müşterilere 1000 TL üzeri alışverişe 100 TL indirim, son kullanım 31.12.2026": "Ankara kampanyası · 100 TL",
	} {
		r, err := brief.ResolveSegment(context.Background(), mock.New(0), text, llm.Hooks{})
		if err != nil {
			t.Fatal(err)
		}
		if r.Name != want {
			t.Errorf("%q: name %q, want %q", text, r.Name, want)
		}
	}
	long := segment.Rule{Conditions: []segment.Condition{{Field: "city", Op: segment.OpEq, Value: "Afyonkarahisar Merkez Bölgesi İlçeleri"}}}
	if n := brief.SuggestName("sepetini unutanlara hatırlatma", long); utf8.RuneCountInString(n) > segment.MaxNameRunes || n == "" {
		t.Fatalf("%q", n)
	}
}

// A model that gives no name (or a long one) is asked again, with the reason.
func TestAModelWithoutANameIsAskedAgain(t *testing.T) {
	good := `{"name":"İstanbul sepet hatırlatma · %30","status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"İstanbul"},{"field":"days_since_last_cart_add","op":"lte","value":30},{"field":"purchased_since_last_cart_add","op":"eq","value":false}],"explanation":"x","question":"","offer":{"kind":"percent","value":30,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`
	noName := strings.Replace(good, `"name":"İstanbul sepet hatırlatma · %30",`, ``, 1)
	tooLong := strings.Replace(good, `İstanbul sepet hatırlatma · %30`, strings.Repeat("a", 41), 1)
	m := mock.NewScripted([]mock.Step{{Kind: mock.Raw, Raw: noName}, {Kind: mock.Raw, Raw: tooLong}, {Kind: mock.Raw, Raw: good}}, nil)
	r, err := brief.ResolveSegment(context.Background(), m, exampleBrief, llm.Hooks{})
	if err != nil || r.Name != "İstanbul sepet hatırlatma · %30" {
		t.Fatalf("%v %q", err, r.Name)
	}
	calls := m.Calls()
	if len(calls) != 3 || !strings.Contains(calls[1].Previous[0].Problems[0], "name") || !strings.Contains(calls[2].Previous[0].Problems[0], "40") {
		t.Fatalf("%+v", calls)
	}
}
