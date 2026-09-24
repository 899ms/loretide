package feedbacklearning

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
)

// specs/034 PR 2, T041 / T042 / SC-003 (D14-V14): the largest-remainder rule.
// Pure; no database.

func weightShares(kinds ...string) []AllocationInput {
	inputs := []AllocationInput{}
	for i := 0; i+1 < len(kinds); i += 2 {
		one := int64(1)
		inputs = append(inputs, AllocationInput{TargetKind: kinds[i], TargetID: kinds[i+1], Weight: &one})
	}
	return inputs
}

func allocatedMinor(allocations []CostAllocation) []int64 {
	out := []int64{}
	for _, allocation := range allocations {
		out = append(out, int64(allocation.AllocatedMinor))
	}
	return out
}

// alloc-10000 (contract §5.4): 100.00 split 1:1:1 over three works is
// 3334/3333/3333, and the extra unit goes to the smallest target key however
// the request ordered the shares.
func TestAllocTenThousandOneToOneToOne(t *testing.T) {
	amount := int64(10000)
	for _, order := range [][]string{
		{"work", "w-a", "work", "w-b", "work", "w-c"},
		{"work", "w-c", "work", "w-b", "work", "w-a"},
		{"work", "w-b", "work", "w-c", "work", "w-a"},
	} {
		allocations, err := ResolveAllocations(&amount, "CNY", weightShares(order...))
		if err != nil {
			t.Fatal(err)
		}
		if got := allocatedMinor(allocations); !slices.Equal(got, []int64{3334, 3333, 3333}) {
			t.Fatalf("order %v: shares %v, want 3334/3333/3333", order, got)
		}
		if allocations[0].TargetID != "w-a" || allocations[0].Allocated != "33.34" {
			t.Fatalf("order %v: the extra unit went to %s (%s), want w-a", order, allocations[0].TargetID, allocations[0].Allocated)
		}
	}
}

func TestSevenEqualSharesOfOneHundredAddUpToOneHundred(t *testing.T) {
	amount := int64(100)
	allocations, err := ResolveAllocations(&amount, "CNY", weightShares(
		"period", "2026-01", "period", "2026-02", "period", "2026-03", "period", "2026-04",
		"period", "2026-05", "period", "2026-06", "period", "2026-07"))
	if err != nil {
		t.Fatal(err)
	}
	got := allocatedMinor(allocations)
	if !slices.Equal(got, []int64{15, 15, 14, 14, 14, 14, 14}) {
		t.Fatalf("shares %v", got)
	}
}

func TestANegativeAmountSplitsByItsMagnitude(t *testing.T) {
	amount := int64(-10000)
	allocations, err := ResolveAllocations(&amount, "CNY", weightShares("work", "w-a", "work", "w-b", "work", "w-c"))
	if err != nil {
		t.Fatal(err)
	}
	if got := allocatedMinor(allocations); !slices.Equal(got, []int64{-3334, -3333, -3333}) {
		t.Fatalf("shares %v, want -3334/-3333/-3333", got)
	}
}

func TestSplitsThatCannotBeDoneAreRefusedNamingAllocations(t *testing.T) {
	amount := int64(10000)
	zero, big := int64(0), int64(MaxWeight+1)
	a, b := "60.00", "40.01"
	bad := "1.001"
	cases := []struct {
		name   string
		amount *int64
		inputs []AllocationInput
	}{
		{"zero weight", &amount, []AllocationInput{{TargetKind: "work", TargetID: "w1", Weight: &zero}}},
		{"weight too large", &amount, []AllocationInput{{TargetKind: "work", TargetID: "w1", Weight: &big}}},
		{"amounts do not add up", &amount, []AllocationInput{
			{TargetKind: "work", TargetID: "w1", Amount: &a}, {TargetKind: "work", TargetID: "w2", Amount: &b}}},
		{"amount too precise", &amount, []AllocationInput{{TargetKind: "work", TargetID: "w1", Amount: &bad}}},
		{"both weight and amount", &amount, []AllocationInput{{TargetKind: "work", TargetID: "w1", Weight: &amount, Amount: &a}}},
		{"neither", &amount, []AllocationInput{{TargetKind: "work", TargetID: "w1"}}},
		{"mixed methods", &amount, []AllocationInput{
			{TargetKind: "work", TargetID: "w1", Weight: &amount}, {TargetKind: "work", TargetID: "w2", Amount: &a}}},
		{"unknown target kind", &amount, weightShares("project", "p1")},
		{"period not YYYY-MM", &amount, weightShares("period", "2026-9")},
		{"empty target", &amount, weightShares("work", "")},
		{"same target twice", &amount, weightShares("work", "w1", "work", "w1")},
		{"labor cost without a rate", nil, weightShares("work", "w1")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveAllocations(tc.amount, "CNY", tc.inputs)
			fieldErr, ok := errors.AsType[FieldError](err)
			if !ok || fieldErr.Field != "allocations" {
				t.Fatalf("err = %v, want a refusal naming allocations", err)
			}
		})
	}
	// Amounts that do add up are taken as given.
	c := "40.00"
	allocations, err := ResolveAllocations(&amount, "CNY", []AllocationInput{
		{TargetKind: "work", TargetID: "w2", Amount: &c}, {TargetKind: "work", TargetID: "w1", Amount: &a}})
	if err != nil || !slices.Equal(allocatedMinor(allocations), []int64{6000, 4000}) {
		t.Fatalf("amounts = %v, %v", allocations, err)
	}
	// All weights zero is refused by the splitter itself, too.
	if _, err = SplitLargestRemainder(100, []int64{0, 0}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("all-zero weights = %v", err)
	}
}

// T042 / SC-003: a fixed seed, a thousand inputs. The shares always add up to
// the amount, and the same input gives the same shares twice.
func TestLargestRemainderSharesAlwaysAddUpProperty(t *testing.T) {
	random := rand.New(rand.NewPCG(34, 2))
	for i := range 1000 {
		total := random.Int64N(2*MaxAbsMinor+1) - MaxAbsMinor
		if i%4 == 0 {
			total = random.Int64N(20001) - 10000
		}
		weights := make([]int64, 2+random.IntN(19))
		for j := range weights {
			weights[j] = 1 + random.Int64N(1000)
		}
		first, err := SplitLargestRemainder(total, weights)
		if err != nil {
			t.Fatal(err)
		}
		second, _ := SplitLargestRemainder(total, weights)
		if !slices.Equal(first, second) {
			t.Fatalf("case %d: two runs differ: %v %v", i, first, second)
		}
		sum := int64(0)
		for j, share := range first {
			sum += share
			if (total >= 0) != (share >= 0) && share != 0 {
				t.Fatalf("case %d: share %d has the wrong sign: %v of %d", i, j, first, total)
			}
		}
		if sum != total {
			t.Fatalf("case %d: shares %v add up to %d, want %d", i, first, sum, total)
		}
	}
}

// Zero weights (a touch the method gives nothing) never receive a leftover
// unit, and the rest still add up.
func TestAZeroWeightShareStaysZeroProperty(t *testing.T) {
	random := rand.New(rand.NewPCG(34, 3))
	for i := range 1000 {
		total := random.Int64N(1_000_001)
		weights := make([]int64, 2+random.IntN(19))
		for j := range weights {
			if random.IntN(3) > 0 {
				weights[j] = 1 + random.Int64N(1000)
			}
		}
		weights[random.IntN(len(weights))] = 1 + random.Int64N(1000)
		shares, err := SplitLargestRemainder(total, weights)
		if err != nil {
			t.Fatal(err)
		}
		sum := int64(0)
		for j, share := range shares {
			sum += share
			if weights[j] == 0 && share != 0 {
				t.Fatalf("case %d: zero weight got %d", i, share)
			}
		}
		if sum != total {
			t.Fatalf("case %d: %v adds up to %d, want %d", i, shares, sum, total)
		}
	}
}

// A revision that says nothing about its split keeps the one it had: the
// stored shares turn back into the request that made them.
func TestStoredSharesBecomeTheSameRequestAgain(t *testing.T) {
	amount := int64(10000)
	weighted, err := ResolveAllocations(&amount, "CNY", weightShares("work", "w-a", "account", "a-1"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := ResolveAllocations(&amount, "CNY", allocationInputs(weighted, "CNY"))
	if err != nil || !slices.Equal(allocatedMinor(again), allocatedMinor(weighted)) {
		t.Fatalf("weights carried: %v %v", again, err)
	}
	larger := int64(10001)
	if again, err = ResolveAllocations(&larger, "CNY", allocationInputs(weighted, "CNY")); err != nil ||
		!slices.Equal(allocatedMinor(again), []int64{5001, 5000}) {
		t.Fatalf("weights recomputed on a new amount: %v %v", allocatedMinor(again), err)
	}
	x, y := "70.00", "30.00"
	byAmount, err := ResolveAllocations(&amount, "CNY", []AllocationInput{
		{TargetKind: "work", TargetID: "w-a", Amount: &x}, {TargetKind: "work", TargetID: "w-b", Amount: &y}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ResolveAllocations(&larger, "CNY", allocationInputs(byAmount, "CNY")); err == nil {
		t.Fatal("amounts that no longer add up to the new amount were carried silently")
	}
}
