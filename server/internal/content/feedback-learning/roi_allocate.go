package feedbacklearning

import (
	"cmp"
	"math/big"
	"slices"
	"strings"
	"time"
)

// Splitting one amount into integer shares (specs/034 PR 2: FR-030 to FR-033,
// D14-V14).
//
// One rule does all the splitting in this module, for shared costs and for
// multi-touch revenue alike: the largest-remainder rule in minor units. Every
// share first gets the floor of its exact portion; the minor units left over
// go one each to the shares with the largest fractional parts, ties to the
// share that comes first in the caller's order. The shares always add up to
// the amount exactly - never a unit more, never a unit less - and the same
// input always gives the same shares. Nothing here rounds: rounding is for
// display and conversion (FR-005), and a rounded split drifts off its total.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §1.2, §5.4

// AllocationTarget is what a share of a shared cost goes to (FR-031).
type AllocationTarget string

const (
	TargetWork          AllocationTarget = "work"
	TargetAccount       AllocationTarget = "account"
	TargetCampaignLabel AllocationTarget = "campaign_label"
	TargetPeriod        AllocationTarget = "period"
)

var AllocationTargets = []AllocationTarget{TargetWork, TargetAccount, TargetCampaignLabel, TargetPeriod}

// AllocationMethod is how the operator stated the split (FR-031).
type AllocationMethod string

const (
	AllocateByWeights AllocationMethod = "weights"
	AllocateByAmounts AllocationMethod = "amounts"
)

var AllocationMethods = []AllocationMethod{AllocateByWeights, AllocateByAmounts}

const (
	// MaxAllocations bounds the shares of one cost revision.
	MaxAllocations = 200
	// MaxWeight keeps a weight inside its integer column. Weights are
	// proportions a person types; a million to one is already absurd.
	MaxWeight = 1_000_000
)

// AllocationInput is one share as a request states it: a target, and either a
// positive integer weight or an amount string - the same one of the two on
// every share of a cost.
type AllocationInput struct {
	TargetKind string  `json:"target_kind"`
	TargetID   string  `json:"target_id"`
	Weight     *int64  `json:"weight"`
	Amount     *string `json:"amount"`
}

// CostAllocation is one stored share of one cost revision. AllocatedMinor is
// the server's figure: the largest-remainder share for 'weights', the
// operator's own amount for 'amounts'.
type CostAllocation struct {
	TargetKind     AllocationTarget `json:"target_kind"`
	TargetID       string           `json:"target_id"`
	Method         AllocationMethod `json:"method"`
	Weight         *int64           `json:"weight"`
	AllocatedMinor Minor            `json:"allocated_minor"`
	Allocated      string           `json:"allocated"`
}

// SplitLargestRemainder divides total into one integer share per weight,
// proportional to the weights (FR-032). The weights must be given in
// tie-break order: when two shares have the same fractional part, the one
// given first gets the leftover unit. A weight may be zero (that share is
// always zero) but none may be negative, and they may not all be zero. A
// negative total is split by its absolute value and every share takes the
// sign back.
func SplitLargestRemainder(total int64, weights []int64) ([]int64, error) {
	if len(weights) == 0 {
		return nil, ErrInvalid
	}
	sum := new(big.Int)
	for _, weight := range weights {
		if weight < 0 {
			return nil, ErrInvalid
		}
		sum.Add(sum, big.NewInt(weight))
	}
	if sum.Sign() == 0 {
		return nil, ErrInvalid
	}
	magnitude := new(big.Int).Abs(big.NewInt(total))
	floors := make([]*big.Int, len(weights))
	remainders := make([]*big.Int, len(weights))
	assigned := new(big.Int)
	for i, weight := range weights {
		product := new(big.Int).Mul(magnitude, big.NewInt(weight))
		floors[i], remainders[i] = new(big.Int).QuoRem(product, sum, new(big.Int))
		assigned.Add(assigned, floors[i])
	}
	// The leftover is fewer units than there are shares with a non-zero
	// remainder, so a zero-weight share never receives one.
	leftover := new(big.Int).Sub(magnitude, assigned).Int64()
	order := make([]int, len(weights))
	for i := range order {
		order[i] = i
	}
	// Stable: equal remainders keep the caller's tie-break order.
	slices.SortStableFunc(order, func(a, b int) int {
		return remainders[b].Cmp(remainders[a])
	})
	for k := range leftover {
		floors[order[k]].Add(floors[order[k]], big.NewInt(1))
	}
	shares := make([]int64, len(weights))
	for i, share := range floors {
		if total < 0 {
			share.Neg(share)
		}
		shares[i] = share.Int64()
	}
	return shares, nil
}

func compareAllocationKey(left, right CostAllocation) int {
	return cmp.Or(cmp.Compare(left.TargetKind, right.TargetKind), cmp.Compare(left.TargetID, right.TargetID))
}

func allocationError(index int, reason string) error {
	return FieldError{Field: "allocations", Row: index + 1, Reason: reason}
}

// checkPeriod accepts exactly YYYY-MM.
func checkPeriod(value string) bool {
	parsed, err := time.Parse("2006-01", value)
	return err == nil && parsed.Format("2006-01") == value
}

// ResolveAllocations checks a cost revision's shares against its amount and
// works out every share in minor units (FR-030 to FR-032). No shares at all
// is a cost that is not shared, and answers nil. Every refusal names
// "allocations", and which share when it is about one.
//
// The shares come back ordered by target key (kind, then id), which is also
// the tie-break order of the weights method.
func ResolveAllocations(amount *int64, currency string, inputs []AllocationInput) ([]CostAllocation, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) > MaxAllocations {
		return nil, FieldError{Field: "allocations", Reason: "too many shares"}
	}
	allocations := make([]CostAllocation, 0, len(inputs))
	seen := map[string]bool{}
	byWeights, byAmounts := 0, 0
	for i, in := range inputs {
		if !oneOf(in.TargetKind, AllocationTargets) {
			return nil, allocationError(i, "target_kind is not an allowed value")
		}
		id := strings.TrimSpace(in.TargetID)
		if id == "" || id != in.TargetID {
			return nil, allocationError(i, "target_id is missing or has surrounding spaces")
		}
		if AllocationTarget(in.TargetKind) == TargetPeriod && !checkPeriod(id) {
			return nil, allocationError(i, "a period is YYYY-MM")
		}
		if checkRunes("allocations", id, MaxShortRunes) != nil {
			return nil, allocationError(i, "target_id is too long")
		}
		key := in.TargetKind + "\x00" + id
		if seen[key] {
			return nil, allocationError(i, "the same target twice")
		}
		seen[key] = true
		allocation := CostAllocation{TargetKind: AllocationTarget(in.TargetKind), TargetID: id}
		switch {
		case in.Weight != nil && in.Amount == nil:
			byWeights++
			if *in.Weight <= 0 || *in.Weight > MaxWeight {
				return nil, allocationError(i, "a weight is a positive integer")
			}
			weight := *in.Weight
			allocation.Method, allocation.Weight = AllocateByWeights, &weight
		case in.Amount != nil && in.Weight == nil:
			byAmounts++
			share, err := ParseAmount("allocations", *in.Amount, currency)
			if err != nil {
				return nil, allocationError(i, "the amount is not a decimal string in the cost's currency")
			}
			allocation.Method, allocation.AllocatedMinor = AllocateByAmounts, Minor(share)
		default:
			return nil, allocationError(i, "give a weight or an amount, not both and not neither")
		}
		allocations = append(allocations, allocation)
	}
	if byWeights > 0 && byAmounts > 0 {
		return nil, FieldError{Field: "allocations", Reason: "every share uses the same method"}
	}
	if amount == nil {
		// A labor cost without a rate has nothing to split yet.
		return nil, FieldError{Field: "allocations", Reason: "the cost has no computable amount to split"}
	}
	slices.SortFunc(allocations, compareAllocationKey)
	if byAmounts > 0 {
		sum := new(big.Int)
		for _, allocation := range allocations {
			sum.Add(sum, big.NewInt(int64(allocation.AllocatedMinor)))
		}
		if sum.Cmp(big.NewInt(*amount)) != 0 {
			return nil, FieldError{Field: "allocations", Reason: "the shares do not add up to the cost's amount"}
		}
	} else {
		weights := make([]int64, len(allocations))
		for i, allocation := range allocations {
			weights[i] = *allocation.Weight
		}
		shares, err := SplitLargestRemainder(*amount, weights)
		if err != nil {
			return nil, FieldError{Field: "allocations", Reason: "the weights cannot all be zero"}
		}
		for i := range allocations {
			allocations[i].AllocatedMinor = Minor(shares[i])
		}
	}
	for i := range allocations {
		allocations[i].Allocated = FormatAmount(int64(allocations[i].AllocatedMinor), currency)
	}
	return allocations, nil
}

// allocationInputs turns a revision's stored shares back into the request
// that would produce them, so a revision that says nothing about its split
// keeps the one it had. Weights recompute against the new amount; amounts
// must still add up to it.
func allocationInputs(allocations []CostAllocation, currency string) []AllocationInput {
	inputs := make([]AllocationInput, 0, len(allocations))
	for _, allocation := range allocations {
		in := AllocationInput{TargetKind: string(allocation.TargetKind), TargetID: allocation.TargetID}
		if allocation.Method == AllocateByWeights && allocation.Weight != nil {
			weight := *allocation.Weight
			in.Weight = &weight
		} else {
			text := FormatAmount(int64(allocation.AllocatedMinor), currency)
			in.Amount = &text
		}
		inputs = append(inputs, in)
	}
	return inputs
}
