package feedbacklearning

import (
	"testing"
	"time"
)

// T014: dedupe keys (contract §4).

func dedupeCost(t *testing.T, category, amount string) CostRevision {
	t.Helper()
	cost, err := ValidateCost(CostInput{
		Category: category, Pricing: "amount", Amount: &amount, Currency: "CNY",
		IncurredAt: "2026-09-01T02:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	return cost
}

func shanghai(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("no tz database: %v", err)
	}
	return location
}

func TestTheSameCostGivesTheSameKey(t *testing.T) {
	location := shanghai(t)
	first := CostDedupeKey(dedupeCost(t, "拍摄", "3000.00"), location)
	if first == "" || first != CostDedupeKey(dedupeCost(t, "拍摄", "3000.00"), location) {
		t.Fatal("the same input gave two keys")
	}
	// Surrounding and repeated whitespace is not a difference.
	if CostDedupeKey(dedupeCost(t, "  拍摄  ", "3000.00"), location) != first {
		t.Error("leading/trailing whitespace changed the key")
	}
	if CostDedupeKey(dedupeCost(t, "外景  拍摄", "3000.00"), location) !=
		CostDedupeKey(dedupeCost(t, "外景 拍摄", "3000.00"), location) {
		t.Error("inner whitespace runs changed the key")
	}
	// Case is a difference: nothing here knows two spellings are one thing.
	if CostDedupeKey(dedupeCost(t, "Shoot", "3000.00"), location) ==
		CostDedupeKey(dedupeCost(t, "shoot", "3000.00"), location) {
		t.Error("case folding happened")
	}
	if CostDedupeKey(dedupeCost(t, "拍摄", "3000.01"), location) == first {
		t.Error("a different amount gave the same key")
	}
}

// The day is the brand's day: 2026-09-01T17:00Z is 09-02 in Shanghai.
func TestTheKeyUsesTheBrandsCalendarDay(t *testing.T) {
	location := shanghai(t)
	lateUTC := time.Date(2026, 9, 1, 17, 0, 0, 0, time.UTC)
	nextMorning := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	if LeadDedupeKey("客户-0412", lateUTC, location) != LeadDedupeKey("客户-0412", nextMorning, location) {
		t.Error("two instants on the same Shanghai day gave different keys")
	}
	if LeadDedupeKey("客户-0412", lateUTC, time.UTC) == LeadDedupeKey("客户-0412", nextMorning, time.UTC) {
		t.Error("the UTC days differ, so the UTC keys must too")
	}
}

// A lead nobody can name has no key and takes no part.
func TestALeadWithNoLabelHasNoKey(t *testing.T) {
	if key := LeadDedupeKey("   ", time.Now(), time.UTC); key != "" {
		t.Fatalf("an unlabelled lead has key %q", key)
	}
}

// An order reference decides alone when there is one.
func TestADealWithAnOrderReferenceIsKeyedByItAlone(t *testing.T) {
	day := time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC)
	other := day.AddDate(0, 1, 0)
	withOrder := DealDedupeKey("TB-8812", "lead-1", day, "CNY", 1000000, time.UTC)
	if withOrder != DealDedupeKey(" TB-8812 ", "lead-2", other, "USD", 5, time.UTC) {
		t.Error("with an order reference, nothing else may change the key")
	}
	if withOrder == DealDedupeKey("tb-8812", "lead-1", day, "CNY", 1000000, time.UTC) {
		t.Error("order references differing in case were folded")
	}
	withoutOrder := DealDedupeKey("", "lead-1", day, "CNY", 1000000, time.UTC)
	if withoutOrder == DealDedupeKey("", "lead-1", day, "CNY", 1000001, time.UTC) {
		t.Error("without an order reference, the amount must be part of the key")
	}
	if withoutOrder == DealDedupeKey("", "lead-2", day, "CNY", 1000000, time.UTC) {
		t.Error("without an order reference, the lead must be part of the key")
	}
}

// Contract §4: Unicode NFC. "é" as one code point and as "e" plus a combining
// acute accent are the same text, and give the same key - for every text part
// of every key.
func TestTheKeyIsTheSameForComposedAndDecomposedText(t *testing.T) {
	composed, decomposed := "Café 拍摄", "Cafe\u0301 拍摄"
	if composed == decomposed {
		t.Fatal("the two spellings are byte-equal; the test proves nothing")
	}
	location := shanghai(t)
	if CostDedupeKey(dedupeCost(t, composed, "3000.00"), location) !=
		CostDedupeKey(dedupeCost(t, decomposed, "3000.00"), location) {
		t.Error("cost: composed and decomposed categories gave different keys")
	}
	day := time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC)
	if LeadDedupeKey("客户-é", day, time.UTC) != LeadDedupeKey("客户-e\u0301", day, time.UTC) {
		t.Error("lead: composed and decomposed customer labels gave different keys")
	}
	if DealDedupeKey("TB-é", "", day, "CNY", 1, time.UTC) != DealDedupeKey("TB-e\u0301", "", day, "CNY", 1, time.UTC) {
		t.Error("deal: composed and decomposed order references gave different keys")
	}
	// NFC is not case folding and not accent stripping.
	if LeadDedupeKey("客户-é", day, time.UTC) == LeadDedupeKey("客户-e", day, time.UTC) {
		t.Error("an accent was stripped")
	}
}
