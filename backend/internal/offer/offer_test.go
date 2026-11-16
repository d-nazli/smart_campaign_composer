package offer

import "testing"

func TestPercentsAndAmounts(t *testing.T) {
	pc := []struct {
		in   string
		want []float64
	}{
		{"%15 indirim", []float64{15}},
		{"15% indirim", []float64{15}},
		{"% 15 indirim", []float64{15}},
		{"Yüzde 15 indirim", []float64{15}},
		{"YÜZDE 15", []float64{15}},
		{"%12,5 indirim", []float64{12.5}},
		{"%15 ve %20", []float64{15, 20}},
		{"indirim yok", nil},
	}
	for _, tt := range pc {
		got := Percents(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("Percents(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Percents(%q) = %v, want %v", tt.in, got, tt.want)
			}
		}
	}
	am := []struct {
		in   string
		want float64
	}{
		{"100 TL indirim", 100}, {"100TL", 100}, {"₺100", 100}, {"100 ₺", 100}, {"100 lira", 100}, {"1.000 TL", 1000}, {"1.250,50 TL", 1250.5},
	}
	for _, tt := range am {
		got := Amounts(tt.in)
		if len(got) != 1 || got[0] != tt.want {
			t.Errorf("Amounts(%q) = %v, want [%v]", tt.in, got, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	good := []Offer{
		{Kind: KindNone}, {}, {Kind: KindPercent, Value: 15}, {Kind: KindPercent, Value: 100}, {Kind: KindAmount, Value: 100},
		{Kind: KindFreeShipping}, {Kind: KindCouponCode, Code: "HOSGELDIN10"}, {Kind: KindPercent, Value: 15, Code: "YAZ-25"},
	}
	for _, o := range good {
		if err := o.Validate(); err != nil {
			t.Errorf("%+v should be valid: %v", o, err)
		}
	}
	bad := []Offer{
		{Kind: KindPercent}, {Kind: KindPercent, Value: 101}, {Kind: KindPercent, Value: -5}, {Kind: KindAmount}, {Kind: KindAmount, Value: 2e6},
		{Kind: KindFreeShipping, Value: 5}, {Kind: KindCouponCode}, {Kind: KindCouponCode, Code: "a b"}, {Kind: "gift"}, {Kind: KindNone, Value: 5},
	}
	for _, o := range bad {
		if err := o.Validate(); err == nil {
			t.Errorf("%+v should be invalid", o)
		}
	}
}

func TestEqualIgnoresCodeCaseAndUnusedFields(t *testing.T) {
	if !(Offer{Kind: KindCouponCode, Code: "abc10"}).Equal(Offer{Kind: KindCouponCode, Code: "ABC10"}) {
		t.Error("codes should compare case-insensitively")
	}
	if !(Offer{}).Equal(Offer{Kind: KindNone}) {
		t.Error("zero offer is none")
	}
	if (Offer{Kind: KindPercent, Value: 15}).Equal(Offer{Kind: KindPercent, Value: 20}) {
		t.Error("different values must differ")
	}
}
