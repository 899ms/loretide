package feedbacklearning

import (
	"encoding/json"
	"slices"
	"testing"
)

// specs/034 FR-001 to FR-005: the currency table, amount strings, and the one
// rounding rule.

// T011: the table is ruling Q1=A's nine, with their minor-unit places.
func TestTheCurrencyTableIsTheRulingsNine(t *testing.T) {
	want := []Currency{
		{"CNY", 2}, {"HKD", 2}, {"TWD", 2}, {"USD", 2}, {"EUR", 2},
		{"GBP", 2}, {"SGD", 2}, {"JPY", 0}, {"KRW", 0},
	}
	if !slices.Equal(Currencies, want) {
		t.Fatalf("currencies = %v, want %v", Currencies, want)
	}
	if _, ok := CurrencyDigits("RMB"); ok {
		t.Error("RMB is not an ISO code and must not be accepted")
	}
	if _, ok := CurrencyDigits("cny"); ok {
		t.Error("a lower-case code must not be accepted")
	}
}

// T011: accepted strings and their minor units.
func TestAnAmountStringBecomesExactMinorUnits(t *testing.T) {
	cases := []struct {
		text, currency string
		want           int64
	}{
		{"3000.00", "CNY", 300000},
		{"3000", "CNY", 300000},
		{"3000.5", "CNY", 300050},
		{"100", "JPY", 100},
		{"-0.01", "CNY", -1},
		{"0", "KRW", 0},
		{"10000000000000.00", "CNY", 1_000_000_000_000_000},
	}
	for _, tc := range cases {
		got, err := ParseAmount("amount", tc.text, tc.currency)
		if err != nil {
			t.Errorf("%q %s: %v", tc.text, tc.currency, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q %s = %d, want %d", tc.text, tc.currency, got, tc.want)
		}
	}
}

// T011: refused strings, each naming the field. The system does not round a
// third decimal place away for the user, and does not guess what "1,000" or
// "1e3" meant.
func TestAnAmountStringOutsideTheFormatIsRefusedByName(t *testing.T) {
	cases := []struct{ name, text, currency string }{
		{"too many places", "3000.005", "CNY"},
		{"places on a zero-digit currency", "1.5", "JPY"},
		{"over 10^15", "10000000000000.01", "CNY"},
		{"leading plus", "+10.00", "CNY"},
		{"space", " 10.00", "CNY"},
		{"inner space", "10 000", "CNY"},
		{"thousands comma", "1,000.00", "CNY"},
		{"exponent", "1e3", "CNY"},
		{"empty", "", "CNY"},
		{"bare point", "10.", "CNY"},
		{"bare minus", "-", "CNY"},
		{"leading point", ".5", "CNY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAmount("amount", tc.text, tc.currency)
			assertField(t, err, "amount")
		})
	}
	_, err := ParseAmount("amount", "10.00", "RMB")
	assertField(t, err, "currency")
}

// The storage spelling round-trips.
func TestFormatAmountWritesTheCurrencysPlaces(t *testing.T) {
	cases := []struct {
		minor    int64
		currency string
		want     string
	}{
		{300000, "CNY", "3000.00"},
		{-1, "CNY", "-0.01"},
		{5, "USD", "0.05"},
		{100, "JPY", "100"},
		{-250, "KRW", "-250"},
	}
	for _, tc := range cases {
		if got := FormatAmount(tc.minor, tc.currency); got != tc.want {
			t.Errorf("FormatAmount(%d, %s) = %q, want %q", tc.minor, tc.currency, got, tc.want)
		}
		back, err := ParseAmount("amount", tc.want, tc.currency)
		if err != nil || back != tc.minor {
			t.Errorf("%q did not round-trip: %d, %v", tc.want, back, err)
		}
	}
}

// An amount leaves the API as a string, never a JSON number.
func TestMinorMarshalsAsAString(t *testing.T) {
	encoded, err := json.Marshal(struct {
		A Minor  `json:"a"`
		B *Minor `json:"b"`
	}{A: 1_000_000_000_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"a":"1000000000000000","b":null}` {
		t.Fatalf("encoded %s", encoded)
	}
}

// T012: FR-011 labor, half away from zero.
func TestLaborTimeIsMinutesTimesRateRoundedHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		minutes, rate, want int64
	}{
		{360, 15000, 90000},
		{1, 1, 0},
		{30, 1, 1},
		{90, 1, 2},
		{-30, 1, -1},
	}
	for _, tc := range cases {
		got, err := LaborAmount(tc.minutes, tc.rate)
		if err != nil {
			t.Fatalf("%d x %d: %v", tc.minutes, tc.rate, err)
		}
		if got != tc.want {
			t.Errorf("%d min x %d/h = %d, want %d", tc.minutes, tc.rate, got, tc.want)
		}
	}
}

// T012: a labor cost with no rate is still saved, with no amount and a reason -
// never an amount of 0.
func TestALaborCostWithoutARateHasNoAmountAndSaysWhy(t *testing.T) {
	minutes := int64(360)
	cost, err := ValidateCost(CostInput{
		Category: "剪辑", Pricing: "labor_time", Currency: "CNY", LaborMinutes: &minutes,
		IncurredAt: "2026-09-01T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cost.AmountMinor != nil || cost.Amount != nil {
		t.Fatalf("a cost with no rate has amount %v / %v", cost.AmountMinor, cost.Amount)
	}
	if cost.AmountStatus != AmountLaborRateMissing {
		t.Fatalf("amount status = %q, want %q", cost.AmountStatus, AmountLaborRateMissing)
	}

	rate := "150.00"
	priced, err := ValidateCost(CostInput{
		Category: "剪辑", Pricing: "labor_time", Currency: "CNY", LaborMinutes: &minutes,
		LaborRate: &rate, IncurredAt: "2026-09-01T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if priced.AmountMinor == nil || *priced.AmountMinor != 90000 || *priced.Amount != "900.00" {
		t.Fatalf("360 min at 150.00/h = %v, want 90000 (900.00)", priced.AmountMinor)
	}
	if priced.AmountStatus != AmountOK {
		t.Fatalf("amount status = %q", priced.AmountStatus)
	}

	// The server computes a labor amount; a client-supplied one is refused
	// rather than silently preferred or silently ignored.
	amount := "900.00"
	_, err = ValidateCost(CostInput{
		Category: "剪辑", Pricing: "labor_time", Currency: "CNY", LaborMinutes: &minutes,
		LaborRate: &rate, Amount: &amount, IncurredAt: "2026-09-01T10:00:00Z",
	})
	assertField(t, err, "amount")
}

// Deal gross basis: each basis takes exactly its own input.
func TestADealTakesOnlyTheInputItsGrossBasisUses(t *testing.T) {
	amount, profit, cogs := "10000.00", "3000.00", "7000.00"
	base := DealInput{Amount: &amount, Currency: "CNY", ClosedAt: "2026-09-01T10:00:00Z"}

	stated := base
	stated.GrossBasis, stated.GrossProfit = "stated_gross_profit", &profit
	deal, err := ValidateDeal(stated)
	if err != nil || deal.GrossProfitMinor == nil || *deal.GrossProfitMinor != 300000 {
		t.Fatalf("stated gross profit: %v %v", deal.GrossProfitMinor, err)
	}

	fromCOGS := base
	fromCOGS.GrossBasis, fromCOGS.COGS = "cogs", &cogs
	deal, err = ValidateDeal(fromCOGS)
	if err != nil || deal.COGSMinor == nil || *deal.COGSMinor != 700000 || deal.GrossProfitMinor != nil {
		t.Fatalf("cogs basis stores its input only: %v %v %v", deal.COGSMinor, deal.GrossProfitMinor, err)
	}

	missing := base
	missing.GrossBasis = "cogs"
	_, err = ValidateDeal(missing)
	assertField(t, err, "cogs")

	crossed := base
	crossed.GrossBasis, crossed.GrossProfit = "none", &profit
	_, err = ValidateDeal(crossed)
	assertField(t, err, "gross_profit")
}

// A refund is a positive reduction.
func TestARefundMustBePositive(t *testing.T) {
	zero := "0.00"
	_, err := ValidateAdjustment(AdjustmentInput{Kind: "refund", Amount: &zero, Currency: "CNY", OccurredAt: "2026-09-01T10:00:00Z"})
	assertField(t, err, "amount")
	negative := "-1.00"
	adjustment, err := ValidateAdjustment(AdjustmentInput{Kind: "adjustment", Amount: &negative, Currency: "CNY", OccurredAt: "2026-09-01T10:00:00Z"})
	if err != nil || adjustment.RevenueDeltaMinor != -100 {
		t.Fatalf("an adjustment may be negative: %v %v", adjustment.RevenueDeltaMinor, err)
	}
}

// FR-024 in isolation: a refund past the net is refused, a revision replaces
// its own previous amount rather than adding to it, and a currency mismatch is
// named.
func TestTheNetCheckCountsEachAdjustmentOnce(t *testing.T) {
	existing := []AdjustmentRevision{
		{AdjustmentID: "a1", RevenueDeltaMinor: 60000, Currency: "CNY"},
		{AdjustmentID: "a2", RevenueDeltaMinor: 30000, Currency: "CNY", Voided: true},
	}
	next := AdjustmentRevision{RevenueDeltaMinor: 40000}
	if err := checkNet(100000, "CNY", existing, "", &next); err != nil {
		t.Fatalf("100000 - 60000 - 40000 = 0 is allowed: %v", err)
	}
	over := AdjustmentRevision{RevenueDeltaMinor: 40001}
	assertField(t, checkNet(100000, "CNY", existing, "", &over), "amount")
	// Revising a1 up to 100000 replaces it; it is not 60000 + 100000.
	revised := AdjustmentRevision{RevenueDeltaMinor: 100000}
	if err := checkNet(100000, "CNY", existing, "a1", &revised); err != nil {
		t.Fatalf("revising a1 counted it twice: %v", err)
	}
	foreign := []AdjustmentRevision{{AdjustmentID: "a3", RevenueDeltaMinor: 1, Currency: "USD"}}
	assertField(t, checkNet(100000, "CNY", foreign, "", nil), "currency")
}
