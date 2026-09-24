package feedbacklearning

import (
	"cmp"
	"math/big"
	"slices"
	"strconv"
	"time"
)

// The ROI calculator (specs/034 PR 2: FR-005, FR-006, FR-025, FR-033 to
// FR-052; D14-V12, D14-V13, D14-V14, D14-V16).
//
// CalculateROI is a pure function of its input. It reads no database, no
// clock and no float: the report's "now" is params.generated_at, money is
// int64 minor units summed in math/big, and every ratio is a big.Rat until it
// is written out as a string. The same input gives the same result byte for
// byte, whatever order the records arrive in.
//
// Every metric is either {status: "ok", value, display, ...} or
// {status: "not_computable", reason, records}. Nothing is ever shown as 0,
// empty or infinite because it could not be computed, and nothing that could
// not be converted is silently left out of a total: it makes the total not
// computable, and the records say which one.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §5

// CalcVersion names this set of formulas and this rounding. Changing either
// is a new version; a stored report keeps the version it was computed with
// (FR-041).
const CalcVersion = "roi-calc/1"

// ReasonCode says why a metric is not computable. ReasonCodes is in priority
// order: when several hold, the earliest is reported (FR-051).
type ReasonCode string

const (
	ReasonNoData                  ReasonCode = "no_data"
	ReasonCurrencyUnconverted     ReasonCode = "currency_unconverted"
	ReasonLaborRateMissing        ReasonCode = "labor_rate_missing"
	ReasonMissingGrossProfit      ReasonCode = "missing_gross_profit"
	ReasonRefundWithoutGrossDelta ReasonCode = "refund_without_gross_delta"
	ReasonMissingDenominator      ReasonCode = "missing_denominator"
	ReasonZeroDenominator         ReasonCode = "zero_denominator"
)

var ReasonCodes = []ReasonCode{
	ReasonNoData, ReasonCurrencyUnconverted, ReasonLaborRateMissing, ReasonMissingGrossProfit,
	ReasonRefundWithoutGrossDelta, ReasonMissingDenominator, ReasonZeroDenominator,
}

// MetricID names one metric of a report (contract §3).
type MetricID string

const (
	MetricSpendTotal                MetricID = "spend_total"
	MetricAdSpendTotal              MetricID = "ad_spend_total"
	MetricQualifiedLeads            MetricID = "qualified_leads"
	MetricBookings                  MetricID = "bookings"
	MetricDeals                     MetricID = "deals"
	MetricNetRevenue                MetricID = "net_revenue"
	MetricAttributedNetRevenue      MetricID = "attributed_net_revenue"
	MetricAttributedGrossProfit     MetricID = "attributed_gross_profit"
	MetricAttributionCoverageCount  MetricID = "attribution_coverage_count"
	MetricAttributionCoverageAmount MetricID = "attribution_coverage_amount"
	MetricConversionRate            MetricID = "conversion_rate"
	MetricCostPerQualifiedLead      MetricID = "cost_per_qualified_lead"
	MetricCostPerDeal               MetricID = "cost_per_deal"
	MetricBusinessROI               MetricID = "business_roi"
	MetricRevenueToSpend            MetricID = "revenue_to_spend"
	MetricAdROAS                    MetricID = "ad_roas"
)

var MetricIDs = []MetricID{
	MetricSpendTotal, MetricAdSpendTotal, MetricQualifiedLeads, MetricBookings, MetricDeals,
	MetricNetRevenue, MetricAttributedNetRevenue, MetricAttributedGrossProfit,
	MetricAttributionCoverageCount, MetricAttributionCoverageAmount, MetricConversionRate,
	MetricCostPerQualifiedLead, MetricCostPerDeal, MetricBusinessROI, MetricRevenueToSpend,
	MetricAdROAS,
}

// FormulaID is the formula a metric was computed with under this calc
// version: "business_roi/1". A formula change bumps its number and
// CalcVersion together.
func FormulaID(metric MetricID) string {
	return string(metric) + "/1"
}

// AttributionMethod is the report's choice of how a multi-touch deal is
// shared out (FR-034). It is the person's choice, never the system's.
type AttributionMethod string

const (
	AttributeFirstTouch       AttributionMethod = "first_touch"
	AttributeLastTouch        AttributionMethod = "last_touch"
	AttributeEvenSplit        AttributionMethod = "even_split"
	AttributeJudgementWeights AttributionMethod = "judgement_weights"
)

var AttributionMethods = []AttributionMethod{
	AttributeFirstTouch, AttributeLastTouch, AttributeEvenSplit, AttributeJudgementWeights,
}

// Units of a metric's display string. The page picks its label by unit; the
// display string itself is the server's and is shown as it is.
const (
	UnitMoney   = "money"
	UnitCount   = "count"
	UnitPercent = "percent"
	UnitTimes   = "times"
)

// Metric statuses.
const (
	StatusOK            = "ok"
	StatusNotComputable = "not_computable"
)

// Bounds on report parameters.
const (
	MaxRates      = 20
	MaxScopeItems = 200
)

// ReportWindow is one analysis window: calendar days in the brand's
// timezone, end inclusive.
type ReportWindow struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

// ExchangeRate is one user-entered rate: Rate units of To per unit of From.
// Who entered it and when are the server's to write.
type ExchangeRate struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Rate      string `json:"rate"`
	Note      string `json:"note"`
	EnteredBy string `json:"entered_by"`
	EnteredAt string `json:"entered_at"`
}

// ConversionStages names the two stages of the conversion rate (FR-048).
type ConversionStages struct {
	FromStage string `json:"from_stage"`
	ToStage   string `json:"to_stage"`
}

// ReportScope narrows a report to accounts, works and campaign labels.
// Empty everywhere is the whole brand.
type ReportScope struct {
	AccountIDs     []string `json:"account_ids"`
	WorkIDs        []string `json:"work_ids"`
	CampaignLabels []string `json:"campaign_labels"`
}

// ReportParams is contract §5.1.
type ReportParams struct {
	Window            ReportWindow     `json:"window"`
	ReportCurrency    string           `json:"report_currency"`
	Rates             []ExchangeRate   `json:"rates"`
	AttributionMethod string           `json:"attribution_method"`
	Conversion        ConversionStages `json:"conversion"`
	BookingStage      string           `json:"booking_stage"`
	Scope             ReportScope      `json:"scope"`
	// GeneratedAt is the report's "now": the cut-off for late refunds
	// (FR-047). The server writes it; the calculator only reads it.
	GeneratedAt string `json:"generated_at"`
}

// ReportInput is a frozen report input: parameters and record revisions.
// Revisions may come in any order and may include older revisions; the
// calculator uses the latest revision of each id, and for leads it also
// reads every revision's stage.
type ReportInput struct {
	Params       ReportParams          `json:"params"`
	Costs        []CostRevision        `json:"costs"`
	Leads        []LeadRevision        `json:"leads"`
	Touches      []TouchRevision       `json:"touches"`
	Deals        []DealRevision        `json:"deals"`
	Adjustments  []AdjustmentRevision  `json:"adjustments"`
	Attributions []AttributionRevision `json:"attributions"`
}

// RecordRef is one record a metric was computed from (FR-050). Amounts are
// minor-unit strings: AmountMinor in the record's own currency,
// ConvertedMinor in the report currency, absent when it could not be
// converted.
type RecordRef struct {
	Kind           string  `json:"kind"`
	ID             string  `json:"id"`
	Revision       int     `json:"revision"`
	AmountMinor    *string `json:"amount_minor,omitempty"`
	Currency       string  `json:"currency,omitempty"`
	ConvertedMinor *string `json:"converted_minor,omitempty"`
}

// MetricResult is one metric's result. Every number in it is a string: Value is a
// minor-unit integer, a count, or a reduced rational ("2", "-1/2"); Display
// is Value written for people, rounded half away from zero.
type MetricResult struct {
	Status      string      `json:"status"`
	Value       string      `json:"value,omitempty"`
	Display     string      `json:"display,omitempty"`
	Unit        string      `json:"unit"`
	Numerator   string      `json:"numerator,omitempty"`
	Denominator string      `json:"denominator,omitempty"`
	Reason      ReasonCode  `json:"reason,omitempty"`
	Formula     string      `json:"formula"`
	Records     []RecordRef `json:"records"`
}

// BreakdownAmount is one amount in a breakdown row: ok with a value, or not
// computable with a reason.
type BreakdownAmount struct {
	Status  string     `json:"status"`
	Value   string     `json:"value,omitempty"`
	Display string     `json:"display,omitempty"`
	Reason  ReasonCode `json:"reason,omitempty"`
}

// BreakdownRow is what was attributed to one work, one account, or one of
// the rows that are not a work. DealsTouched counts deals, each once per row;
// the same deal can appear under several works (FR-038).
type BreakdownRow struct {
	Kind                  string          `json:"kind"`
	ID                    string          `json:"id"`
	AttributedNetRevenue  BreakdownAmount `json:"attributed_net_revenue"`
	AttributedGrossProfit BreakdownAmount `json:"attributed_gross_profit"`
	DealsTouched          int             `json:"deals_touched"`
}

// Breakdown is contract §5.2's breakdown. Shares of touches with no work go
// to AccountLevelUnknownWork, and with no account either to
// BrandLevelUnknownAccount - never to a work (FR-036). Deals nobody
// attributed are Unattributed: in the brand's totals, in no work's or
// account's row (FR-035). EvenSplitFallback lists the deals that
// judgement_weights had no weights for and split evenly instead (FR-034).
type Breakdown struct {
	ByWork                   []BreakdownRow `json:"by_work"`
	ByAccount                []BreakdownRow `json:"by_account"`
	AccountLevelUnknownWork  BreakdownRow   `json:"account_level_unknown_work"`
	BrandLevelUnknownAccount BreakdownRow   `json:"brand_level_unknown_account"`
	Unattributed             BreakdownRow   `json:"unattributed"`
	EvenSplitFallback        []string       `json:"even_split_fallback"`
}

// Result is contract §5.2.
type Result struct {
	CalcVersion       string                    `json:"calc_version"`
	Window            ReportWindow              `json:"window"`
	ReportCurrency    string                    `json:"report_currency"`
	AttributionMethod string                    `json:"attribution_method"`
	GeneratedAt       string                    `json:"generated_at"`
	Metrics           map[MetricID]MetricResult `json:"metrics"`
	Breakdown         Breakdown                 `json:"breakdown"`
	Rules             []string                  `json:"rules"`
}

// ReportRules are the calculation rules and limits every report states, in
// this order (FR-038, FR-047, FR-048a and the scope and allocation rules).
var ReportRules = []string{
	"成本按发生时间、线索按首次登记时间、成交按成交时间落入窗口；退款与调整跟随所属成交计入，截止到报告生成时间。",
	"分摊到期间的成本份额按该期间第一天落入窗口，其余份额按成本的发生时间。",
	"阶段是自由文本，没有先后顺序；转化率只按线索是否到达过某阶段计算，不反映阶段之间的先后。",
	"品牌层面的线索、预约、成交数按线索与成交去重计数，与触点数量无关。",
	"同一笔成交可能出现在多个作品下，不可相加。",
	"来源不明、未判断或没有采信触点的成交不分给作品与账号，只计入品牌合计与「来源不明 / 未判断」。",
	"范围过滤只作用于投入与归因到作品、账号的份额；线索、预约、成交数与成交净额是品牌层面的数。",
	"跨币种只按本报告录入的汇率换算；没有汇率的记录使相关指标不可计算，不会被排除。",
	"退款与调整的毛利冲减未填写时，该成交的毛利不可计算，不按比例推算。",
	"舍入一律四舍五入、远离零；分摊与多触点分配用最大余数法，各份之和等于原额。",
}

// ---------------------------------------------------------------- params

type preparedParams struct {
	params      ReportParams
	start, end  time.Time // end is exclusive: the day after the window's last day
	generatedAt time.Time
	method      AttributionMethod
	currency    string
	digits      int
	rates       map[string]*big.Rat // by source currency
	scoped      bool
	accounts    map[string]bool
	works       map[string]bool
	labels      map[string]bool
}

func setOf(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	return set
}

// prepareReportParams checks report parameters and resolves the window to
// instants. Every refusal names its field.
func prepareReportParams(params ReportParams) (preparedParams, error) {
	prepared := preparedParams{params: params}
	location, err := time.LoadLocation(params.Window.Timezone)
	if err != nil || params.Window.Timezone == "" {
		return prepared, FieldError{Field: "window.timezone", Reason: "not a known timezone"}
	}
	start, err := time.ParseInLocation(time.DateOnly, params.Window.Start, location)
	if err != nil {
		return prepared, FieldError{Field: "window.start", Reason: "not a YYYY-MM-DD date"}
	}
	last, err := time.ParseInLocation(time.DateOnly, params.Window.End, location)
	if err != nil {
		return prepared, FieldError{Field: "window.end", Reason: "not a YYYY-MM-DD date"}
	}
	if last.Before(start) {
		return prepared, FieldError{Field: "window.end", Reason: "before the start"}
	}
	// The window ends where the day after its last day begins.
	prepared.start = start
	prepared.end = time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, location)
	digits, ok := CurrencyDigits(params.ReportCurrency)
	if !ok {
		return prepared, FieldError{Field: "report_currency", Reason: "not a supported currency"}
	}
	prepared.currency, prepared.digits = params.ReportCurrency, digits
	if len(params.Rates) > MaxRates {
		return prepared, FieldError{Field: "rates", Reason: "too many"}
	}
	prepared.rates = map[string]*big.Rat{}
	for _, rate := range params.Rates {
		if _, known := CurrencyDigits(rate.From); !known || rate.From == params.ReportCurrency {
			return prepared, FieldError{Field: "rates", Reason: "from is not another supported currency"}
		}
		if rate.To != params.ReportCurrency {
			return prepared, FieldError{Field: "rates", Reason: "to must be the report currency"}
		}
		if _, repeated := prepared.rates[rate.From]; repeated {
			return prepared, FieldError{Field: "rates", Reason: "one rate per currency"}
		}
		parsed, rateErr := ParseRate("rates", rate.Rate)
		if rateErr != nil {
			return prepared, rateErr
		}
		if checkRunes("rates", rate.Note, MaxNoteRunes) != nil {
			return prepared, FieldError{Field: "rates", Reason: "note too long"}
		}
		prepared.rates[rate.From] = parsed
	}
	if !oneOf(params.AttributionMethod, AttributionMethods) {
		return prepared, FieldError{Field: "attribution_method", Reason: "not an allowed value"}
	}
	prepared.method = AttributionMethod(params.AttributionMethod)
	if err = firstError(
		checkRunes("conversion.from_stage", params.Conversion.FromStage, MaxStageRunes),
		checkRunes("conversion.to_stage", params.Conversion.ToStage, MaxStageRunes),
		checkRunes("booking_stage", params.BookingStage, MaxStageRunes),
	); err != nil {
		return prepared, err
	}
	scope := params.Scope
	if len(scope.AccountIDs)+len(scope.WorkIDs)+len(scope.CampaignLabels) > MaxScopeItems {
		return prepared, FieldError{Field: "scope", Reason: "too many"}
	}
	prepared.accounts, prepared.works, prepared.labels = setOf(scope.AccountIDs), setOf(scope.WorkIDs), setOf(scope.CampaignLabels)
	prepared.scoped = len(prepared.accounts)+len(prepared.works)+len(prepared.labels) > 0
	generated, err := time.Parse(time.RFC3339, params.GeneratedAt)
	if err != nil {
		return prepared, FieldError{Field: "generated_at", Reason: "not an RFC 3339 timestamp"}
	}
	prepared.generatedAt = generated
	return prepared, nil
}

func (p preparedParams) inWindow(at time.Time) bool {
	return !at.Before(p.start) && at.Before(p.end)
}

// periodInWindow places a YYYY-MM period on its first day, in the window's
// timezone, so two back-to-back windows never both count one period.
func (p preparedParams) periodInWindow(period string) bool {
	first, err := time.ParseInLocation("2006-01", period, p.start.Location())
	return err == nil && p.inWindow(first)
}

// convert answers minor units of the report currency, or nil when there is
// no rate for this currency (FR-006).
func (p preparedParams) convert(minor *big.Int, currency string) *big.Int {
	if currency == p.currency {
		return new(big.Int).Set(minor)
	}
	rate, ok := p.rates[currency]
	fromDigits, known := CurrencyDigits(currency)
	if !ok || !known {
		return nil
	}
	return ConvertMinor(minor, fromDigits, p.digits, rate)
}

// ---------------------------------------------------------------- accumulation

// reasonRank is a reason's place in ReasonCodes; "" ranks after all of them.
func reasonRank(reason ReasonCode) int {
	if index := slices.Index(ReasonCodes, reason); index >= 0 {
		return index
	}
	return len(ReasonCodes)
}

func earliest(reasons ...ReasonCode) ReasonCode {
	best := ReasonCode("")
	for _, reason := range reasons {
		if reason != "" && reasonRank(reason) < reasonRank(best) {
			best = reason
		}
	}
	return best
}

// quantity is a running total that remembers every record it came from and
// the earliest reason, if any, that it cannot be computed.
type quantity struct {
	total   *big.Int
	reason  ReasonCode
	records []RecordRef
	count   int
	// present: there was something to add up, even if nothing was added.
	present bool
}

func newQuantity() *quantity { return &quantity{total: new(big.Int)} }

func (q *quantity) add(value *big.Int, reason ReasonCode, record RecordRef) {
	q.count++
	q.records = append(q.records, record)
	if reason != "" {
		q.reason = earliest(q.reason, reason)
		return
	}
	q.total.Add(q.total, value)
}

// fail marks the total not computable without adding to it.
func (q *quantity) fail(reason ReasonCode, record RecordRef) {
	q.count++
	q.records = append(q.records, record)
	q.reason = earliest(q.reason, reason)
}

// status is the quantity's reason, with an empty quantity being no_data.
func (q *quantity) status() ReasonCode {
	if q.count == 0 && !q.present {
		return ReasonNoData
	}
	return q.reason
}

func minorText(value *big.Int) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func sortedRecords(groups ...[]RecordRef) []RecordRef {
	records := []RecordRef{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, record := range group {
			key := record.Kind + "\x00" + record.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			records = append(records, record)
		}
	}
	slices.SortFunc(records, func(left, right RecordRef) int {
		return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID))
	})
	return records
}

func notComputable(metric MetricID, unit string, reason ReasonCode, records ...[]RecordRef) MetricResult {
	return MetricResult{
		Status: StatusNotComputable, Unit: unit, Reason: reason,
		Formula: FormulaID(metric), Records: sortedRecords(records...),
	}
}

// moneyDisplay writes minor units of the report currency for people.
func (p preparedParams) moneyDisplay(minor *big.Rat) string {
	major := new(big.Rat).Quo(minor, new(big.Rat).SetInt(pow10(p.digits)))
	return FormatRat(major, p.digits)
}

func (p preparedParams) moneyMetric(metric MetricID, q *quantity) MetricResult {
	if reason := q.status(); reason != "" {
		return notComputable(metric, UnitMoney, reason, q.records)
	}
	return MetricResult{
		Status: StatusOK, Value: q.total.String(), Display: p.moneyDisplay(new(big.Rat).SetInt(q.total)),
		Unit: UnitMoney, Formula: FormulaID(metric), Records: sortedRecords(q.records),
	}
}

func countMetric(metric MetricID, records []RecordRef) MetricResult {
	count := strconv.Itoa(len(records))
	return MetricResult{
		Status: StatusOK, Value: count, Display: count, Unit: UnitCount,
		Formula: FormulaID(metric), Records: sortedRecords(records),
	}
}

// ratioMetric divides numerator by denominator. Either side's reason makes
// the ratio not computable, the earliest winning; a zero denominator is
// zero_denominator. Nothing is ever divided by zero and nothing is shown as
// infinite.
func (p preparedParams) ratioMetric(metric MetricID, unit string, numerator, denominator *big.Int,
	numeratorReason, denominatorReason ReasonCode, records ...[]RecordRef) MetricResult {
	reason := earliest(numeratorReason, denominatorReason)
	if reason == "" && denominator.Sign() == 0 {
		reason = ReasonZeroDenominator
	}
	if reason != "" {
		return notComputable(metric, unit, reason, records...)
	}
	value := new(big.Rat).SetFrac(numerator, denominator)
	display := ""
	switch unit {
	case UnitPercent:
		display = FormatRat(new(big.Rat).Mul(value, big.NewRat(100, 1)), 2) + "%"
	case UnitTimes:
		display = FormatRat(value, 2) + " 倍"
	default:
		display = p.moneyDisplay(value)
	}
	return MetricResult{
		Status: StatusOK, Value: value.RatString(), Display: display, Unit: unit,
		Numerator: numerator.String(), Denominator: denominator.String(),
		Formula: FormulaID(metric), Records: sortedRecords(records...),
	}
}

// ---------------------------------------------------------------- records

func latestByID[T any](items []T, id func(T) string, revision func(T) int) []T {
	latest := map[string]T{}
	for _, item := range items {
		if current, ok := latest[id(item)]; !ok || revision(item) > revision(current) {
			latest[id(item)] = item
		}
	}
	keys := make([]string, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]T, 0, len(keys))
	for _, key := range keys {
		out = append(out, latest[key])
	}
	return out
}

// costLine is one cost's part in the window: the sum of its shares that fall
// in the window and the scope, in its own currency and converted.
type costLine struct {
	record    RecordRef
	adSpend   bool
	converted *big.Int
	reason    ReasonCode
}

func (p preparedParams) costInScope(cost CostRevision) bool {
	return p.accounts[cost.AccountID] || p.works[cost.WorkID] || p.labels[cost.CampaignLabel]
}

func (p preparedParams) shareInScope(cost CostRevision, allocation CostAllocation) bool {
	switch allocation.TargetKind {
	case TargetWork:
		return p.works[allocation.TargetID]
	case TargetAccount:
		return p.accounts[allocation.TargetID]
	case TargetCampaignLabel:
		return p.labels[allocation.TargetID]
	}
	return p.costInScope(cost)
}

func (p preparedParams) costLines(costs []CostRevision) []costLine {
	lines := []costLine{}
	for _, cost := range latestByID(costs, func(c CostRevision) string { return c.CostID },
		func(c CostRevision) int { return c.Revision }) {
		if cost.Voided {
			continue
		}
		counted := new(big.Int)
		included := false
		if len(cost.Allocations) == 0 {
			included = p.inWindow(cost.IncurredAt) && (!p.scoped || p.costInScope(cost))
			if included && cost.AmountMinor != nil {
				counted.SetInt64(int64(*cost.AmountMinor))
			}
		}
		for _, allocation := range cost.Allocations {
			inWindow := p.inWindow(cost.IncurredAt)
			if allocation.TargetKind == TargetPeriod {
				inWindow = p.periodInWindow(allocation.TargetID)
			}
			if inWindow && (!p.scoped || p.shareInScope(cost, allocation)) {
				included = true
				counted.Add(counted, big.NewInt(int64(allocation.AllocatedMinor)))
			}
		}
		if !included {
			continue
		}
		line := costLine{
			record:  RecordRef{Kind: "cost", ID: cost.CostID, Revision: cost.Revision, Currency: cost.Currency},
			adSpend: cost.AdSpend,
		}
		if cost.AmountMinor == nil {
			line.reason = ReasonLaborRateMissing
		} else {
			line.record.AmountMinor = minorText(counted)
			line.converted = p.convert(counted, cost.Currency)
			if line.converted == nil {
				line.reason = ReasonCurrencyUnconverted
			}
			line.record.ConvertedMinor = minorText(line.converted)
		}
		lines = append(lines, line)
	}
	return lines
}

// dealLine is one deal closed in the window, with its refunds/adjustments up
// to the report's generation time, the touches its judgement accepted, and
// its shares.
type dealLine struct {
	deal         DealRevision
	record       RecordRef
	netConverted *big.Int
	netReason    ReasonCode
	grossConv    *big.Int
	grossReason  ReasonCode
	adjustments  []RecordRef
	touches      []TouchRevision
	netShares    []int64
	grossShares  []int64
	fallback     bool
}

func (d dealLine) attributed() bool { return len(d.touches) > 0 }

// grossProfit is FR-025, in the deal's currency: the basis plus every
// adjustment's gross delta. The gross delta is SIGNED and added (主控已裁定
// 2026-09-25): "毛利冲减 300" is stored as -300 and lowers the gross profit by
// 300. Unlike the revenue delta, it is not a reduction.
func grossProfit(deal DealRevision, adjustments []AdjustmentRevision) (*big.Int, ReasonCode) {
	var gross *big.Int
	switch deal.GrossBasis {
	case GrossStated:
		if deal.GrossProfitMinor == nil {
			return nil, ReasonMissingGrossProfit
		}
		gross = big.NewInt(int64(*deal.GrossProfitMinor))
	case GrossCOGS:
		if deal.COGSMinor == nil {
			return nil, ReasonMissingGrossProfit
		}
		gross = new(big.Int).Sub(big.NewInt(int64(deal.AmountMinor)), big.NewInt(int64(*deal.COGSMinor)))
	default:
		return nil, ReasonMissingGrossProfit
	}
	for _, adjustment := range adjustments {
		if adjustment.GrossDeltaMinor == nil {
			return nil, ReasonRefundWithoutGrossDelta
		}
		gross.Add(gross, big.NewInt(int64(*adjustment.GrossDeltaMinor)))
	}
	return gross, ""
}

func compareTouches(left, right TouchRevision) int {
	return cmp.Or(left.OccurredAt.Compare(right.OccurredAt), cmp.Compare(left.TouchID, right.TouchID))
}

// touchWeights is FR-034: the report's method turned into one weight per
// accepted touch, the touches being in (occurred_at, touch_id) order.
func (p preparedParams) touchWeights(touches []TouchRevision, judgement AttributionRevision) ([]int64, bool) {
	weights := make([]int64, len(touches))
	pick := func(role TouchRole, last bool) {
		chosen := -1
		for i, touch := range touches {
			if touch.Role == role && (chosen < 0 || last) {
				chosen = i
			}
		}
		if chosen < 0 {
			chosen = 0
			if last {
				chosen = len(touches) - 1
			}
		}
		weights[chosen] = 1
	}
	switch p.method {
	case AttributeFirstTouch:
		pick(RoleFirstTouch, false)
		return weights, false
	case AttributeLastTouch:
		pick(RolePreBooking, true)
		return weights, false
	case AttributeJudgementWeights:
		if len(judgement.Weights) > 0 && len(judgement.Weights) == len(judgement.TouchIDs) {
			byTouch := map[string]int64{}
			for i, id := range judgement.TouchIDs {
				byTouch[id] = judgement.Weights[i]
			}
			total := int64(0)
			for i, touch := range touches {
				weights[i] = byTouch[touch.TouchID]
				total += weights[i]
			}
			if total > 0 {
				return weights, false
			}
		}
		for i := range weights {
			weights[i] = 1
		}
		return weights, true
	}
	for i := range weights {
		weights[i] = 1
	}
	return weights, false
}

func (p preparedParams) dealLines(input ReportInput) []dealLine {
	touches := map[string]TouchRevision{}
	for _, touch := range latestByID(input.Touches, func(t TouchRevision) string { return t.TouchID },
		func(t TouchRevision) int { return t.Revision }) {
		if !touch.Voided {
			touches[touch.TouchID] = touch
		}
	}
	judgements := map[string]AttributionRevision{}
	for _, judgement := range latestByID(input.Attributions, func(a AttributionRevision) string { return a.DealID },
		func(a AttributionRevision) int { return a.Revision }) {
		if !judgement.Voided {
			judgements[judgement.DealID] = judgement
		}
	}
	adjustments := map[string][]AdjustmentRevision{}
	for _, adjustment := range latestByID(input.Adjustments, func(a AdjustmentRevision) string { return a.AdjustmentID },
		func(a AdjustmentRevision) int { return a.Revision }) {
		// FR-047: an adjustment follows its deal, up to the report's
		// generation time and no later.
		if !adjustment.Voided && !adjustment.OccurredAt.After(p.generatedAt) {
			adjustments[adjustment.DealID] = append(adjustments[adjustment.DealID], adjustment)
		}
	}
	lines := []dealLine{}
	for _, deal := range latestByID(input.Deals, func(d DealRevision) string { return d.DealID },
		func(d DealRevision) int { return d.Revision }) {
		if deal.Voided || !p.inWindow(deal.ClosedAt) {
			continue
		}
		line := dealLine{deal: deal}
		net := big.NewInt(int64(deal.AmountMinor))
		for _, adjustment := range adjustments[deal.DealID] {
			net.Sub(net, big.NewInt(int64(adjustment.RevenueDeltaMinor)))
			line.adjustments = append(line.adjustments, RecordRef{
				Kind: "adjustment", ID: adjustment.AdjustmentID, Revision: adjustment.Revision,
				AmountMinor: minorText(big.NewInt(int64(adjustment.RevenueDeltaMinor))), Currency: adjustment.Currency,
			})
		}
		line.netConverted = p.convert(net, deal.Currency)
		if line.netConverted == nil {
			line.netReason = ReasonCurrencyUnconverted
		}
		line.record = RecordRef{
			Kind: "deal", ID: deal.DealID, Revision: deal.Revision, AmountMinor: minorText(net),
			Currency: deal.Currency, ConvertedMinor: minorText(line.netConverted),
		}
		gross, grossReason := grossProfit(deal, adjustments[deal.DealID])
		line.grossReason = grossReason
		if gross != nil {
			line.grossConv = p.convert(gross, deal.Currency)
			if line.grossConv == nil {
				line.grossReason = ReasonCurrencyUnconverted
			}
		}
		// FR-035: only touches the judgement accepted take part. No
		// judgement, "unknown", or nothing accepted: the deal stays
		// unattributed. Nothing here goes looking for touches of its own.
		judgement, judged := judgements[deal.DealID]
		if judged && judgement.Judgement != JudgementUnknown {
			for _, id := range judgement.TouchIDs {
				if touch, ok := touches[id]; ok {
					line.touches = append(line.touches, touch)
				}
			}
		}
		slices.SortFunc(line.touches, compareTouches)
		if line.attributed() {
			weights, fallback := p.touchWeights(line.touches, judgement)
			line.fallback = fallback
			if line.netConverted != nil {
				line.netShares, _ = SplitLargestRemainder(line.netConverted.Int64(), weights)
			}
			if line.grossConv != nil {
				line.grossShares, _ = SplitLargestRemainder(line.grossConv.Int64(), weights)
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func (p preparedParams) touchInScope(touch TouchRevision) bool {
	return !p.scoped || p.works[touch.WorkID] || p.accounts[touch.AccountID]
}

// ---------------------------------------------------------------- breakdown

type rowAccumulator struct {
	kind, id string
	net      *quantity
	gross    *quantity
	deals    map[string]bool
}

func newRow(kind, id string) *rowAccumulator {
	return &rowAccumulator{kind: kind, id: id, net: newQuantity(), gross: newQuantity(), deals: map[string]bool{}}
}

func (p preparedParams) rowAmount(q *quantity) BreakdownAmount {
	if q.reason != "" {
		return BreakdownAmount{Status: StatusNotComputable, Reason: q.reason}
	}
	return BreakdownAmount{Status: StatusOK, Value: q.total.String(), Display: p.moneyDisplay(new(big.Rat).SetInt(q.total))}
}

func (p preparedParams) row(accumulator *rowAccumulator) BreakdownRow {
	return BreakdownRow{
		Kind: accumulator.kind, ID: accumulator.id,
		AttributedNetRevenue:  p.rowAmount(accumulator.net),
		AttributedGrossProfit: p.rowAmount(accumulator.gross),
		DealsTouched:          len(accumulator.deals),
	}
}

func (p preparedParams) rows(accumulators map[string]*rowAccumulator) []BreakdownRow {
	keys := make([]string, 0, len(accumulators))
	for key := range accumulators {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	rows := make([]BreakdownRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, p.row(accumulators[key]))
	}
	return rows
}

// addShare adds one touch's share of one deal to a row.
func addShare(row *rowAccumulator, line dealLine, index int) {
	row.deals[line.deal.DealID] = true
	if line.netShares != nil {
		row.net.add(big.NewInt(line.netShares[index]), "", line.record)
	} else {
		row.net.fail(line.netReason, line.record)
	}
	if line.grossShares != nil {
		row.gross.add(big.NewInt(line.grossShares[index]), "", line.record)
	} else {
		row.gross.fail(line.grossReason, line.record)
	}
}

// ---------------------------------------------------------------- leads

// leadStages answers, for each lead that counts at brand level (latest
// revision not voided and not merged into another), every stage it or a lead
// merged into it has ever been at.
func leadStages(leads []LeadRevision) (map[string]LeadRevision, map[string]map[string]bool) {
	current := map[string]LeadRevision{}
	for _, lead := range latestByID(leads, func(l LeadRevision) string { return l.LeadID },
		func(l LeadRevision) int { return l.Revision }) {
		current[lead.LeadID] = lead
	}
	root := func(id string) string {
		for range maxMergeChain {
			lead, ok := current[id]
			if !ok || lead.MergedInto == "" {
				return id
			}
			id = lead.MergedInto
		}
		return id
	}
	stages := map[string]map[string]bool{}
	for _, lead := range leads {
		owner := root(lead.LeadID)
		if stages[owner] == nil {
			stages[owner] = map[string]bool{}
		}
		stages[owner][lead.Stage] = true
	}
	counted := map[string]LeadRevision{}
	for id, lead := range current {
		if !lead.Voided && lead.MergedInto == "" {
			counted[id] = lead
		}
	}
	return counted, stages
}

// ---------------------------------------------------------------- calculate

// CalculateROI computes a report from a frozen input (FR-040). It fails only
// on parameters it cannot use, naming the field.
func CalculateROI(input ReportInput) (Result, error) {
	p, err := prepareReportParams(input.Params)
	if err != nil {
		return Result{}, err
	}
	metrics := map[MetricID]MetricResult{}

	// Spend.
	spend, adSpend := newQuantity(), newQuantity()
	for _, line := range p.costLines(input.Costs) {
		spend.add(line.converted, line.reason, line.record)
		if line.adSpend {
			adSpend.add(line.converted, line.reason, line.record)
		}
	}
	metrics[MetricSpendTotal] = p.moneyMetric(MetricSpendTotal, spend)
	metrics[MetricAdSpendTotal] = p.moneyMetric(MetricAdSpendTotal, adSpend)

	// Deals and what was attributed.
	lines := p.dealLines(input)
	netRevenue := newQuantity()
	attributedNet, attributedGross, paidNet := newQuantity(), newQuantity(), newQuantity()
	attributedCount := 0
	coveredNet := newQuantity()
	dealRecords := []RecordRef{}
	byWork, byAccount := map[string]*rowAccumulator{}, map[string]*rowAccumulator{}
	accountLevel := newRow("account_level_unknown_work", "")
	brandLevel := newRow("brand_level_unknown_account", "")
	unattributed := newRow("unattributed", "")
	fallback := []string{}
	for _, line := range lines {
		dealRecords = append(dealRecords, line.record)
		netRevenue.add(line.netConverted, line.netReason, line.record)
		for _, adjustment := range line.adjustments {
			netRevenue.records = append(netRevenue.records, adjustment)
		}
		if !line.attributed() {
			unattributed.deals[line.deal.DealID] = true
			unattributed.net.add(line.netConverted, line.netReason, line.record)
			unattributed.gross.add(line.grossConv, line.grossReason, line.record)
			continue
		}
		attributedCount++
		coveredNet.add(line.netConverted, line.netReason, line.record)
		if line.fallback {
			fallback = append(fallback, line.deal.DealID)
		}
		scopedNet, scopedGross, scopedPaid := new(big.Int), new(big.Int), new(big.Int)
		paidTouched := false
		for i, touch := range line.touches {
			if !p.touchInScope(touch) {
				continue
			}
			if line.netShares != nil {
				scopedNet.Add(scopedNet, big.NewInt(line.netShares[i]))
				if touch.Paid {
					scopedPaid.Add(scopedPaid, big.NewInt(line.netShares[i]))
				}
			}
			if line.grossShares != nil {
				scopedGross.Add(scopedGross, big.NewInt(line.grossShares[i]))
			}
			paidTouched = paidTouched || touch.Paid
			if touch.WorkID != "" {
				if byWork[touch.WorkID] == nil {
					byWork[touch.WorkID] = newRow("work", touch.WorkID)
				}
				addShare(byWork[touch.WorkID], line, i)
			}
			if touch.AccountID != "" {
				if byAccount[touch.AccountID] == nil {
					byAccount[touch.AccountID] = newRow("account", touch.AccountID)
				}
				addShare(byAccount[touch.AccountID], line, i)
			}
			switch {
			case touch.WorkID == "" && touch.AccountID != "":
				addShare(accountLevel, line, i)
			case touch.WorkID == "" && touch.AccountID == "":
				addShare(brandLevel, line, i)
			}
		}
		attributedNet.add(scopedNet, line.netReason, line.record)
		attributedGross.add(scopedGross, line.grossReason, line.record)
		if paidTouched {
			paidNet.add(scopedPaid, line.netReason, line.record)
		}
	}
	metrics[MetricDeals] = countMetric(MetricDeals, dealRecords)
	metrics[MetricNetRevenue] = p.moneyMetric(MetricNetRevenue, netRevenue)
	// With deals in the window but none attributed, "attributed" is a
	// real zero; with no deals at all it is no data.
	for _, q := range []*quantity{attributedNet, attributedGross, paidNet} {
		q.present = len(lines) > 0
	}
	metrics[MetricAttributedNetRevenue] = p.moneyMetric(MetricAttributedNetRevenue, attributedNet)
	metrics[MetricAttributedGrossProfit] = p.moneyMetric(MetricAttributedGrossProfit, attributedGross)

	// Coverage (FR-049).
	if len(lines) == 0 {
		metrics[MetricAttributionCoverageCount] = notComputable(MetricAttributionCoverageCount, UnitPercent, ReasonNoData)
		metrics[MetricAttributionCoverageAmount] = notComputable(MetricAttributionCoverageAmount, UnitPercent, ReasonNoData)
	} else {
		metrics[MetricAttributionCoverageCount] = p.ratioMetric(MetricAttributionCoverageCount, UnitPercent,
			big.NewInt(int64(attributedCount)), big.NewInt(int64(len(lines))), "", "", dealRecords)
		coveredReason := coveredNet.reason
		metrics[MetricAttributionCoverageAmount] = p.ratioMetric(MetricAttributionCoverageAmount, UnitPercent,
			coveredNet.total, netRevenue.total, coveredReason, netRevenue.reason, netRevenue.records)
	}

	// Leads (FR-037, FR-048).
	counted, stages := leadStages(input.Leads)
	leadIDs := make([]string, 0, len(counted))
	for id := range counted {
		leadIDs = append(leadIDs, id)
	}
	slices.Sort(leadIDs)
	qualified, bookings, reachedFrom, reachedBoth := []RecordRef{}, []RecordRef{}, []RecordRef{}, []RecordRef{}
	for _, id := range leadIDs {
		lead := counted[id]
		if !p.inWindow(lead.FirstSeenAt) {
			continue
		}
		record := RecordRef{Kind: "lead", ID: id, Revision: lead.Revision}
		if lead.Qualified {
			qualified = append(qualified, record)
		}
		reached := stages[id]
		if p.params.BookingStage != "" && reached[p.params.BookingStage] {
			bookings = append(bookings, record)
		}
		if reached[p.params.Conversion.FromStage] {
			reachedFrom = append(reachedFrom, record)
			if reached[p.params.Conversion.ToStage] {
				reachedBoth = append(reachedBoth, record)
			}
		}
	}
	metrics[MetricQualifiedLeads] = countMetric(MetricQualifiedLeads, qualified)
	if p.params.BookingStage == "" {
		metrics[MetricBookings] = notComputable(MetricBookings, UnitCount, ReasonNoData)
	} else {
		metrics[MetricBookings] = countMetric(MetricBookings, bookings)
	}
	if p.params.Conversion.FromStage == "" || p.params.Conversion.ToStage == "" {
		metrics[MetricConversionRate] = notComputable(MetricConversionRate, UnitPercent, ReasonMissingDenominator)
	} else {
		metrics[MetricConversionRate] = p.ratioMetric(MetricConversionRate, UnitPercent,
			big.NewInt(int64(len(reachedBoth))), big.NewInt(int64(len(reachedFrom))), "", "", reachedFrom)
	}

	// Ratios over spend.
	spendReason := spend.status()
	metrics[MetricCostPerQualifiedLead] = p.ratioMetric(MetricCostPerQualifiedLead, UnitMoney,
		spend.total, big.NewInt(int64(len(qualified))), spendReason, "", spend.records, qualified)
	metrics[MetricCostPerDeal] = p.ratioMetric(MetricCostPerDeal, UnitMoney,
		spend.total, big.NewInt(int64(len(lines))), spendReason, "", spend.records, dealRecords)
	// FR-043: (attributed gross profit - spend) / spend.
	roiNumerator := new(big.Int).Sub(attributedGross.total, spend.total)
	metrics[MetricBusinessROI] = p.ratioMetric(MetricBusinessROI, UnitPercent,
		roiNumerator, spend.total, attributedGross.status(), spendReason, attributedGross.records, spend.records)
	// FR-044: attributed net revenue / spend. A revenue ratio, never a
	// profit ROI.
	metrics[MetricRevenueToSpend] = p.ratioMetric(MetricRevenueToSpend, UnitTimes,
		attributedNet.total, spend.total, attributedNet.status(), spendReason, attributedNet.records, spend.records)
	// FR-045: revenue attributed to paid touches / ad spend.
	metrics[MetricAdROAS] = p.ratioMetric(MetricAdROAS, UnitTimes,
		paidNet.total, adSpend.total, paidNet.status(), adSpend.status(), paidNet.records, adSpend.records)

	slices.Sort(fallback)
	return Result{
		CalcVersion:       CalcVersion,
		Window:            input.Params.Window,
		ReportCurrency:    p.currency,
		AttributionMethod: string(p.method),
		GeneratedAt:       input.Params.GeneratedAt,
		Metrics:           metrics,
		Breakdown: Breakdown{
			ByWork:                   p.rows(byWork),
			ByAccount:                p.rows(byAccount),
			AccountLevelUnknownWork:  p.row(accountLevel),
			BrandLevelUnknownAccount: p.row(brandLevel),
			Unattributed:             p.row(unattributed),
			EvenSplitFallback:        fallback,
		},
		Rules: slices.Clone(ReportRules),
	}, nil
}
