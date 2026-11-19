package seed

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

// wantLocales is the exact split of 5000 users.
var wantLocales = map[string]int{
	"tr-TR": 2000, "en-GB": 400, "en-US": 400, "de-DE": 400, "fr-FR": 300, "es-ES": 250, "pt-BR": 250,
	"en-AU": 200, "es-MX": 200, "it-IT": 200, "nl-NL": 200, "ar-SA": 200,
}

func TestLocaleSplitIsExact(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	got := map[string]int{}
	for _, u := range ds.Users {
		got[u.Locale]++
	}
	if len(got) != len(wantLocales) {
		t.Fatalf("locales = %v", got)
	}
	for code, n := range wantLocales {
		if got[code] != n {
			t.Fatalf("locales = %v, want %v", got, wantLocales)
		}
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

// ageOn is the age in whole years on day, counted like PostgreSQL's age().
func ageOn(birth, day time.Time) int {
	a := day.Year() - birth.Year()
	if day.Month() < birth.Month() || (day.Month() == birth.Month() && day.Day() < birth.Day()) {
		a--
	}
	return a
}

func TestBirthDatesAreAdultsAndMostlyPresent(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)

	missing := 0
	var minD, maxD time.Time
	for _, u := range ds.Users {
		if u.BirthDate == nil {
			missing++
			continue
		}
		d := *u.BirthDate
		if a := ageOn(d, testNow); a < minAge || a > maxAge {
			t.Fatalf("birth date %s is age %d, outside %d..%d", d.Format(time.DateOnly), a, minAge, maxAge)
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

// The ages follow the bands exactly (4850 people with a birth date), and
// every age from 18 to 75 occurs.
func TestAgeBandsAreExact(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	want := map[string]int{"18-24": 776, "25-34": 1358, "35-44": 1164, "45-54": 776, "55-64": 485, "65-75": 291}
	got := map[string]int{}
	seen := map[int]bool{}
	for _, u := range ds.Users {
		if u.BirthDate == nil {
			continue
		}
		a := ageOn(*u.BirthDate, testNow)
		seen[a] = true
		for _, b := range ageBands {
			if a >= b.lo && a <= b.hi {
				got[fmt.Sprintf("%d-%d", b.lo, b.hi)]++
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("age bands = %v, want %v", got, want)
	}
	for a := minAge; a <= maxAge; a++ {
		if !seen[a] {
			t.Errorf("nobody is %d", a)
		}
	}
}

// Locale, gender and age are independent of the city and of the brief
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
		langs[u.Locale]++
		if u.Gender != nil {
			genders[*u.Gender]++
		}
	}
	if cohort != 300 {
		t.Fatalf("cohort = %d, want 300 (profile fields must not move it)", cohort)
	}
	for code := range wantLocales {
		if langs[code] < 3 {
			t.Fatalf("only %d people with %s in the brief cohort: %v", langs[code], code, langs)
		}
	}
	if langs["tr-TR"] < 80 {
		t.Fatalf("the source locale is the largest group: %v", langs)
	}
	if genders["female"] < 50 || genders["male"] < 50 {
		t.Fatalf("genders in the cohort not mixed: %v", genders)
	}
}
