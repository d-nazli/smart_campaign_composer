package segment_test

import (
	"context"
	"strings"
	"testing"

	"smartcampaign/internal/segment"
)

// A gender condition selects exactly the people who stated that gender:
// "other" and "not stated" (NULL) never match either value.
func TestGenderSelectsOnlyPeopleWhoStatedIt(t *testing.T) {
	db, now := seeded(t)
	var female, male, unknown int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE gender='female'), count(*) FILTER (WHERE gender='male'),
		count(*) FILTER (WHERE gender IS NULL OR gender='other') FROM users`).Scan(&female, &male, &unknown); err != nil {
		t.Fatal(err)
	}
	if female == 0 || male == 0 || unknown == 0 {
		t.Fatalf("the seed must contain every case: female=%d male=%d other/unknown=%d", female, male, unknown)
	}
	for g, want := range map[string]int{"female": female, "male": male} {
		got := count(t, db, `{"status":"ok","match":"all","conditions":[{"field":"gender","op":"eq","value":"`+g+`"}],"explanation":"x","question":""}`, now)
		if got.Matched != want {
			t.Errorf("%s: matched %d, want %d", g, got.Matched, want)
		}
	}
}

func TestGenderValuesAreAWhitelist(t *testing.T) {
	for _, v := range []string{`"other"`, `"kadın"`, `"FEMALE"`, `true`, `""`} {
		_, err := segment.Parse(`{"status":"ok","match":"all","conditions":[{"field":"gender","op":"eq","value":` + v + `}],"explanation":"x","question":""}`)
		if err == nil || !strings.Contains(err.Error(), "female") {
			t.Errorf("gender %s must be rejected naming the allowed values, got %v", v, err)
		}
	}
	_, err := segment.Parse(`{"status":"ok","match":"all","conditions":[{"field":"gender","op":"neq","value":"male"}],"explanation":"x","question":""}`)
	if err == nil {
		t.Error("only eq is allowed for gender")
	}
}
