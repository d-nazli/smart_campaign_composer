package seed

import (
	"reflect"
	"testing"
	"time"
	"unicode/utf8"
)

var testNow = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func mustGenerate(t *testing.T, seed uint64, now time.Time, n int) *Dataset {
	t.Helper()
	ds, err := Generate(seed, now, n)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestGenerateIsDeterministic(t *testing.T) {
	a := mustGenerate(t, 42, testNow, DefaultUsers)
	b := mustGenerate(t, 42, testNow, DefaultUsers)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed and time must produce an identical dataset")
	}
	c := mustGenerate(t, 43, testNow, DefaultUsers)
	if reflect.DeepEqual(a, c) {
		t.Fatal("different seeds must produce different datasets")
	}
}

func TestGenerateRejectsTinyDataset(t *testing.T) {
	if _, err := Generate(1, testNow, 10); err == nil {
		t.Fatal("expected error for a too-small dataset")
	}
}

func TestExactCounts(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	if len(ds.Users) != 5000 {
		t.Fatalf("users = %d", len(ds.Users))
	}

	var noPhone, noEmail, badPhone, badEmail, longNames, istanbul int
	for _, u := range ds.Users {
		if u.Phone == nil {
			noPhone++
		} else if !validPhoneShape(*u.Phone) {
			badPhone++
		}
		if u.Email == nil {
			noEmail++
		} else if !validEmailShape(*u.Email) {
			badEmail++
		}
		if utf8.RuneCountInString(u.FirstName) >= 30 {
			longNames++
			if n := utf8.RuneCountInString(u.FirstName); n > 45 {
				t.Fatalf("first name too long (%d runes): %q", n, u.FirstName)
			}
		}
		if u.City == BriefCity {
			istanbul++
		}
	}
	check := func(name string, got, want int) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	check("missing phones", noPhone, 150)
	check("missing emails", noEmail, 150)
	check("malformed phones", badPhone, 25)
	check("malformed emails", badEmail, 25)
	check("long first names", longNames, 30)
	check("istanbul users", istanbul, 1750)
}

func TestBriefCohortIsExactlyThreeHundred(t *testing.T) {
	// If any random user accidentally matched the brief, or a decoy group
	// leaked into the cohort, this count would not be exactly 300.
	for _, now := range []time.Time{testNow, testNow.Add(7 * time.Hour), time.Date(2027, 3, 1, 3, 30, 0, 0, time.UTC)} {
		ds := mustGenerate(t, 42, now, DefaultUsers)
		if got := ds.CohortCount(now); got != 300 {
			t.Fatalf("cohort at %v = %d, want 300", now, got)
		}
	}
}

func TestCohortReachability(t *testing.T) {
	// Pins the exact SMS/email reachable counts of the 300-user cohort for
	// SEED=42, so any change to the generator is noticed. Phase 2 tests rely on
	// these numbers (consent + contact filtering of the segment).
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	var sms, email, both int
	for _, u := range ds.Users {
		if !u.InBriefCohort(testNow) {
			continue
		}
		if u.SMSReachable() {
			sms++
		}
		if u.EmailReachable() {
			email++
		}
		if u.SMSReachable() && u.EmailReachable() {
			both++
		}
	}
	// These do not depend on `now`: only the dates move with it.
	if sms != 203 || email != 227 || both != 165 {
		t.Fatalf("cohort reachable sms=%d email=%d both=%d, want 203/227/165 "+
			"(if the generator changed on purpose, update these numbers)", sms, email, both)
	}
}

func TestConsentDistribution(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	var both, smsOnly, emailOnly, none int
	for _, u := range ds.Users {
		switch {
		case u.SMSConsent && u.EmailConsent:
			both++
		case u.SMSConsent:
			smsOnly++
		case u.EmailConsent:
			emailOnly++
		default:
			none++
		}
	}
	if both != 3000 || smsOnly != 600 || emailOnly != 900 || none != 500 {
		t.Fatalf("consent split both=%d smsOnly=%d emailOnly=%d none=%d", both, smsOnly, emailOnly, none)
	}
}

func TestConsentWithoutContactExists(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	var smsNoPhone, emailNoAddr int
	for _, u := range ds.Users {
		if u.SMSConsent && u.Phone == nil {
			smsNoPhone++
		}
		if u.EmailConsent && u.Email == nil {
			emailNoAddr++
		}
	}
	if smsNoPhone == 0 || emailNoAddr == 0 {
		t.Fatalf("seed must contain consent-without-contact users, got sms=%d email=%d", smsNoPhone, emailNoAddr)
	}
}

func TestCityShares(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	count := map[string]int{}
	for _, u := range ds.Users {
		count[u.City]++
	}
	for _, c := range cityShares {
		if want := 5000 * c.percent / 100; count[c.name] != want {
			t.Errorf("%s = %d, want %d", c.name, count[c.name], want)
		}
	}
}

func TestNoActivityShare(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	none := 0
	for _, u := range ds.Users {
		if u.LastCartAddAt == nil && u.LastOrderAt == nil {
			none++
		}
	}
	if share := float64(none) / 5000; share < 0.25 || share > 0.40 {
		t.Fatalf("share of users without activity = %.2f, want about 0.30", share)
	}
}

func TestNoCartEventsInTheBoundaryBand(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	for _, c := range ds.CartEvents {
		age := testNow.Sub(c.AddedAt)
		if age >= 26*24*time.Hour && age < 36*24*time.Hour {
			t.Fatalf("cart event %d is %v old: inside the reserved 26..35 day band", c.ID, age)
		}
		if c.AddedAt.After(testNow) {
			t.Fatalf("cart event %d is in the future", c.ID)
		}
	}
	for _, o := range ds.Orders {
		if o.CreatedAt.After(testNow) {
			t.Fatalf("order %d is in the future", o.ID)
		}
	}
}

func TestDerivedColumnsMatchRawEvents(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	type agg struct {
		lastCart, lastOrder time.Time
		count               int
		cents               int64
	}
	by := map[int64]*agg{}
	get := func(id int64) *agg {
		if by[id] == nil {
			by[id] = &agg{}
		}
		return by[id]
	}
	for _, c := range ds.CartEvents {
		if a := get(c.UserID); c.AddedAt.After(a.lastCart) {
			a.lastCart = c.AddedAt
		}
	}
	for _, o := range ds.Orders {
		a := get(o.UserID)
		if o.CreatedAt.After(a.lastOrder) {
			a.lastOrder = o.CreatedAt
		}
		a.count++
		a.cents += o.TotalCents
	}
	for _, u := range ds.Users {
		a := get(u.ID)
		if (u.LastCartAddAt == nil) != a.lastCart.IsZero() || (u.LastCartAddAt != nil && !u.LastCartAddAt.Equal(a.lastCart)) {
			t.Fatalf("user %d last_cart_add_at mismatch", u.ID)
		}
		if (u.LastOrderAt == nil) != a.lastOrder.IsZero() || (u.LastOrderAt != nil && !u.LastOrderAt.Equal(a.lastOrder)) {
			t.Fatalf("user %d last_order_at mismatch", u.ID)
		}
		if u.OrderCount != a.count || u.TotalSpentCents != a.cents {
			t.Fatalf("user %d order aggregates mismatch", u.ID)
		}
	}
}

func TestContactValuesAreFakeAndUnique(t *testing.T) {
	ds := mustGenerate(t, 42, testNow, DefaultUsers)
	seen := map[string]bool{}
	for _, u := range ds.Users {
		if u.Email != nil && validEmailShape(*u.Email) {
			if seen[*u.Email] {
				t.Fatalf("duplicate email %s", *u.Email)
			}
			seen[*u.Email] = true
			if want := "@example.com"; len(*u.Email) < len(want) || (*u.Email)[len(*u.Email)-len(want):] != want {
				t.Fatalf("email %q is not on the reserved example.com domain", *u.Email)
			}
		}
	}
}

// Shape checks mirroring what the mock provider accepts.
func validPhoneShape(s string) bool {
	if len(s) != 13 || s[:3] != "+90" {
		return false
	}
	for _, r := range s[3:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validEmailShape(s string) bool {
	at := -1
	for i, r := range s {
		if r == '@' {
			if at >= 0 {
				return false
			}
			at = i
		}
	}
	if at <= 0 || at == len(s)-1 {
		return false
	}
	domain := s[at+1:]
	for i, r := range domain {
		if r == '.' && i > 0 && i < len(domain)-1 {
			return true
		}
	}
	return false
}
