package feedbacklearning

import (
	"math/big"
	"strconv"
	"strings"
)

// Money for specs/034 (FR-001 to FR-005).
//
// Every amount is an integer count of its currency's minor unit, carried with
// the ISO code. It enters the API as a decimal STRING ("3000.00") and leaves it
// as one: a JSON number may already have been through a float on the way in,
// and nothing here can tell. There is no float anywhere in this file, and a
// guard test scans for one.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §3

// MaxAbsMinor bounds one stored amount, |amount_minor| <= 10^15 (spec "极大
// 金额"). Totals are computed with math/big, so the bound is about keeping a
// typo of six extra zeros out of storage, not about overflow.
const MaxAbsMinor int64 = 1_000_000_000_000_000

// Currency is one row of the currency table: an ISO 4217 code and how many
// decimal places its minor unit has.
type Currency struct {
	Code   string
	Digits int
}

// Currencies is ruling Q1=A, exactly nine. Adding one is a code change, not a
// migration: the column is plain text and this table is the authority.
var Currencies = []Currency{
	{Code: "CNY", Digits: 2},
	{Code: "HKD", Digits: 2},
	{Code: "TWD", Digits: 2},
	{Code: "USD", Digits: 2},
	{Code: "EUR", Digits: 2},
	{Code: "GBP", Digits: 2},
	{Code: "SGD", Digits: 2},
	{Code: "JPY", Digits: 0},
	{Code: "KRW", Digits: 0},
}

// CurrencyDigits answers the minor-unit places of a code, exact match only.
func CurrencyDigits(code string) (int, bool) {
	for _, currency := range Currencies {
		if currency.Code == code {
			return currency.Digits, true
		}
	}
	return 0, false
}

// Minor is an amount in minor units. It marshals as a decimal integer string,
// never a JSON number, so a JavaScript reader cannot lose precision above 2^53
// and cannot be tempted to do arithmetic on it.
type Minor int64

func (m Minor) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strconv.FormatInt(int64(m), 10) + `"`), nil
}

func minorPtr(value *int64) *Minor {
	if value == nil {
		return nil
	}
	converted := Minor(*value)
	return &converted
}

// ParseAmount reads a decimal string in the given currency into minor units.
//
// Accepted: an optional leading '-', digits, and optionally '.' followed by at
// most the currency's number of places. Refused, naming field: an empty
// string, a leading '+', any space, a thousands separator, an exponent, too
// many decimal places (FR-003: the system does not round for the user), and a
// magnitude above MaxAbsMinor. An unknown currency is refused naming
// "currency".
func ParseAmount(field, text, currency string) (int64, error) {
	digits, ok := CurrencyDigits(currency)
	if !ok {
		return 0, FieldError{Field: "currency", Reason: "not a supported currency"}
	}
	notDecimal := FieldError{Field: field, Reason: "not a decimal string"}
	body := text
	negative := false
	if strings.HasPrefix(body, "-") {
		negative = true
		body = body[1:]
	}
	whole, fraction, hasPoint := strings.Cut(body, ".")
	if whole == "" || !allDigits(whole) || (hasPoint && (fraction == "" || !allDigits(fraction))) {
		return 0, notDecimal
	}
	if len(fraction) > digits {
		return 0, FieldError{Field: field, Reason: "more decimal places than the currency has"}
	}
	fraction += strings.Repeat("0", digits-len(fraction))
	value, ok := new(big.Int).SetString(whole+fraction, 10)
	if !ok {
		return 0, notDecimal
	}
	if negative {
		value.Neg(value)
	}
	if !withinBound(value) {
		return 0, FieldError{Field: field, Reason: "amount out of range"}
	}
	return value.Int64(), nil
}

func allDigits(text string) bool {
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func withinBound(value *big.Int) bool {
	return new(big.Int).Abs(value).Cmp(big.NewInt(MaxAbsMinor)) <= 0
}

// FormatAmount writes minor units back as the currency's decimal string:
// 300000 CNY is "3000.00", -1 CNY is "-0.01", 100 JPY is "100". This is the
// storage spelling, with an ASCII minus; the display minus (U+2212) is the
// report's business.
func FormatAmount(minor int64, currency string) string {
	digits, ok := CurrencyDigits(currency)
	text := strconv.FormatInt(minor, 10)
	if !ok || digits == 0 {
		return text
	}
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign, text = "-", text[1:]
	}
	if len(text) <= digits {
		text = strings.Repeat("0", digits-len(text)+1) + text
	}
	return sign + text[:len(text)-digits] + "." + text[len(text)-digits:]
}

func formatAmountPtr(minor *int64, currency string) *string {
	if minor == nil {
		return nil
	}
	text := FormatAmount(*minor, currency)
	return &text
}

// LaborAmount is FR-011: minutes x hourly rate / 60, rounded half away from
// zero (FR-005, ruling Q2=A). The one rounding rule in the module; allocations
// do not round, they use the largest-remainder rule (PR 2).
func LaborAmount(minutes, rateMinor int64) (int64, error) {
	product := new(big.Int).Mul(big.NewInt(minutes), big.NewInt(rateMinor))
	rounded := divRoundHalfAwayFromZero(product, big.NewInt(60))
	if !withinBound(rounded) {
		return 0, FieldError{Field: "labor_rate", Reason: "amount out of range"}
	}
	return rounded.Int64(), nil
}

// divRoundHalfAwayFromZero divides and rounds, halves going away from zero.
// denominator must be positive.
func divRoundHalfAwayFromZero(numerator, denominator *big.Int) *big.Int {
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	twice := new(big.Int).Abs(remainder)
	twice.Lsh(twice, 1)
	if twice.Cmp(denominator) >= 0 {
		if numerator.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return quotient
}

// UnmarshalJSON reads the decimal integer string MarshalJSON writes, so a
// stored snapshot of a revision reads back as the same revision. A JSON
// number is refused: it may already have been through a float.
func (m *Minor) UnmarshalJSON(data []byte) error {
	text, err := strconv.Unquote(string(data))
	if err != nil || text == "" || (text[0] == '-' && len(text) == 1) {
		return ErrInvalid
	}
	value, parseErr := strconv.ParseInt(text, 10, 64)
	if parseErr != nil {
		return ErrInvalid
	}
	*m = Minor(value)
	return nil
}

// MaxRatePlaces is FR-006: a rate is a decimal string of at most twelve
// places.
const MaxRatePlaces = 12

// ParseRate reads a user-entered exchange rate: units of the target currency
// per one unit of the source, a positive decimal string with at most twelve
// places. The same spelling rules as an amount: no sign, no spaces, no
// separators, no exponent.
func ParseRate(field, text string) (*big.Rat, error) {
	refused := FieldError{Field: field, Reason: "not a positive decimal rate"}
	whole, fraction, hasPoint := strings.Cut(text, ".")
	if whole == "" || !allDigits(whole) || (hasPoint && (fraction == "" || !allDigits(fraction))) {
		return nil, refused
	}
	if len(fraction) > MaxRatePlaces || len(whole) > 15 {
		return nil, refused
	}
	rate, ok := new(big.Rat).SetString(text)
	if !ok || rate.Sign() <= 0 {
		return nil, refused
	}
	return rate, nil
}

// ConvertMinor converts an amount in minor units of one currency into minor
// units of another at rate (target units per source unit), rounded half away
// from zero (FR-005). Exact until that one rounding.
func ConvertMinor(minor *big.Int, fromDigits, toDigits int, rate *big.Rat) *big.Int {
	numerator := new(big.Int).Mul(minor, rate.Num())
	numerator.Mul(numerator, pow10(toDigits))
	denominator := new(big.Int).Mul(rate.Denom(), pow10(fromDigits))
	return divRoundHalfAwayFromZero(numerator, denominator)
}

func pow10(places int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil)
}

// displayMinus is the minus sign a report shows (contract §5.2): U+2212, not
// the ASCII hyphen the stored spelling uses.
const displayMinus = "−"

// FormatRat writes a rational number with exactly places decimals, rounded
// half away from zero, with U+2212 for a negative result. A value that
// rounds to zero is written without a sign.
func FormatRat(value *big.Rat, places int) string {
	scaled := new(big.Int).Mul(value.Num(), pow10(places))
	rounded := divRoundHalfAwayFromZero(scaled, value.Denom())
	sign := ""
	if rounded.Sign() < 0 {
		sign = displayMinus
		rounded.Neg(rounded)
	}
	text := rounded.String()
	if places == 0 {
		return sign + text
	}
	if len(text) <= places {
		text = strings.Repeat("0", places-len(text)+1) + text
	}
	return sign + text[:len(text)-places] + "." + text[len(text)-places:]
}
