package segment_test

import (
	"strings"
	"testing"

	"smartcampaign/internal/segment"
)

func TestTheCampaignNameIsChecked(t *testing.T) {
	for name, want := range map[string]string{
		"":                       "boş",
		"   ":                    "boş",
		strings.Repeat("a", 41):  "en fazla 40",
		"iki\nsatır":             "tek satır",
		"Merhaba {{first_name}}": "süslü parantez",
	} {
		if p := strings.Join(segment.NameProblems(name), " "); !strings.Contains(p, want) {
			t.Errorf("%q: %q should mention %q", name, p, want)
		}
	}
	for _, ok := range []string{"İstanbul sepet hatırlatma · %30", strings.Repeat("ş", 40)} {
		if p := segment.NameProblems(ok); len(p) != 0 {
			t.Errorf("%q: %v", ok, p)
		}
	}
}

// A name is optional when parsing (stored rules may have none) but must be
// valid when present; whitespace is tidied.
func TestParseKeepsAValidNameAndRejectsABadOne(t *testing.T) {
	const tail = `"status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"Ankara"}],"explanation":"x","question":""}`
	r, err := segment.Parse(`{"name":"  Ankara   kampanyası ",` + tail)
	if err != nil || r.Name != "Ankara kampanyası" {
		t.Fatalf("%v %q", err, r.Name)
	}
	if r, err := segment.Parse(`{` + tail); err != nil || r.Name != "" {
		t.Fatalf("no name: %v %q", err, r.Name)
	}
	if _, err := segment.Parse(`{"name":"` + strings.Repeat("a", 41) + `",` + tail); err == nil {
		t.Fatal("a 41-character name must be rejected")
	}
	if req, _ := segment.JSONSchema()["required"].([]any); len(req) == 0 || req[0] != "name" {
		t.Fatalf("the model must give a name: %v", req)
	}
}
