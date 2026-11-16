package segment

import (
	"encoding/json"
	"strings"
	"testing"

	"smartcampaign/internal/offer"
)

const everyoneCond = `{"field":"all_customers","op":"eq","value":true}`

func TestAllCustomersIsAnExplicitConditionNotAnEmptyRule(t *testing.T) {
	r, err := Parse(rule(everyoneCond))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Conditions) != 1 || r.Conditions[0].Field != "all_customers" || r.Conditions[0].Value != true {
		t.Fatalf("%+v", r.Conditions)
	}
	c, err := Compile(r, now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Where != "((TRUE))" || len(c.Args) != 0 {
		t.Fatalf("where %q args %v", c.Where, c.Args)
	}
}

func TestAllCustomersCannotBeMisused(t *testing.T) {
	for name, tc := range map[string]struct{ raw, want string }{
		"false":             {rule(`{"field":"all_customers","op":"eq","value":false}`), "true"},
		"neq":               {rule(`{"field":"all_customers","op":"neq","value":true}`), "geçersiz"},
		"string":            {rule(`{"field":"all_customers","op":"eq","value":"evet"}`), "true"},
		"with another cond": {rule(everyoneCond + `,{"field":"city","op":"eq","value":"Ankara"}`), "tek başına"},
		"twice":             {rule(everyoneCond + `,` + everyoneCond), "tek başına"},
	} {
		_, err := Parse(tc.raw)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v (want %q)", name, err, tc.want)
		}
	}
}

func TestOfferTermsSurviveParsing(t *testing.T) {
	raw := `{"status":"ok","match":"all","conditions":[` + everyoneCond + `],"explanation":"x","question":"",
		"offer":{"kind":"amount","value":100,"code":"","min_spend":1000,"expires_at":"2026-12-31"}}`
	r, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}
	if !r.Offer.Equal(want) || r.Offer.MinSpend != 1000 || r.Offer.ExpiresAt != "2026-12-31" {
		t.Fatalf("%+v", r.Offer)
	}
	// A rule without the new fields (older rows, older prompts) still parses.
	old := `{"status":"ok","match":"all","conditions":[` + everyoneCond + `],"explanation":"x","question":"","offer":{"kind":"percent","value":10,"code":""}}`
	if r, err := Parse(old); err != nil || r.Offer.MinSpend != 0 || r.Offer.ExpiresAt != "" {
		t.Fatalf("%+v %v", r.Offer, err)
	}
}

func TestBadOfferTermsAreRejectedWithTheFieldName(t *testing.T) {
	for _, tc := range []struct{ offer, want string }{
		{`{"kind":"amount","value":100,"code":"","min_spend":-5,"expires_at":""}`, "min_spend"},
		{`{"kind":"amount","value":100,"code":"","min_spend":0,"expires_at":"31.12.2026"}`, "expires_at"},
		{`{"kind":"none","value":0,"code":"","min_spend":1000,"expires_at":""}`, "min_spend"},
	} {
		raw := `{"status":"ok","match":"all","conditions":[` + everyoneCond + `],"explanation":"x","question":"","offer":` + tc.offer + `}`
		if _, err := Parse(raw); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.offer, err)
		}
	}
}

func TestSchemaDeclaresTheOfferTermsAndTheNewField(t *testing.T) {
	raw, _ := json.Marshal(JSONSchema())
	s := string(raw)
	for _, want := range []string{`"min_spend"`, `"expires_at"`, `"all_customers"`} {
		if !strings.Contains(s, want) {
			t.Errorf("schema lacks %s", want)
		}
	}
	var sch struct {
		Properties struct {
			Offer struct {
				Required []string `json:"required"`
			} `json:"offer"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &sch); err != nil {
		t.Fatal(err)
	}
	req := strings.Join(sch.Properties.Offer.Required, ",")
	if !strings.Contains(req, "min_spend") || !strings.Contains(req, "expires_at") {
		t.Fatalf("structured outputs need every property required: %s", req)
	}
}

func TestASuggestionIsPartOfTheStoredRuleButOnlyWhenSet(t *testing.T) {
	with, _ := json.Marshal(Rule{Status: StatusNeedsClarification, Conditions: []Condition{}, Question: "q", Suggestion: SuggestAllCustomers})
	if !strings.Contains(string(with), `"suggestion":"all_customers"`) {
		t.Fatalf("%s", with)
	}
	without, _ := json.Marshal(Rule{Status: StatusOK, Conditions: []Condition{}})
	if strings.Contains(string(without), "suggestion") {
		t.Fatalf("%s", without)
	}
}
