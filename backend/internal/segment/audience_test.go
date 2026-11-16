package segment_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"smartcampaign/internal/seed"
	"smartcampaign/internal/segment"
	"smartcampaign/internal/testdb"

	"io"
	"log/slog"
)

var log = slog.New(slog.NewTextHandler(io.Discard, nil))

func seeded(t *testing.T) (segment.Querier, time.Time) {
	t.Helper()
	pool, _ := testdb.New(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := seed.Run(context.Background(), pool, 42, now, seed.DefaultUsers, log); err != nil {
		t.Fatal(err)
	}
	return pool, now
}

func count(t *testing.T, db segment.Querier, raw string, now time.Time) segment.Audience {
	t.Helper()
	r, err := segment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	c, err := segment.Compile(r, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := segment.CountAudience(context.Background(), db, c)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

const brief = `{"status":"ok","match":"all","conditions":[
 {"field":"city","op":"eq","value":"İstanbul"},
 {"field":"days_since_last_cart_add","op":"lte","value":30},
 {"field":"purchased_since_last_cart_add","op":"eq","value":false}],"explanation":"x","question":""}`

// The example brief must yield exactly the cohort the seed was built around.
func TestExampleBriefAudienceMatchesSeedGoldenNumbers(t *testing.T) {
	db, now := seeded(t)
	got := count(t, db, brief, now)
	want := segment.Audience{Matched: 300, SMSReachable: 203, EmailReachable: 227, BothReachable: 165}
	if got != want {
		t.Fatalf("audience = %+v, want %+v", got, want)
	}
}

// "Has not ordered in the last N days" must include people who NEVER ordered.
// days_since_last_order gt N cannot: a NULL timestamp never matches it.
func TestNoOrderInWindowIncludesPeopleWhoNeverOrdered(t *testing.T) {
	db, now := seeded(t)
	ctx := context.Background()
	rule := func(field, op string, n int) string {
		return `{"status":"ok","match":"all","conditions":[{"field":"` + field + `","op":"` + op + `","value":` + strconv.Itoa(n) + `}],"explanation":"x","question":""}`
	}
	cut := now.Add(-30 * 24 * time.Hour)

	var want, never, recent int
	if err := db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE last_order_at IS NULL OR last_order_at < $1),
	                                   count(*) FILTER (WHERE last_order_at IS NULL),
	                                   count(*) FILTER (WHERE last_order_at >= $1) FROM users`, cut).Scan(&want, &never, &recent); err != nil {
		t.Fatal(err)
	}
	if never == 0 || recent == 0 {
		t.Fatalf("seed must contain people who never ordered (%d) and recent buyers (%d)", never, recent)
	}

	got := count(t, db, rule("days_since_last_order_or_never", "gt", 30), now)
	if got.Matched != want {
		t.Fatalf("no order in 30 days = %d, want %d (never ordered %d + ordered long ago)", got.Matched, want, never)
	}
	// The lookalike field really does lose them, which is why the new one exists.
	lookalike := count(t, db, rule("days_since_last_order", "gt", 30), now)
	if lookalike.Matched != want-never {
		t.Fatalf("days_since_last_order gt 30 = %d, want %d", lookalike.Matched, want-never)
	}
	// Together with "ordered within 30 days" it partitions all users.
	bought := count(t, db, rule("days_since_last_order", "lte", 30), now)
	if got.Matched+bought.Matched != 5000 || bought.Matched != recent {
		t.Fatalf("no-order(%d) + ordered(%d) != 5000 (recent=%d)", got.Matched, bought.Matched, recent)
	}
	if ge := count(t, db, rule("days_since_last_order_or_never", "gte", 30), now); ge.Matched < got.Matched {
		t.Fatalf("gte %d < gt %d", ge.Matched, got.Matched)
	}
}

// Consent and contact data are applied by the system, whatever the rule says.
func TestReachableCountsNeverIncludeNonConsentingUsers(t *testing.T) {
	db, now := seeded(t)
	ctx := context.Background()

	var all segment.Audience
	r, _ := segment.Parse(`{"status":"ok","match":"all","conditions":[{"field":"order_count","op":"gte","value":0}],"explanation":"x","question":""}`)
	c, _ := segment.Compile(r, now)
	all, err := segment.CountAudience(ctx, db, c)
	if err != nil {
		t.Fatal(err)
	}
	if all.Matched != 5000 {
		t.Fatalf("matched = %d, want everyone (5000)", all.Matched)
	}

	var smsConsent, smsConsentWithPhone, emailConsent, emailConsentWithAddr int
	row := db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE sms_consent),
	                                count(*) FILTER (WHERE sms_consent AND phone IS NOT NULL),
	                                count(*) FILTER (WHERE email_consent),
	                                count(*) FILTER (WHERE email_consent AND email IS NOT NULL) FROM users`)
	if err := row.Scan(&smsConsent, &smsConsentWithPhone, &emailConsent, &emailConsentWithAddr); err != nil {
		t.Fatal(err)
	}
	if all.SMSReachable != smsConsentWithPhone || all.EmailReachable != emailConsentWithAddr {
		t.Fatalf("reachable %+v, want sms=%d email=%d", all, smsConsentWithPhone, emailConsentWithAddr)
	}
	if all.SMSReachable >= smsConsent || all.EmailReachable >= emailConsent {
		t.Fatal("seed must contain consenting users without contact data, who are excluded")
	}
	if all.SMSReachable >= all.Matched || all.EmailReachable >= all.Matched {
		t.Fatal("users without consent must be excluded")
	}
}

func TestEveryOperatorExecutesAgainstRealSchema(t *testing.T) {
	db, now := seeded(t)
	for name, raw := range map[string]string{
		"city eq":         `{"status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"Ankara"}],"explanation":"x","question":""}`,
		"city neq":        `{"status":"ok","match":"all","conditions":[{"field":"city","op":"neq","value":"Ankara"}],"explanation":"x","question":""}`,
		"city in":         `{"status":"ok","match":"all","conditions":[{"field":"city","op":"in","value":["Ankara","izmir"]}],"explanation":"x","question":""}`,
		"city not_in":     `{"status":"ok","match":"all","conditions":[{"field":"city","op":"not_in","value":["Ankara"]}],"explanation":"x","question":""}`,
		"cart lt":         `{"status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"lt","value":10}],"explanation":"x","question":""}`,
		"cart gt":         `{"status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"gt","value":40}],"explanation":"x","question":""}`,
		"cart eq":         `{"status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"eq","value":5}],"explanation":"x","question":""}`,
		"order gte":       `{"status":"ok","match":"all","conditions":[{"field":"days_since_last_order","op":"gte","value":100}],"explanation":"x","question":""}`,
		"order_count eq":  `{"status":"ok","match":"all","conditions":[{"field":"order_count","op":"eq","value":0}],"explanation":"x","question":""}`,
		"total_spent lte": `{"status":"ok","match":"all","conditions":[{"field":"total_spent","op":"lte","value":1000.5}],"explanation":"x","question":""}`,
		"purchased true":  `{"status":"ok","match":"all","conditions":[{"field":"purchased_since_last_cart_add","op":"eq","value":true}],"explanation":"x","question":""}`,
		"any":             `{"status":"ok","match":"any","conditions":[{"field":"city","op":"eq","value":"Bursa"},{"field":"order_count","op":"gt","value":6}],"explanation":"x","question":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			if a := count(t, db, raw, now); a.Matched < 0 {
				t.Fatal("impossible")
			}
		})
	}
}

func TestComplementaryConditionsPartitionTheirBase(t *testing.T) {
	db, now := seeded(t)
	yes := count(t, db, `{"status":"ok","match":"all","conditions":[{"field":"purchased_since_last_cart_add","op":"eq","value":true}],"explanation":"x","question":""}`, now)
	no := count(t, db, `{"status":"ok","match":"all","conditions":[{"field":"purchased_since_last_cart_add","op":"eq","value":false}],"explanation":"x","question":""}`, now)
	withCart := count(t, db, `{"status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"gte","value":0}],"explanation":"x","question":""}`, now)
	if yes.Matched+no.Matched != withCart.Matched {
		t.Fatalf("true(%d) + false(%d) != customers with a cart add (%d)", yes.Matched, no.Matched, withCart.Matched)
	}
	in := count(t, db, `{"status":"ok","match":"all","conditions":[{"field":"city","op":"in","value":["Ankara","İzmir"]}],"explanation":"x","question":""}`, now)
	notIn := count(t, db, `{"status":"ok","match":"all","conditions":[{"field":"city","op":"not_in","value":["Ankara","İzmir"]}],"explanation":"x","question":""}`, now)
	if in.Matched+notIn.Matched != 5000 {
		t.Fatalf("in(%d) + not_in(%d) != 5000", in.Matched, notIn.Matched)
	}
}

// Even a value that slipped past validation could only ever be data: prove the
// compiled query treats hostile text as a plain string.
func TestHostileValueIsOnlyDataNeverSQL(t *testing.T) {
	db, now := seeded(t)
	ctx := context.Background()
	hostile := segment.Rule{Status: segment.StatusOK, Match: segment.MatchAll, Conditions: []segment.Condition{
		{Field: "city", Op: segment.OpEq, Value: "x'; DROP TABLE users; --"},
	}}
	c, err := segment.Compile(hostile, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := segment.CountAudience(ctx, db, c)
	if err != nil {
		t.Fatal(err)
	}
	if a.Matched != 0 {
		t.Fatalf("matched = %d, want 0", a.Matched)
	}
	var users int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&users); err != nil || users != 5000 {
		t.Fatalf("users table damaged: %d %v", users, err)
	}
}

func TestSeedCitiesAreAllKnownToTheSegmentPackage(t *testing.T) {
	ds, err := seed.Generate(42, time.Now(), seed.DefaultUsers)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, u := range ds.Users {
		seen[u.City] = true
	}
	for city := range seen {
		if c, ok := segment.CanonicalCity(city); !ok || c != city {
			t.Errorf("seed city %q is not a canonical known city", city)
		}
	}
	if len(seen) != len(segment.KnownCities) {
		t.Errorf("seed has %d cities, segment knows %d", len(seen), len(segment.KnownCities))
	}
}
