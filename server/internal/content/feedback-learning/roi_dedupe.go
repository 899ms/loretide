package feedbacklearning

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Dedupe keys (contract §4, FR-026, D14-V11).
//
// A key says "these two records look like the same business event". Equal
// keys make a write stop with possible_duplicate until the caller confirms;
// they never merge or drop anything by themselves. The key is a hint, not a
// guarantee: two real costs with the same category, day and amount collide,
// and an order number copied down one digit wrong does not.
//
// Normalization is: trimmed, inner whitespace collapsed to one space. There
// is NO case folding, for the same reason oneOf does none: "A-1"
// and "a-1" are different order numbers as far as anybody here knows.
// Dates are the brand's calendar day, so the same instant lands on the day
// the operator saw it on.
//
// Contract §4 also asks for Unicode NFC. That needs golang.org/x/text, which
// is not an approved import for content modules (scripts/content-boundaries
// .json upstreamImports), and this PR may only add an adapters line there.
// Until that import is approved, two spellings of one character that differ
// only in composition give different keys - a missed hint, never a wrong
// merge, because a key only ever asks the operator to confirm.

func normalizeKeyText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func keyDate(instant time.Time, location *time.Location) string {
	if location == nil {
		location = time.UTC
	}
	return instant.In(location).Format(time.DateOnly)
}

func hashKey(kind string, parts ...string) string {
	sum := sha256.Sum256([]byte(kind + "\x1f" + strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func optionalInt(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

// CostDedupeKey: category, day, currency, the stated money (amount, or labor
// minutes and rate) and whether it is ad spend. Never empty.
func CostDedupeKey(cost CostRevision, location *time.Location) string {
	money := "amount:" + optionalInt(cost.amountMinor)
	if cost.Pricing == PricingLaborTime {
		money = "labor:" + optionalInt(cost.LaborMinutes) + "/" + optionalInt(cost.laborRateMinor)
	}
	return hashKey("cost",
		normalizeKeyText(cost.Category),
		keyDate(cost.IncurredAt, location),
		cost.Currency,
		money,
		strconv.FormatBool(cost.AdSpend),
	)
}

// LeadDedupeKey: the customer label and the day first seen. A lead with no
// label has key "" and takes no part in duplicate detection - "we do not know
// who this is" twice on one day is two leads, not one.
func LeadDedupeKey(customerRef string, firstSeenAt time.Time, location *time.Location) string {
	label := normalizeKeyText(customerRef)
	if label == "" {
		return ""
	}
	return hashKey("lead", label, keyDate(firstSeenAt, location))
}

// DealDedupeKey: the order reference alone when there is one - the same order
// number is the same order whatever else differs; otherwise lead, day,
// currency and amount.
func DealDedupeKey(orderRef, leadID string, closedAt time.Time, currency string, amountMinor int64, location *time.Location) string {
	if order := normalizeKeyText(orderRef); order != "" {
		return hashKey("deal-order", order)
	}
	return hashKey("deal",
		normalizeKeyText(leadID),
		keyDate(closedAt, location),
		currency,
		strconv.FormatInt(amountMinor, 10),
	)
}
