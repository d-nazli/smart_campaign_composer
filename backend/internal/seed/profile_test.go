package seed

import (
	"testing"
	"time"
)

func TestLanguageSplitIsExact(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	got := map[string]int{}
	for _, u := range ds.Users {
		got[u.Language]++
	}
	if got["tr"] != 3000 || got["en"] != 1250 || got["de"] != 750 || len(got) != 3 {
		t.Fatalf("languages = %v, want tr=3000 en=1250 de=750", got)
	}
}

func TestGenderSplit(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	got := map[string]int{}
	for _, u := range ds.Users {
		if u.Gender == nil {
			got["unstated"]++
			continue
		}
		got[*u.Gender]++
	}
	if got["female"] != 2400 || got["male"] != 2400 || got["other"] != 100 || got["unstated"] != 100 {
		t.Fatalf("genders = %v", got)
	}
}

func TestBirthDatesAreAdultsAndMostlyPresent(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	today := time.Date(testNow.Year(), testNow.Month(), testNow.Day(), 0, 0, 0, 0, time.UTC)
	oldest, youngest := today.AddDate(-maxAge, 0, 0), today.AddDate(-minAge, 0, 0)

	missing := 0
	var minD, maxD time.Time
	for _, u := range ds.Users {
		if u.BirthDate == nil {
			missing++
			continue
		}
		d := *u.BirthDate
		if d.Before(oldest) || d.After(youngest) {
			t.Fatalf("birth date %s outside [%s, %s]", d.Format(time.DateOnly), oldest.Format(time.DateOnly), youngest.Format(time.DateOnly))
		}
		if d != d.Truncate(24*time.Hour) {
			t.Fatalf("birth date %s is not a plain date", d)
		}
		if minD.IsZero() || d.Before(minD) {
			minD = d
		}
		if d.After(maxD) {
			maxD = d
		}
	}
	if missing != 150 {
		t.Fatalf("users without a birth date = %d, want 150 (3%%)", missing)
	}
	// "Mixed": the dates really spread over the range instead of clustering.
	if span := maxD.Sub(minD); span < 50*365*24*time.Hour {
		t.Fatalf("birth dates only span %s", span)
	}
}

// Language, gender and age are independent of the city and of the brief
// cohort: every group has a real mix, none is all-Turkish or all-female.
func TestProfileFieldsAreMixedInsideTheBriefCohort(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	langs, genders := map[string]int{}, map[string]int{}
	cohort := 0
	for _, u := range ds.Users {
		if !u.InBriefCohort(testNow) {
			continue
		}
		cohort++
		langs[u.Language]++
		if u.Gender != nil {
			genders[*u.Gender]++
		}
	}
	if cohort != 300 {
		t.Fatalf("cohort = %d, want 300 (profile fields must not move it)", cohort)
	}
	for _, l := range []string{"tr", "en", "de"} {
		if langs[l] < 20 {
			t.Fatalf("only %d %s speakers in the brief cohort: %v", langs[l], l, langs)
		}
	}
	if genders["female"] < 50 || genders["male"] < 50 {
		t.Fatalf("genders in the cohort not mixed: %v", genders)
	}
}
