package feedbacklearning

import (
	"encoding/json"
	"errors"
	"math/big"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
)

// specs/034 PR 2: the calculator, T043 and T048 to T052 (SC-001, SC-002,
// SC-004, SC-006; D14-V12, D14-V13, D14-V14, D14-V16). Every test here is a
// pure function call: no database, no clock.

var inWindow = time.Date(2026, 9, 10, 2, 0, 0, 0, time.UTC)

func minorOf(value int64) *Minor {
	minor := Minor(value)
	return &minor
}

func sampleParams() ReportParams {
	return ReportParams{
		Window:            ReportWindow{Start: "2026-09-01", End: "2026-09-30", Timezone: "Asia/Shanghai"},
		ReportCurrency:    "CNY",
		AttributionMethod: "even_split",
		Conversion:        ConversionStages{FromStage: "咨询", ToStage: "预约"},
		BookingStage:      "预约",
		GeneratedAt:       "2026-10-01T00:00:00Z",
	}
}

func sampleCost(id string, minor int64) CostRevision {
	return CostRevision{
		CostID: id, Revision: 1, Category: "投放", Pricing: PricingAmount,
		AmountMinor: minorOf(minor), Currency: "CNY", IncurredAt: inWindow,
	}
}

func sampleDeal(id, leadID string, amount int64) DealRevision {
	return DealRevision{
		DealID: id, Revision: 1, LeadID: leadID, AmountMinor: Minor(amount), Currency: "CNY",
		ClosedAt: inWindow, GrossBasis: GrossNone,
	}
}

func statedDeal(id, leadID string, amount, gross int64) DealRevision {
	deal := sampleDeal(id, leadID, amount)
	deal.GrossBasis, deal.GrossProfitMinor = GrossStated, minorOf(gross)
	return deal
}

func sampleTouch(id, leadID, workID, accountID string, at time.Time) TouchRevision {
	return TouchRevision{
		TouchID: id, Revision: 1, LeadID: leadID, EvidenceType: EvidencePlatformLinkedContent,
		Platform: "douyin", WorkID: workID, AccountID: accountID, Role: RoleOther, OccurredAt: at,
	}
}

func judged(dealID string, judgement Judgement, touchIDs ...string) AttributionRevision {
	return AttributionRevision{DealID: dealID, Revision: 1, Judgement: judgement, TouchIDs: touchIDs, Weights: []int64{}}
}

func calculate(t *testing.T, input ReportInput) Result {
	t.Helper()
	result, err := CalculateROI(input)
	if err != nil {
		t.Fatalf("CalculateROI: %v", err)
	}
	return result
}

func wantOK(t *testing.T, result Result, metric MetricID, display string) MetricResult {
	t.Helper()
	got := result.Metrics[metric]
	if got.Status != StatusOK || got.Display != display {
		t.Fatalf("%s = %+v, want ok %q", metric, got, display)
	}
	return got
}

func wantReason(t *testing.T, result Result, metric MetricID, reason ReasonCode) MetricResult {
	t.Helper()
	got := result.Metrics[metric]
	if got.Status != StatusNotComputable || got.Reason != reason || got.Value != "" || got.Display != "" {
		t.Fatalf("%s = %+v, want not_computable %s with no number", metric, got, reason)
	}
	return got
}

func hasRecord(metric MetricResult, kind, id string) bool {
	return slices.ContainsFunc(metric.Records, func(record RecordRef) bool {
		return record.Kind == kind && record.ID == id
	})
}

// oneTouchDeal is a deal judged "confirmed" on one touch of its lead.
func oneTouchDeal(deal DealRevision) ([]DealRevision, []TouchRevision, []AttributionRevision) {
	touch := sampleTouch("t-"+deal.DealID, deal.LeadID, "w-1", "a-1", inWindow)
	return []DealRevision{deal}, []TouchRevision{touch},
		[]AttributionRevision{judged(deal.DealID, JudgementConfirmed, touch.TouchID)}
}

// TestFixedSamples is contract §5.4, each sample under its contract name.
// D14-V12/roi and D14-V12/revenue are the D14-V12 acceptance cases (SC-001).
func TestFixedSamples(t *testing.T) {
	t.Run("D14-V12/roi", func(t *testing.T) {
		deals, touches, judgements := oneTouchDeal(statedDeal("d-1", "l-1", 1000000, 300000))
		result := calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 100000)},
			Deals: deals, Touches: touches, Attributions: judgements,
		})
		roi := wantOK(t, result, MetricBusinessROI, "200.00%")
		if roi.Value != "2" || roi.Numerator != "200000" || roi.Denominator != "100000" || roi.Formula != "business_roi/1" {
			t.Fatalf("business_roi = %+v", roi)
		}
		if !hasRecord(roi, "cost", "c-1") || !hasRecord(roi, "deal", "d-1") {
			t.Fatalf("business_roi records = %+v", roi.Records)
		}
		if result.CalcVersion != "roi-calc/1" {
			t.Fatalf("calc_version = %q", result.CalcVersion)
		}
	})

	t.Run("D14-V12/revenue", func(t *testing.T) {
		deals, touches, judgements := oneTouchDeal(sampleDeal("d-1", "l-1", 1000000))
		result := calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 100000)},
			Deals: deals, Touches: touches, Attributions: judgements,
		})
		revenue := wantOK(t, result, MetricRevenueToSpend, "10.00 倍")
		if revenue.Value != "10" || revenue.Formula != "revenue_to_spend/1" || revenue.Unit != UnitTimes {
			t.Fatalf("revenue_to_spend = %+v", revenue)
		}
		wantReason(t, result, MetricBusinessROI, ReasonMissingGrossProfit)
		// A revenue ratio, labelled as one: nothing about it says ROI or
		// profit (FR-044). Ad ROAS is its own metric.
		for _, label := range []string{string(MetricRevenueToSpend), revenue.Formula} {
			lower := strings.ToLower(label)
			if strings.Contains(lower, "roi") || strings.Contains(lower, "profit") || strings.Contains(label, "利润") {
				t.Errorf("revenue_to_spend is labelled %q", label)
			}
		}
		if MetricAdROAS == MetricRevenueToSpend || MetricAdROAS == MetricBusinessROI {
			t.Fatal("ad ROAS is not a separate metric")
		}
	})

	t.Run("negative", func(t *testing.T) {
		deals, touches, judgements := oneTouchDeal(statedDeal("d-1", "l-1", 90000, 50000))
		result := calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 100000)},
			Deals: deals, Touches: touches, Attributions: judgements,
		})
		if roi := wantOK(t, result, MetricBusinessROI, "−50.00%"); roi.Value != "-1/2" {
			t.Fatalf("business_roi value = %q", roi.Value)
		}
	})

	t.Run("zero-spend", func(t *testing.T) {
		deals, touches, judgements := oneTouchDeal(statedDeal("d-1", "l-1", 1000000, 300000))
		// No cost in the window at all: no data.
		none := calculate(t, ReportInput{Params: sampleParams(), Deals: deals, Touches: touches, Attributions: judgements})
		for _, metric := range []MetricID{MetricBusinessROI, MetricRevenueToSpend, MetricCostPerDeal, MetricSpendTotal} {
			wantReason(t, none, metric, ReasonNoData)
		}
		// Costs in the window that add up to zero: a zero denominator.
		zero := calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 0)},
			Deals: deals, Touches: touches, Attributions: judgements,
		})
		wantOK(t, zero, MetricSpendTotal, "0.00")
		for _, metric := range []MetricID{MetricBusinessROI, MetricRevenueToSpend} {
			wantReason(t, zero, metric, ReasonZeroDenominator)
		}
		wantReason(t, zero, MetricAdROAS, ReasonNoData)
	})

	t.Run("unconverted", func(t *testing.T) {
		cost := sampleCost("c-usd", 10000)
		cost.Currency = "USD"
		result := calculate(t, ReportInput{Params: sampleParams(), Costs: []CostRevision{cost}})
		spend := wantReason(t, result, MetricSpendTotal, ReasonCurrencyUnconverted)
		if !hasRecord(spend, "cost", "c-usd") {
			t.Fatalf("the unconverted cost is not listed: %+v", spend.Records)
		}
		wantReason(t, result, MetricBusinessROI, ReasonNoData)
	})

	t.Run("converted", func(t *testing.T) {
		cost := sampleCost("c-usd", 10000)
		cost.Currency = "USD"
		params := sampleParams()
		params.Rates = []ExchangeRate{{From: "USD", To: "CNY", Rate: "7.1234", Note: "9 月 30 日中行牌价"}}
		result := calculate(t, ReportInput{Params: params, Costs: []CostRevision{cost}})
		spend := wantOK(t, result, MetricSpendTotal, "712.34")
		if spend.Value != "71234" || *spend.Records[0].ConvertedMinor != "71234" || *spend.Records[0].AmountMinor != "10000" {
			t.Fatalf("spend_total = %+v", spend)
		}
	})

	t.Run("split-10001", func(t *testing.T) {
		deal := sampleDeal("d-1", "l-1", 10001)
		first := sampleTouch("t-b", "l-1", "w-b", "", inWindow)
		second := sampleTouch("t-a", "l-1", "w-a", "", inWindow.Add(time.Hour))
		result := calculate(t, ReportInput{
			Params: sampleParams(), Deals: []DealRevision{deal}, Touches: []TouchRevision{first, second},
			Attributions: []AttributionRevision{judged("d-1", JudgementMulti, "t-a", "t-b")},
		})
		// The earlier touch wins the tie, whatever its id.
		byWork := workShares(result)
		if byWork["w-b"] != "5001" || byWork["w-a"] != "5000" {
			t.Fatalf("shares %v, want w-b 5001 and w-a 5000", byWork)
		}
	})

	t.Run("alloc-10000", func(t *testing.T) {
		amount := int64(10000)
		allocations, err := ResolveAllocations(&amount, "CNY", weightShares("work", "w-3", "work", "w-1", "work", "w-2"))
		if err != nil {
			t.Fatal(err)
		}
		if got := allocatedMinor(allocations); !slices.Equal(got, []int64{3334, 3333, 3333}) || allocations[0].TargetID != "w-1" {
			t.Fatalf("shares %v (%s first)", got, allocations[0].TargetID)
		}
		// And the report counts the cost once, at its original total.
		cost := sampleCost("c-1", amount)
		cost.Allocations = allocations
		result := calculate(t, ReportInput{Params: sampleParams(), Costs: []CostRevision{cost}})
		wantOK(t, result, MetricSpendTotal, "100.00")
		// Scoped to one work, only that work's share.
		params := sampleParams()
		params.Scope.WorkIDs = []string{"w-1"}
		wantOK(t, calculate(t, ReportInput{Params: params, Costs: []CostRevision{cost}}), MetricSpendTotal, "33.34")
	})

	t.Run("one-booking-two-works", func(t *testing.T) {
		result := calculate(t, oneBookingTwoWorks())
		wantOK(t, result, MetricBookings, "1")
		wantOK(t, result, MetricDeals, "1")
	})
}

func workShares(result Result) map[string]string {
	shares := map[string]string{}
	for _, row := range result.Breakdown.ByWork {
		shares[row.ID] = row.AttributedNetRevenue.Value
	}
	return shares
}

// oneBookingTwoWorks is one lead that reached 预约 (after 咨询), with two
// touches on two works, and one deal judged multi-touch on both.
func oneBookingTwoWorks() ReportInput {
	firstSeen := time.Date(2026, 9, 5, 2, 0, 0, 0, time.UTC)
	return ReportInput{
		Params: sampleParams(),
		Leads: []LeadRevision{
			{LeadID: "l-1", Revision: 1, Stage: "咨询", Qualified: true, FirstSeenAt: firstSeen},
			{LeadID: "l-1", Revision: 2, Stage: "预约", Qualified: true, FirstSeenAt: firstSeen},
		},
		Touches: []TouchRevision{
			sampleTouch("t-1", "l-1", "w-1", "a-1", inWindow),
			sampleTouch("t-2", "l-1", "w-2", "a-1", inWindow.Add(time.Hour)),
		},
		Deals:        []DealRevision{sampleDeal("d-1", "l-1", 1000000)},
		Attributions: []AttributionRevision{judged("d-1", JudgementMulti, "t-1", "t-2")},
		Costs:        []CostRevision{sampleCost("c-1", 100000)},
	}
}

// T049 / SC-002 (D14-V13): each way a figure can fail to exist, and the
// window rules.
func TestNotComputableCases(t *testing.T) {
	t.Run("missing gross profit keeps the revenue metrics", func(t *testing.T) {
		deals, touches, judgements := oneTouchDeal(sampleDeal("d-1", "l-1", 1000000))
		result := calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 100000)},
			Deals: deals, Touches: touches, Attributions: judgements,
		})
		wantReason(t, result, MetricBusinessROI, ReasonMissingGrossProfit)
		wantReason(t, result, MetricAttributedGrossProfit, ReasonMissingGrossProfit)
		wantOK(t, result, MetricRevenueToSpend, "10.00 倍")
		wantOK(t, result, MetricAttributedNetRevenue, "10000.00")
	})

	t.Run("a refund without a gross reduction", func(t *testing.T) {
		deals, touches, judgements := oneTouchDeal(statedDeal("d-1", "l-1", 1000000, 300000))
		refund := AdjustmentRevision{
			AdjustmentID: "r-1", Revision: 1, DealID: "d-1", Kind: AdjustmentRefund,
			RevenueDeltaMinor: 100000, Currency: "CNY", OccurredAt: inWindow,
		}
		result := calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 100000)},
			Deals: deals, Touches: touches, Attributions: judgements, Adjustments: []AdjustmentRevision{refund},
		})
		wantReason(t, result, MetricBusinessROI, ReasonRefundWithoutGrossDelta)
		wantOK(t, result, MetricRevenueToSpend, "9.00 倍")
		// With the change given - signed, "毛利冲减 300" is -300 - it is
		// added, not guessed (FR-025).
		refund.GrossDeltaMinor = minorOf(-30000)
		result = calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 100000)},
			Deals: deals, Touches: touches, Attributions: judgements, Adjustments: []AdjustmentRevision{refund},
		})
		wantOK(t, result, MetricBusinessROI, "170.00%")
	})

	t.Run("a cogs deal nets its cost of goods", func(t *testing.T) {
		deal := sampleDeal("d-1", "l-1", 1000000)
		deal.GrossBasis, deal.COGSMinor = GrossCOGS, minorOf(700000)
		deals, touches, judgements := oneTouchDeal(deal)
		result := calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 100000)},
			Deals: deals, Touches: touches, Attributions: judgements,
		})
		wantOK(t, result, MetricBusinessROI, "200.00%")
	})

	t.Run("an unconverted deal is listed, never dropped", func(t *testing.T) {
		deal := sampleDeal("d-usd", "l-1", 50000)
		deal.Currency = "USD"
		deals, touches, judgements := oneTouchDeal(deal)
		result := calculate(t, ReportInput{
			Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 100000)},
			Deals: append(deals, sampleDeal("d-cny", "", 1000)), Touches: touches, Attributions: judgements,
		})
		for _, metric := range []MetricID{MetricNetRevenue, MetricAttributedNetRevenue, MetricRevenueToSpend} {
			if got := wantReason(t, result, metric, ReasonCurrencyUnconverted); !hasRecord(got, "deal", "d-usd") {
				t.Errorf("%s does not list the unconverted deal: %+v", metric, got.Records)
			}
		}
	})

	t.Run("the window and late refunds", func(t *testing.T) {
		shanghai, _ := time.LoadLocation("Asia/Shanghai")
		lastMinute := time.Date(2026, 9, 30, 23, 59, 0, 0, shanghai)
		nextDay := time.Date(2026, 10, 1, 0, 0, 0, 0, shanghai)
		inside, outside := sampleDeal("d-in", "", 100000), sampleDeal("d-out", "", 900000)
		inside.ClosedAt, outside.ClosedAt = lastMinute.UTC(), nextDay.UTC()
		lateRefund := AdjustmentRevision{
			AdjustmentID: "r-late", Revision: 1, DealID: "d-in", Kind: AdjustmentRefund,
			RevenueDeltaMinor: 10000, Currency: "CNY", OccurredAt: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		}
		input := ReportInput{Params: sampleParams(), Deals: []DealRevision{inside, outside},
			Adjustments: []AdjustmentRevision{lateRefund}}
		// Generated before the refund happened: not counted yet.
		input.Params.GeneratedAt = "2026-10-02T00:00:00Z"
		result := calculate(t, input)
		wantOK(t, result, MetricDeals, "1")
		if net := wantOK(t, result, MetricNetRevenue, "1000.00"); hasRecord(net, "deal", "d-out") {
			t.Fatal("a deal outside the window was counted")
		}
		// Generated after it: the refund follows its deal into the window.
		input.Params.GeneratedAt = "2026-10-06T00:00:00Z"
		result = calculate(t, input)
		if net := wantOK(t, result, MetricNetRevenue, "900.00"); !hasRecord(net, "adjustment", "r-late") {
			t.Fatalf("net_revenue records = %+v", net.Records)
		}
	})

	t.Run("conversion needs both stages", func(t *testing.T) {
		params := sampleParams()
		params.Conversion.ToStage = ""
		input := oneBookingTwoWorks()
		input.Params = params
		wantReason(t, calculate(t, input), MetricConversionRate, ReasonMissingDenominator)
		input.Params = sampleParams()
		wantOK(t, calculate(t, input), MetricConversionRate, "100.00%")
		input.Params.Conversion.FromStage = "到店"
		wantReason(t, calculate(t, input), MetricConversionRate, ReasonZeroDenominator)
	})
}

// T052 / FR-051: a labor cost without a rate and a USD cost without a rate.
// The earlier reason in the table wins, and both records are listed.
func TestTheEarliestReasonWinsAndEveryRecordIsListed(t *testing.T) {
	labor := CostRevision{
		CostID: "c-labor", Revision: 1, Category: "人工", Pricing: PricingLaborTime,
		Currency: "CNY", IncurredAt: inWindow, AmountStatus: AmountLaborRateMissing,
	}
	usd := sampleCost("c-usd", 10000)
	usd.Currency = "USD"
	result := calculate(t, ReportInput{Params: sampleParams(), Costs: []CostRevision{labor, usd, sampleCost("c-ok", 5000)}})
	spend := wantReason(t, result, MetricSpendTotal, ReasonCurrencyUnconverted)
	for _, id := range []string{"c-labor", "c-usd", "c-ok"} {
		if !hasRecord(spend, "cost", id) {
			t.Errorf("spend_total does not list %s: %+v", id, spend.Records)
		}
	}
	wantReason(t, calculate(t, ReportInput{Params: sampleParams(), Costs: []CostRevision{labor}}),
		MetricSpendTotal, ReasonLaborRateMissing)
	// Spend as numerator or denominator carries its reason; business ROI
	// also has no deals at all, and no_data comes first.
	for _, metric := range []MetricID{MetricCostPerDeal, MetricCostPerQualifiedLead} {
		wantReason(t, result, metric, ReasonCurrencyUnconverted)
	}
	for _, metric := range []MetricID{MetricRevenueToSpend, MetricBusinessROI} {
		wantReason(t, result, metric, ReasonNoData)
	}
	withDeal := calculate(t, ReportInput{Params: sampleParams(), Costs: []CostRevision{labor, usd},
		Deals: []DealRevision{sampleDeal("d-1", "", 100)}})
	for _, metric := range []MetricID{MetricCostPerDeal, MetricCostPerQualifiedLead} {
		wantReason(t, withDeal, metric, ReasonCurrencyUnconverted)
	}
}

// T050 / SC-006 (D14-V16): the brand counts leads and deals, not touches.
func TestTheBrandCountsLeadsAndDealsOnce(t *testing.T) {
	result := calculate(t, oneBookingTwoWorks())
	wantOK(t, result, MetricBookings, "1")
	wantOK(t, result, MetricDeals, "1")
	wantOK(t, result, MetricQualifiedLeads, "1")
	if len(result.Breakdown.ByWork) != 2 {
		t.Fatalf("by_work = %+v", result.Breakdown.ByWork)
	}
	for _, row := range result.Breakdown.ByWork {
		if row.DealsTouched != 1 {
			t.Errorf("work %s touched %d deals, want 1", row.ID, row.DealsTouched)
		}
	}
	if shares := workShares(result); shares["w-1"] != "500000" || shares["w-2"] != "500000" {
		t.Fatalf("work shares %v", shares)
	}
	// Both touches are the same account's: one deal, whole amount, once.
	if len(result.Breakdown.ByAccount) != 1 || result.Breakdown.ByAccount[0].DealsTouched != 1 ||
		result.Breakdown.ByAccount[0].AttributedNetRevenue.Value != "1000000" {
		t.Fatalf("by_account = %+v", result.Breakdown.ByAccount)
	}
	if !slices.Contains(result.Rules, "同一笔成交可能出现在多个作品下，不可相加。") {
		t.Error("the rules do not say work rows cannot be added up")
	}
}

func TestAnUnknownSourceIsNeverAttributed(t *testing.T) {
	input := oneBookingTwoWorks()
	input.Deals = append(input.Deals, sampleDeal("d-unknown", "l-2", 400000), sampleDeal("d-unjudged", "l-2", 100000))
	// The lead of both deals has a touch on a work. Neither deal was
	// judged to come from it: one is "unknown", one was never judged.
	input.Touches = append(input.Touches, sampleTouch("t-9", "l-2", "w-9", "a-9", inWindow))
	input.Attributions = append(input.Attributions, judged("d-unknown", JudgementUnknown))
	result := calculate(t, input)
	for _, row := range append(result.Breakdown.ByWork, result.Breakdown.ByAccount...) {
		if row.ID == "w-9" || row.ID == "a-9" {
			t.Fatalf("an unattributed deal was given to %s %s", row.Kind, row.ID)
		}
	}
	unattributed := result.Breakdown.Unattributed
	if unattributed.DealsTouched != 2 || unattributed.AttributedNetRevenue.Value != "500000" {
		t.Fatalf("unattributed = %+v", unattributed)
	}
	// Still in the brand total and in the coverage denominator.
	wantOK(t, result, MetricDeals, "3")
	wantOK(t, result, MetricNetRevenue, "15000.00")
	wantOK(t, result, MetricAttributionCoverageCount, "33.33%")
	wantOK(t, result, MetricAttributionCoverageAmount, "66.67%")
	wantOK(t, result, MetricAttributedNetRevenue, "10000.00")
}

func TestATouchWithoutAWorkStaysAtAccountOrBrandLevel(t *testing.T) {
	deal := sampleDeal("d-1", "l-1", 30000)
	accountOnly := sampleTouch("t-acct", "l-1", "", "a-1", inWindow)
	accountOnly.EvidenceType = EvidenceAccountOnly
	statement := sampleTouch("t-said", "l-1", "", "", inWindow.Add(time.Hour))
	statement.EvidenceType, statement.Platform = EvidenceCustomerStatement, ""
	withWork := sampleTouch("t-work", "l-1", "w-1", "a-1", inWindow.Add(2*time.Hour))
	result := calculate(t, ReportInput{
		Params: sampleParams(), Deals: []DealRevision{deal},
		Touches:      []TouchRevision{accountOnly, statement, withWork},
		Attributions: []AttributionRevision{judged("d-1", JudgementMulti, "t-acct", "t-said", "t-work")},
	})
	breakdown := result.Breakdown
	if breakdown.AccountLevelUnknownWork.AttributedNetRevenue.Value != "10000" ||
		breakdown.BrandLevelUnknownAccount.AttributedNetRevenue.Value != "10000" {
		t.Fatalf("account level %+v, brand level %+v", breakdown.AccountLevelUnknownWork, breakdown.BrandLevelUnknownAccount)
	}
	if len(breakdown.ByWork) != 1 || breakdown.ByWork[0].AttributedNetRevenue.Value != "10000" {
		t.Fatalf("by_work = %+v; a share without a work went to a work", breakdown.ByWork)
	}
}

// T043 / SC-004: the four attribution methods on one deal of 10001 over
// three touches, and the shares always add up to the deal.
func TestEachAttributionMethodSharesTheWholeDeal(t *testing.T) {
	early := sampleTouch("t-1", "l-1", "w-1", "", inWindow)
	early.Role = RoleFirstTouch
	middle := sampleTouch("t-2", "l-1", "w-2", "", inWindow.Add(time.Hour))
	middle.Role = RolePreBooking
	late := sampleTouch("t-3", "l-1", "w-3", "", inWindow.Add(2*time.Hour))
	judgement := judged("d-1", JudgementMulti, "t-3", "t-2", "t-1")
	run := func(method string, judgement AttributionRevision) Result {
		params := sampleParams()
		params.AttributionMethod = method
		return calculate(t, ReportInput{
			Params: params, Deals: []DealRevision{sampleDeal("d-1", "l-1", 10001)},
			Touches: []TouchRevision{late, early, middle}, Attributions: []AttributionRevision{judgement},
		})
	}
	cases := []struct {
		method   string
		weights  []int64
		want     map[string]string
		fallback bool
	}{
		{"even_split", nil, map[string]string{"w-1": "3334", "w-2": "3334", "w-3": "3333"}, false},
		// The first touch in the first_touch role.
		{"first_touch", nil, map[string]string{"w-1": "10001", "w-2": "0", "w-3": "0"}, false},
		// The last touch in the pre_booking role, even though t-3 is later.
		{"last_touch", nil, map[string]string{"w-1": "0", "w-2": "10001", "w-3": "0"}, false},
		// Weights in touch_ids order: t-3 gets 2, t-2 gets 1, t-1 gets 1.
		{"judgement_weights", []int64{2, 1, 1}, map[string]string{"w-1": "2500", "w-2": "2500", "w-3": "5001"}, false},
		{"judgement_weights", nil, map[string]string{"w-1": "3334", "w-2": "3334", "w-3": "3333"}, true},
	}
	for _, tc := range cases {
		judgement.Weights = tc.weights
		result := run(tc.method, judgement)
		got := workShares(result)
		sum := new(big.Int)
		for work, want := range tc.want {
			if got[work] != want {
				t.Errorf("%s %v: %s got %q, want %q", tc.method, tc.weights, work, got[work], want)
			}
			value, _ := new(big.Int).SetString(got[work], 10)
			sum.Add(sum, value)
		}
		if sum.Int64() != 10001 {
			t.Errorf("%s: shares add up to %s, want 10001", tc.method, sum)
		}
		if fell := slices.Contains(result.Breakdown.EvenSplitFallback, "d-1"); fell != tc.fallback {
			t.Errorf("%s %v: even-split fallback marked %v, want %v", tc.method, tc.weights, fell, tc.fallback)
		}
	}
	// With no first_touch or pre_booking role, first and last fall back to
	// the earliest and the latest touch.
	early.Role, middle.Role = RoleOther, RoleOther
	if got := workShares(run("first_touch", judgement)); got["w-1"] != "10001" {
		t.Errorf("first_touch without the role: %v", got)
	}
	if got := workShares(run("last_touch", judgement)); got["w-3"] != "10001" {
		t.Errorf("last_touch without the role: %v", got)
	}
}

// T043 property: for any deal and any accepted touches, what the works get
// adds up to the deal, never more.
func TestMultiTouchSharesAddUpToTheDealProperty(t *testing.T) {
	random := rand.New(rand.NewPCG(34, 43))
	methods := []string{"even_split", "first_touch", "last_touch", "judgement_weights"}
	for i := range 300 {
		amount := random.Int64N(100_000_000)
		touches := []TouchRevision{}
		ids := []string{}
		weights := []int64{}
		for j := range 1 + random.IntN(8) {
			id := "t-" + string(rune('a'+j))
			touch := sampleTouch(id, "l-1", "w-"+id, "", inWindow.Add(time.Duration(random.IntN(5))*time.Minute))
			touch.Role = TouchRoles[random.IntN(len(TouchRoles))]
			touches = append(touches, touch)
			ids = append(ids, id)
			weights = append(weights, 1+random.Int64N(50))
		}
		judgement := judged("d-1", JudgementMulti, ids...)
		judgement.Weights = weights
		params := sampleParams()
		params.AttributionMethod = methods[i%len(methods)]
		result := calculate(t, ReportInput{
			Params: params, Deals: []DealRevision{sampleDeal("d-1", "l-1", amount)},
			Touches: touches, Attributions: []AttributionRevision{judgement},
		})
		sum := new(big.Int)
		for _, row := range result.Breakdown.ByWork {
			value, _ := new(big.Int).SetString(row.AttributedNetRevenue.Value, 10)
			sum.Add(sum, value)
		}
		if sum.Int64() != amount {
			t.Fatalf("case %d (%s): works got %s of %d", i, params.AttributionMethod, sum, amount)
		}
	}
}

// T051 / FR-040: the same input twice gives byte-identical JSON, and so does
// the same input in another order.
func TestTheSameInputGivesTheSameBytes(t *testing.T) {
	input := oneBookingTwoWorks()
	usd := sampleCost("c-usd", 12345)
	usd.Currency = "USD"
	usd.AdSpend = true
	input.Costs = append(input.Costs, usd, sampleCost("c-2", 777))
	input.Params.Rates = []ExchangeRate{{From: "USD", To: "CNY", Rate: "7.1234"}}
	input.Deals = append(input.Deals, statedDeal("d-2", "l-1", 55555, 22222), sampleDeal("d-3", "", 1))
	input.Attributions = append(input.Attributions, judged("d-2", JudgementConfirmed, "t-2"))
	input.Adjustments = []AdjustmentRevision{{AdjustmentID: "r-1", Revision: 1, DealID: "d-2",
		Kind: AdjustmentRefund, RevenueDeltaMinor: 5, GrossDeltaMinor: minorOf(-2), Currency: "CNY", OccurredAt: inWindow}}
	encode := func(input ReportInput) string {
		result := calculate(t, input)
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	want := encode(input)
	if again := encode(input); again != want {
		t.Fatalf("two runs differ:\n%s\n%s", want, again)
	}
	random := rand.New(rand.NewPCG(51, 51))
	for range 20 {
		shuffled := input
		shuffled.Costs = slices.Clone(input.Costs)
		shuffled.Leads = slices.Clone(input.Leads)
		shuffled.Touches = slices.Clone(input.Touches)
		shuffled.Deals = slices.Clone(input.Deals)
		shuffled.Attributions = slices.Clone(input.Attributions)
		for _, shuffle := range []func(){
			func() {
				random.Shuffle(len(shuffled.Costs), func(i, j int) { shuffled.Costs[i], shuffled.Costs[j] = shuffled.Costs[j], shuffled.Costs[i] })
			},
			func() {
				random.Shuffle(len(shuffled.Leads), func(i, j int) { shuffled.Leads[i], shuffled.Leads[j] = shuffled.Leads[j], shuffled.Leads[i] })
			},
			func() {
				random.Shuffle(len(shuffled.Touches), func(i, j int) { shuffled.Touches[i], shuffled.Touches[j] = shuffled.Touches[j], shuffled.Touches[i] })
			},
			func() {
				random.Shuffle(len(shuffled.Deals), func(i, j int) { shuffled.Deals[i], shuffled.Deals[j] = shuffled.Deals[j], shuffled.Deals[i] })
			},
			func() {
				random.Shuffle(len(shuffled.Attributions), func(i, j int) {
					shuffled.Attributions[i], shuffled.Attributions[j] = shuffled.Attributions[j], shuffled.Attributions[i]
				})
			},
		} {
			shuffle()
		}
		if got := encode(shuffled); got != want {
			t.Fatalf("a reordered input gave another result:\n%s\n%s", want, got)
		}
	}
	// A JSON round trip of the input - what a stored report will hold -
	// gives the same result too.
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ReportInput
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the input does not read back: %v", err)
	}
	if got := encode(decoded); got != want {
		t.Fatalf("a round-tripped input gave another result:\n%s\n%s", want, got)
	}
}

// FR-005 / ruling Q2=A: half away from zero, two places, U+2212 for minus.
func TestDisplayRoundsHalfAwayFromZero(t *testing.T) {
	for _, tc := range []struct {
		value *big.Rat
		want  string
	}{
		{big.NewRat(1, 8), "0.13"},
		{big.NewRat(-1, 8), "−0.13"},
		{big.NewRat(3, 8), "0.38"},
		{big.NewRat(1, 3), "0.33"},
		{big.NewRat(2, 3), "0.67"},
		{big.NewRat(-1, 1000), "0.00"},
		{big.NewRat(1, 200), "0.01"},
		{big.NewRat(-1, 200), "−0.01"},
		{big.NewRat(2, 1), "2.00"},
	} {
		if got := FormatRat(tc.value, 2); got != tc.want {
			t.Errorf("FormatRat(%s) = %q, want %q", tc.value.RatString(), got, tc.want)
		}
	}
	// A ratio of exactly 0.125% as a percent: half a hundredth.
	deals, touches, judgements := oneTouchDeal(statedDeal("d-1", "l-1", 1000000, 801000))
	result := calculate(t, ReportInput{
		Params: sampleParams(), Costs: []CostRevision{sampleCost("c-1", 800000)},
		Deals: deals, Touches: touches, Attributions: judgements,
	})
	wantOK(t, result, MetricBusinessROI, "0.13%")
	// Conversion rounds half away from zero too: 0.01 USD at 0.5 is half a
	// fen, which is one fen; minus that is minus one.
	half := big.NewRat(1, 2)
	if got := ConvertMinor(big.NewInt(1), 2, 2, half); got.Int64() != 1 {
		t.Errorf("0.005 converted to %s", got)
	}
	if got := ConvertMinor(big.NewInt(-1), 2, 2, half); got.Int64() != -1 {
		t.Errorf("-0.005 converted to %s", got)
	}
	if got := ConvertMinor(big.NewInt(3), 2, 0, big.NewRat(50, 1)); got.Int64() != 2 {
		t.Errorf("0.03 USD at 50 JPY is 1.5 yen, rounds to 2; got %s", got)
	}
}

func TestReportParametersAreRefusedByName(t *testing.T) {
	for _, tc := range []struct {
		field  string
		change func(*ReportParams)
	}{
		{"window.timezone", func(p *ReportParams) { p.Window.Timezone = "Mars/Olympus" }},
		{"window.start", func(p *ReportParams) { p.Window.Start = "2026-9-1" }},
		{"window.end", func(p *ReportParams) { p.Window.End = "2026-08-31" }},
		{"report_currency", func(p *ReportParams) { p.ReportCurrency = "RMB" }},
		{"rates", func(p *ReportParams) { p.Rates = []ExchangeRate{{From: "USD", To: "CNY", Rate: "7.1234567890123"}} }},
		{"rates", func(p *ReportParams) { p.Rates = []ExchangeRate{{From: "USD", To: "CNY", Rate: "-7"}} }},
		{"rates", func(p *ReportParams) { p.Rates = []ExchangeRate{{From: "USD", To: "EUR", Rate: "0.9"}} }},
		{"rates", func(p *ReportParams) {
			p.Rates = []ExchangeRate{{From: "USD", To: "CNY", Rate: "7"}, {From: "USD", To: "CNY", Rate: "7.1"}}
		}},
		{"attribution_method", func(p *ReportParams) { p.AttributionMethod = "time_decay" }},
		{"generated_at", func(p *ReportParams) { p.GeneratedAt = "" }},
	} {
		params := sampleParams()
		tc.change(&params)
		_, err := CalculateROI(ReportInput{Params: params})
		if fieldErr, ok := errors.AsType[FieldError](err); !ok || fieldErr.Field != tc.field {
			t.Errorf("%s: err = %v", tc.field, err)
		}
	}
}

// Q6: a cost split over periods counts in a window by each period's first
// day; the rest of it by when it was incurred.
func TestAPeriodShareCountsInTheWindowOfItsMonth(t *testing.T) {
	amount := int64(90000)
	allocations, err := ResolveAllocations(&amount, "CNY", weightShares(
		"period", "2026-08", "period", "2026-09", "period", "2026-10"))
	if err != nil {
		t.Fatal(err)
	}
	cost := sampleCost("c-quarter", amount)
	cost.IncurredAt = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cost.Allocations = allocations
	result := calculate(t, ReportInput{Params: sampleParams(), Costs: []CostRevision{cost}})
	if spend := wantOK(t, result, MetricSpendTotal, "300.00"); *spend.Records[0].AmountMinor != "30000" {
		t.Fatalf("spend records = %+v", spend.Records)
	}
}

func TestEveryReportStatesTheStageLimitation(t *testing.T) {
	result := calculate(t, ReportInput{Params: sampleParams()})
	want := "阶段是自由文本，没有先后顺序；转化率只按线索是否到达过某阶段计算，不反映阶段之间的先后。"
	if !slices.Contains(result.Rules, want) {
		t.Fatalf("rules = %v", result.Rules)
	}
	for _, metric := range MetricIDs {
		if _, ok := result.Metrics[metric]; !ok {
			t.Errorf("metric %s is missing from an empty report", metric)
		}
	}
}
