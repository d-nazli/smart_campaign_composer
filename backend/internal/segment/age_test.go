package segment_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"smartcampaign/internal/segment"
	"smartcampaign/internal/testdb"
)

func ageRule(conds ...string) string {
	return `{"status":"ok","match":"all","conditions":[` + strings.Join(conds, ",") + `],"explanation":"x","question":""}`
}

func ageCond(op string, n int) string {
	return fmt.Sprintf(`{"field":"age","op":"%s","value":%d}`, op, n)
}

func TestAgeParsesAndCompilesToBirthDateBounds(t *testing.T) {
	now := time.Date(2026, 11, 15, 22, 30, 0, 0, time.UTC)
	r, err := segment.Parse(ageRule(ageCond("gte", 18), ageCond("lte", 25)))
	if err != nil {
		t.Fatal(err)
	}
	if r.Conditions[0].Value != int64(18) || r.Conditions[1].Value != int64(25) {
		t.Fatalf("values: %+v", r.Conditions)
	}
	c, err := segment.Compile(r, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := "((birth_date <= $1::date) AND (birth_date > $2::date))"; c.Where != want {
		t.Fatalf("where:\n got %s\nwant %s", c.Where, want)
	}
	day := func(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	// 18 or older: born on or before 15.11.2008; at most 25: not yet 26, born after 15.11.2000.
	if want := []any{day(2008, 11, 15), day(2000, 11, 15)}; !reflect.DeepEqual(c.Args, want) {
		t.Fatalf("args: got %v want %v", c.Args, want)
	}

	for op, where := range map[string]string{
		"gt": "((birth_date <= $1::date))", "lt": "((birth_date > $1::date))",
		"eq": "(((birth_date <= $1::date AND birth_date > $2::date)))",
	} {
		r, err := segment.Parse(ageRule(ageCond(op, 30)))
		if err != nil {
			t.Fatal(err)
		}
		c, err := segment.Compile(r, now)
		if err != nil || c.Where != where {
			t.Errorf("%s: where %q, want %q (%v)", op, c.Where, where, err)
		}
		if strings.Contains(c.Where, "30") {
			t.Errorf("%s: the value leaked into the SQL text: %s", op, c.Where)
		}
	}
}

func TestAgeOutsideTheRangeOrOfTheWrongTypeIsRejected(t *testing.T) {
	for name, raw := range map[string]string{
		"below 13":         ageRule(ageCond("gte", 12)),
		"above 100":        ageRule(ageCond("lte", 101)),
		"negative":         ageRule(ageCond("gt", -1)),
		"a fraction":       ageRule(`{"field":"age","op":"gte","value":18.5}`),
		"a string":         ageRule(`{"field":"age","op":"gte","value":"18"}`),
		"a list":           ageRule(`{"field":"age","op":"gte","value":[18,25]}`),
		"neq":              ageRule(`{"field":"age","op":"neq","value":30}`),
		"in":               ageRule(`{"field":"age","op":"in","value":[30]}`),
		"no age is left":   ageRule(ageCond("gte", 40), ageCond("lt", 30)),
		"two different eq": ageRule(ageCond("eq", 30), ageCond("eq", 31)),
	} {
		if _, err := segment.Parse(raw); err == nil {
			t.Errorf("%s: must be rejected", name)
		}
	}
	_, err := segment.Parse(ageRule(ageCond("gte", 12)))
	if err == nil || !strings.Contains(err.Error(), "yaş 13 ile 100") {
		t.Errorf("the problem must name the allowed range: %v", err)
	}
	// The ends of the range are allowed, and "any" rules are not checked for an
	// empty intersection ("under 20 or over 60").
	for _, raw := range []string{
		ageRule(ageCond("gte", 13)), ageRule(ageCond("lte", 100)),
		strings.Replace(ageRule(ageCond("lt", 20), ageCond("gt", 60)), `"all"`, `"any"`, 1),
	} {
		if _, err := segment.Parse(raw); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
}

// Every age condition selects exactly the people PostgreSQL's own age() puts
// there, on the seeded database. People without a birth date never match.
func TestAgeCountsMatchPostgresAge(t *testing.T) {
	db, now := seeded(t)
	ctx := context.Background()
	var noBirth int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE birth_date IS NULL`).Scan(&noBirth); err != nil || noBirth == 0 {
		t.Fatalf("the seed must have people without a birth date: %d %v", noBirth, err)
	}
	sym := map[string]string{"eq": "=", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}
	for _, n := range []int{13, 17, 18, 24, 25, 26, 40, 64, 65, 75, 76, 100} {
		for op, s := range sym {
			var want int
			if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE extract(year FROM age($1::date, birth_date))::int `+s+` $2`,
				now.Format(time.DateOnly), n).Scan(&want); err != nil {
				t.Fatal(err)
			}
			if got := count(t, db, ageRule(ageCond(op, n)), now); got.Matched != want {
				t.Errorf("age %s %d: matched %d, want %d", op, n, got.Matched, want)
			}
		}
	}
}

const ageRangeIstanbul = 330

// The brief "18-25 yaş arası İstanbul'daki kullanıcılar": the seed is dealt
// relative to today, so the count is the same every day.
func TestAgeRangeInACityGoldenCount(t *testing.T) {
	db, now := seeded(t)
	raw := ageRule(ageCond("gte", 18), ageCond("lte", 25), `{"field":"city","op":"eq","value":"İstanbul"}`)
	got := count(t, db, raw, now)
	var want int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE city = 'İstanbul'
		AND extract(year FROM age($1::date, birth_date))::int BETWEEN 18 AND 25`, now.Format(time.DateOnly)).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if got.Matched != want || got.Matched != ageRangeIstanbul {
		t.Fatalf("matched %d, SQL %d, golden %d", got.Matched, want, ageRangeIstanbul)
	}
}

// Birthdays at the edges: today, tomorrow, yesterday, and 29 February.
func TestAgeAtTheBirthdayEdges(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	births := []string{"2008-11-19", "2008-11-20", "2008-11-18", "1951-11-19", "1950-11-20", "2010-02-28", "2010-03-01", "2008-02-29", ""}
	for _, b := range births {
		var v any
		if b != "" {
			v = b
		}
		if _, err := pool.Exec(ctx, `INSERT INTO users (first_name, last_name, city, sms_consent, email_consent, created_at, birth_date)
			VALUES ('A', 'B', 'İstanbul', false, false, now(), $1::date)`, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, now := range []time.Time{
		time.Date(2026, 11, 19, 0, 0, 1, 0, time.UTC),
		time.Date(2026, 11, 19, 23, 59, 59, 0, time.UTC),
		time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2027, 2, 28, 12, 0, 0, 0, time.UTC),
		time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC),
	} {
		for _, n := range []int{16, 17, 18, 19, 20, 74, 75, 76} {
			for op, s := range map[string]string{"eq": "=", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="} {
				var want int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE extract(year FROM age($1::date, birth_date))::int `+s+` $2`,
					now.Format(time.DateOnly), n).Scan(&want); err != nil {
					t.Fatal(err)
				}
				if got := count(t, pool, ageRule(ageCond(op, n)), now); got.Matched != want {
					t.Errorf("%s: age %s %d matched %d, want %d", now.Format(time.DateOnly), op, n, got.Matched, want)
				}
			}
		}
	}
	// Spelled out for one day: on 19.11.2026 the one born 19.11.2008 turned 18
	// today and the one born 20.11.2008 turns 18 only tomorrow.
	if got := count(t, pool, ageRule(ageCond("eq", 18)), time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)); got.Matched != 3 {
		t.Errorf("18-year-olds on 19.11.2026 = %d, want 3 (born 29.02.2008, 18.11.2008 and 19.11.2008)", got.Matched)
	}
	if got := count(t, pool, ageRule(ageCond("lt", 18)), time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)); got.Matched != 3 {
		t.Errorf("under 18 on 19.11.2026 = %d, want 3 (born 20.11.2008, 28.02.2010 and 01.03.2010)", got.Matched)
	}
}

func TestYearsBeforeOnALeapDay(t *testing.T) {
	leap := time.Date(2028, 2, 29, 15, 0, 0, 0, time.UTC)
	if got := segment.YearsBefore(leap, 18); got != time.Date(2010, 2, 28, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("18 years before 29.02.2028 = %s", got.Format(time.DateOnly))
	}
	if got := segment.YearsBefore(leap, 20); got != time.Date(2008, 2, 29, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("20 years before 29.02.2028 = %s", got.Format(time.DateOnly))
	}
}
