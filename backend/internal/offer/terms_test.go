package offer

import (
	"reflect"
	"strings"
	"testing"
)

var termsOffer = Offer{Kind: KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}

func TestTermsAreValidated(t *testing.T) {
	ok := []Offer{
		termsOffer,
		{Kind: KindPercent, Value: 10, MinSpend: 500.5},
		{Kind: KindFreeShipping, ExpiresAt: "2027-01-01"},
		{Kind: KindCouponCode, Code: "YAZ25", MinSpend: 1000, ExpiresAt: "2026-12-31"},
	}
	for _, o := range ok {
		if err := o.Validate(); err != nil {
			t.Errorf("%+v: %v", o, err)
		}
	}
	bad := map[string]Offer{
		"negatif alt limit": {Kind: KindAmount, Value: 100, MinSpend: -1},
		"çok büyük":         {Kind: KindAmount, Value: 100, MinSpend: MaxAmount + 1},
		"tarih biçimi":      {Kind: KindAmount, Value: 100, ExpiresAt: "31.12.2026"},
		"imkansız tarih":    {Kind: KindAmount, Value: 100, ExpiresAt: "2026-02-31"},
		"none ile şart":     {Kind: KindNone, MinSpend: 1000},
		"none ile tarih":    {Kind: KindNone, ExpiresAt: "2026-12-31"},
	}
	for name, o := range bad {
		if err := o.Validate(); err == nil {
			t.Errorf("%s: %+v should be invalid", name, o)
		}
	}
}

func TestNormalizeDropsTermsOfAnOfferThatPromisesNothing(t *testing.T) {
	if got := (Offer{Kind: KindNone, MinSpend: 5, ExpiresAt: "2026-12-31"}).Normalize(); got != (Offer{Kind: KindNone}) {
		t.Fatalf("%+v", got)
	}
	if got := (Offer{Kind: KindAmount, Value: 1, MinSpend: 5, ExpiresAt: " 2026-12-31 "}).Normalize(); got.MinSpend != 5 || got.ExpiresAt != "2026-12-31" {
		t.Fatalf("terms must survive: %+v", got)
	}
}

func TestEqualComparesTheTerms(t *testing.T) {
	if !termsOffer.Equal(termsOffer) {
		t.Fatal("same offer")
	}
	for name, o := range map[string]Offer{
		"min": {Kind: KindAmount, Value: 100, MinSpend: 500, ExpiresAt: "2026-12-31"},
		"exp": {Kind: KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-30"},
		"yok": {Kind: KindAmount, Value: 100},
	} {
		if termsOffer.Equal(o) {
			t.Errorf("%s: offers with different terms are not equal", name)
		}
	}
}

func TestDescribeStatesTheTerms(t *testing.T) {
	for _, tt := range []struct {
		o    Offer
		want string
	}{
		{termsOffer, "100 TL indirim (1000 TL ve üzeri alışverişte, son kullanım 31.12.2026)"},
		{Offer{Kind: KindPercent, Value: 10, MinSpend: 1500.5}, "%10 indirim (1500,5 TL ve üzeri alışverişte)"},
		{Offer{Kind: KindFreeShipping, ExpiresAt: "2026-12-31"}, "ücretsiz kargo (son kullanım 31.12.2026)"},
		{Offer{Kind: KindAmount, Value: 100, Code: "YAZ25", MinSpend: 1000}, "100 TL indirim (kupon kodu YAZ25, 1000 TL ve üzeri alışverişte)"},
		{Offer{Kind: KindCouponCode, Code: "YAZ25", ExpiresAt: "2026-12-31"}, "kupon kodu YAZ25 (son kullanım 31.12.2026)"},
		{Offer{Kind: KindPercent, Value: 15}, "%15 indirim"},
	} {
		if got := tt.o.Describe(); got != tt.want {
			t.Errorf("Describe(%+v)\n got %q\nwant %q", tt.o, got, tt.want)
		}
	}
	if termsOffer.MinSpendPhrase() != "1000 TL ve üzeri alışverişte" || termsOffer.ExpiryPhrase() != "Son kullanım: 31.12.2026" {
		t.Fatalf("phrases: %q / %q", termsOffer.MinSpendPhrase(), termsOffer.ExpiryPhrase())
	}
	if (Offer{Kind: KindAmount, Value: 1}).MinSpendPhrase() != "" || (Offer{Kind: KindAmount, Value: 1}).ExpiryPhrase() != "" {
		t.Fatal("no terms, no phrases")
	}
}

func TestDatesAreReadInEveryCommonWriting(t *testing.T) {
	for in, want := range map[string][]string{
		"son kullanım 31.12.2026":                 {"2026-12-31"},
		"31/12/2026'ya kadar":                     {"2026-12-31"},
		"31-12-2026":                              {"2026-12-31"},
		"2026-12-31 tarihine kadar":               {"2026-12-31"},
		"31 Aralık 2026 tarihine kadar":           {"2026-12-31"},
		"1 Mayıs 2027":                            {"2027-05-01"},
		"5.1.2027":                                {"2027-01-05"},
		"31.12.2026 ve 15.01.2027":                {"2026-12-31", "2027-01-15"},
		"31.12.2026 ... 31 Aralık 2026":           {"2026-12-31"}, // the same day twice is one date
		"1.000 TL üzeri, 12.2026, 100 TL indirim": nil,
		"31.02.2026":                              nil, // impossible
		"indirim var":                             nil,
	} {
		if got := Dates(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Dates(%q) = %v, want %v", in, got, want)
		}
	}
	m := FindDates("Son kullanım 31.02.2026")
	if len(m) != 1 || m[0].Valid || m[0].ISO != "" {
		t.Fatalf("an impossible date must be reported, not dropped: %+v", m)
	}
}

func TestDayAndMonthWithoutAYearIsReported(t *testing.T) {
	if got := FindDayMonths("31 Aralık'a kadar geçerli"); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	if got := FindDayMonths("31 Aralık 2026'ya kadar"); len(got) != 0 {
		t.Fatalf("a full date is not year-less: %v", got)
	}
	if got := FindDayMonths("anneler günü, mayıs ayında"); len(got) != 0 {
		t.Fatalf("no day number: %v", got)
	}
}

func TestFormatDate(t *testing.T) {
	if FormatDate("2026-12-31") != "31.12.2026" || FormatDate("2027-01-05") != "05.01.2027" || FormatDate("") != "" {
		t.Fatal("dd.mm.yyyy")
	}
}

func TestMentionsRequiresTheTermsToo(t *testing.T) {
	full := "1000 TL üzeri alışverişte 100 TL indirim! Son kullanım: 31.12.2026"
	if !termsOffer.Mentions(full) {
		t.Fatal("a message with everything must pass")
	}
	for name, text := range map[string]string{
		"şart yok":         "100 TL indirim! Son kullanım: 31.12.2026",
		"tarih yok":        "1000 TL üzeri alışverişte 100 TL indirim!",
		"yanlış tarih":     "1000 TL üzeri alışverişte 100 TL indirim! Son kullanım: 30.12.2026",
		"yanlış alt limit": "500 TL üzeri alışverişte 100 TL indirim! Son kullanım: 31.12.2026",
		"indirim yok":      "1000 TL üzeri alışverişte indirim! Son kullanım: 31.12.2026",
	} {
		if termsOffer.Mentions(text) {
			t.Errorf("%s: %q must not pass", name, text)
		}
	}
	for _, text := range []string{
		"1.000 TL ve üzeri alışverişte 100 TL indirim, 31 Aralık 2026'ya kadar geçerli",
		"100 TL indirim: alışverişin 1000 TL'yi geçerse. SON KULLANIM 31/12/2026",
	} {
		if !termsOffer.Mentions(text) {
			t.Errorf("%q should pass", text)
		}
	}
	// A percent offer's minimum is checked as an amount too.
	p := Offer{Kind: KindPercent, Value: 10, MinSpend: 1000}
	if !p.Mentions("1000 TL üzeri alışverişte %10 indirim") || p.Mentions("%10 indirim") {
		t.Fatal("percent offer with a minimum")
	}
}

func TestDateConflicts(t *testing.T) {
	if got := termsOffer.DateConflicts("Son kullanım: 31.12.2026"); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	got := termsOffer.DateConflicts("Son kullanım: 30.11.2026, 31 Aralık 2026")
	if len(got) != 1 || got[0] != "30.11.2026" {
		t.Fatalf("%v", got)
	}
	// No expiry in the brief: any date in the text is an invention.
	noExp := Offer{Kind: KindAmount, Value: 100}
	if got := noExp.DateConflicts("Son kullanım: 31.12.2026"); len(got) != 1 || !strings.Contains(got[0], "31.12.2026") {
		t.Fatalf("%v", got)
	}
}
