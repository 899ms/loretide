import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Costs, leads, touches, deals and refunds/adjustments (specs/034 PR 1).
//
// Two rules this file keeps:
//
// - Every amount is a STRING, in and out. The server sends minor units as a
//   decimal integer string ("300000") and the decimal form ("3000.00"); this
//   file never turns either into a number, because a JS number is a float and
//   above 2^53 it is not even an integer. A response that sends an amount as a
//   JSON number fails the schema and degrades, rather than being trusted.
// - A labor cost with no rate has amountMinor === null and amountStatus
//   "labor_rate_missing". That is "not computable", never 0.
//
// Contract: specs/034-roi-review/contracts/roi-review.md

/** R-061's six, with no "other". */
export const ROI_EVIDENCE_TYPES = [
  "platform_linked_content", "content_comment", "customer_statement",
  "dedicated_channel", "account_only", "unknown",
] as const;
export type RoiEvidenceType = (typeof ROI_EVIDENCE_TYPES)[number];

export const ROI_PRICINGS = ["amount", "labor_time"] as const;
export type RoiPricing = (typeof ROI_PRICINGS)[number];

export const ROI_TOUCH_ROLES = ["first_touch", "pre_booking", "other"] as const;
export type RoiTouchRole = (typeof ROI_TOUCH_ROLES)[number];

export const ROI_GROSS_BASES = ["none", "stated_gross_profit", "cogs"] as const;
export type RoiGrossBasis = (typeof ROI_GROSS_BASES)[number];

export const ROI_ADJUSTMENT_KINDS = ["refund", "adjustment"] as const;
export type RoiAdjustmentKind = (typeof ROI_ADJUSTMENT_KINDS)[number];

export const ROI_RECORD_SOURCES = ["manual", "import"] as const;

/** specs/034 PR 2: what a share of a shared cost goes to, and how it is stated. */
export const ROI_ALLOCATION_TARGETS = ["work", "account", "campaign_label", "period"] as const;
export const ROI_ALLOCATION_METHODS = ["weights", "amounts"] as const;

/** The operator's judgement on a deal: R-061's four. */
export const ROI_JUDGEMENTS = ["confirmed", "operator_judgement", "multi_touch", "unknown"] as const;
export type RoiJudgement = (typeof ROI_JUDGEMENTS)[number];

/** How a report shares a multi-touch deal out; the person's choice. */
export const ROI_ATTRIBUTION_METHODS = ["first_touch", "last_touch", "even_split", "judgement_weights"] as const;
export type RoiAttributionMethod = (typeof ROI_ATTRIBUTION_METHODS)[number];

/** Why a metric is not computable, in the server's priority order. */
export const ROI_REASON_CODES = [
  "no_data", "currency_unconverted", "labor_rate_missing", "missing_gross_profit",
  "refund_without_gross_delta", "missing_denominator", "zero_denominator",
] as const;

/** The report's sixteen metrics. revenue_to_spend and ad_roas are revenue
 *  ratios; only business_roi is the profit ROI. */
export const ROI_METRIC_IDS = [
  "spend_total", "ad_spend_total", "qualified_leads", "bookings", "deals", "net_revenue",
  "attributed_net_revenue", "attributed_gross_profit", "attribution_coverage_count",
  "attribution_coverage_amount", "conversion_rate", "cost_per_qualified_lead", "cost_per_deal",
  "business_roi", "revenue_to_spend", "ad_roas",
] as const;
export type RoiMetricId = (typeof ROI_METRIC_IDS)[number];

/** ip-profile's eight. A touch may also leave the platform empty. */
export const ROI_TOUCH_PLATFORMS = [
  "xiaohongshu", "douyin", "wechat_mp", "bilibili", "zhihu", "weibo", "kuaishou", "shipinhao",
] as const;

/** Ruling Q1=A: nine currencies and their minor-unit places. */
export const ROI_CURRENCIES = [
  { code: "CNY", digits: 2 }, { code: "HKD", digits: 2 }, { code: "TWD", digits: 2 },
  { code: "USD", digits: 2 }, { code: "EUR", digits: 2 }, { code: "GBP", digits: 2 },
  { code: "SGD", digits: 2 }, { code: "JPY", digits: 0 }, { code: "KRW", digits: 0 },
] as const;

/** One share of a shared cost's revision. allocatedMinor is the server's
 *  figure; the shares of one revision add up to its amount exactly. */
export interface RoiCostAllocation {
  targetKind: string;
  targetId: string;
  method: string;
  weight: number | null;
  allocatedMinor: string;
  allocated: string;
}

export interface RoiCost {
  costId: string;
  revision: number;
  voided: boolean;
  category: string;
  pricing: string;
  /** Minor units as a decimal integer string. null: not computable. */
  amountMinor: string | null;
  /** The same amount in the currency's decimal form. null with amountMinor. */
  amount: string | null;
  amountStatus: string;
  currency: string;
  laborMinutes: number | null;
  laborRateMinor: string | null;
  laborRate: string | null;
  incurredAt: string;
  adSpend: boolean;
  accountId: string;
  workId: string;
  campaignLabel: string;
  evidenceNote: string;
  note: string;
  notDuplicateOf: string[];
  sourceType: string;
  importBatchId: string;
  recordedBy: string;
  createdAt: string;
  /** Empty for a cost that is not shared. */
  allocations: RoiCostAllocation[];
}

export interface RoiLead {
  leadId: string;
  revision: number;
  voided: boolean;
  /** A label the operator chose. Never a real name or contact detail. */
  customerRef: string;
  stage: string;
  qualified: boolean;
  firstSeenAt: string;
  /** Non-empty: this lead was merged into that one. */
  mergedInto: string;
  note: string;
  notDuplicateOf: string[];
  sourceType: string;
  importBatchId: string;
  recordedBy: string;
  createdAt: string;
}

/** Evidence, not a judgement. An empty workId is a real answer. */
export interface RoiTouch {
  touchId: string;
  revision: number;
  voided: boolean;
  leadId: string;
  evidenceType: string;
  platform: string;
  accountId: string;
  workId: string;
  publicationRecordId: string;
  role: string;
  paid: boolean;
  occurredAt: string;
  evidenceNote: string;
  note: string;
  recordedBy: string;
  createdAt: string;
}

export interface RoiDeal {
  dealId: string;
  revision: number;
  voided: boolean;
  leadId: string;
  orderRef: string;
  amountMinor: string;
  amount: string;
  currency: string;
  closedAt: string;
  grossBasis: string;
  grossProfitMinor: string | null;
  grossProfit: string | null;
  cogsMinor: string | null;
  cogs: string | null;
  note: string;
  notDuplicateOf: string[];
  sourceType: string;
  importBatchId: string;
  recordedBy: string;
  createdAt: string;
}

/** revenueDelta is a reduction: a refund of 1000.00 is "1000.00". */
export interface RoiAdjustment {
  adjustmentId: string;
  revision: number;
  voided: boolean;
  dealId: string;
  kind: string;
  revenueDeltaMinor: string;
  revenueDelta: string;
  /** Signed, added to gross profit: a reduction is negative ("-300.00").
   *  null: the operator did not say how gross profit moved. Not 0. */
  grossDeltaMinor: string | null;
  grossDelta: string | null;
  currency: string;
  occurredAt: string;
  note: string;
  recordedBy: string;
  createdAt: string;
}

/** The operator's judgement on a deal: which touches it accepts. Not
 *  evidence, and writing it changes no touch. */
export interface RoiAttribution {
  dealId: string;
  revision: number;
  voided: boolean;
  judgement: string;
  touchIds: string[];
  /** Empty: no manual weights. Otherwise one per touch id. */
  weights: number[];
  note: string;
  recordedBy: string;
  createdAt: string;
}

export interface RoiHistory<T> {
  current: T;
  revisions: T[];
}

export interface RoiCostHistory extends RoiHistory<RoiCost> {
  costId: string;
}

export interface RoiLeadDetail extends RoiHistory<RoiLead> {
  leadId: string;
  /** Its own touches and those of leads merged into it, each once. */
  touches: (RoiHistory<RoiTouch> & { touchId: string })[];
}

export interface RoiDealDetail extends RoiHistory<RoiDeal> {
  dealId: string;
  adjustments: (RoiHistory<RoiAdjustment> & { adjustmentId: string })[];
  /** Every revision of the judgement, oldest first. */
  attributions: RoiAttribution[];
}

// Lenient where the page only reads (a controlled value this build has not
// heard of stays a string), strict where it matters: every amount is a
// string or null, never a number.
const amount = z.string();
const optionalAmount = z.string().nullable().optional();
const text = z.string().optional();
const flag = z.boolean().optional();
const ids = z.array(z.string()).nullable().optional();

const allocationSchema = z.object({
  target_kind: z.string(),
  target_id: z.string(),
  method: text,
  weight: z.number().int().nullable().optional(),
  allocated_minor: amount,
  allocated: amount,
});

const costSchema = z.object({
  cost_id: z.string(),
  revision: z.number().int(),
  voided: flag,
  category: text,
  pricing: text,
  amount_minor: optionalAmount,
  amount: optionalAmount,
  amount_status: text,
  currency: text,
  labor_minutes: z.number().int().nullable().optional(),
  labor_rate_minor: optionalAmount,
  labor_rate: optionalAmount,
  incurred_at: text,
  ad_spend: flag,
  account_id: text,
  work_id: text,
  campaign_label: text,
  evidence_note: text,
  note: text,
  not_duplicate_of: ids,
  source_type: text,
  import_batch_id: text,
  recorded_by: text,
  created_at: text,
  allocations: z.array(allocationSchema).nullable().optional(),
});

const leadSchema = z.object({
  lead_id: z.string(),
  revision: z.number().int(),
  voided: flag,
  customer_ref: text,
  stage: text,
  qualified: flag,
  first_seen_at: text,
  merged_into: text,
  note: text,
  not_duplicate_of: ids,
  source_type: text,
  import_batch_id: text,
  recorded_by: text,
  created_at: text,
});

const touchSchema = z.object({
  touch_id: z.string(),
  revision: z.number().int(),
  voided: flag,
  lead_id: text,
  evidence_type: text,
  platform: text,
  account_id: text,
  work_id: text,
  publication_record_id: text,
  role: text,
  paid: flag,
  occurred_at: text,
  evidence_note: text,
  note: text,
  recorded_by: text,
  created_at: text,
});

const dealSchema = z.object({
  deal_id: z.string(),
  revision: z.number().int(),
  voided: flag,
  lead_id: text,
  order_ref: text,
  amount_minor: amount,
  amount: amount,
  currency: text,
  closed_at: text,
  gross_basis: text,
  gross_profit_minor: optionalAmount,
  gross_profit: optionalAmount,
  cogs_minor: optionalAmount,
  cogs: optionalAmount,
  note: text,
  not_duplicate_of: ids,
  source_type: text,
  import_batch_id: text,
  recorded_by: text,
  created_at: text,
});

const adjustmentSchema = z.object({
  adjustment_id: z.string(),
  revision: z.number().int(),
  voided: flag,
  deal_id: text,
  kind: text,
  revenue_delta_minor: amount,
  revenue_delta: amount,
  gross_delta_minor: optionalAmount,
  gross_delta: optionalAmount,
  currency: text,
  occurred_at: text,
  note: text,
  recorded_by: text,
  created_at: text,
});

const attributionSchema = z.object({
  deal_id: z.string(),
  revision: z.number().int(),
  voided: flag,
  judgement: text,
  touch_ids: ids,
  weights: z.array(z.number().int()).nullable().optional(),
  note: text,
  recorded_by: text,
  created_at: text,
});

function history<T extends z.ZodTypeAny>(item: T) {
  return z.object({ current: item, revisions: z.array(item) });
}

const costListSchema = z.object({ costs: z.array(costSchema).nullable().optional() });
const leadListSchema = z.object({ leads: z.array(leadSchema).nullable().optional() });
const dealListSchema = z.object({ deals: z.array(dealSchema).nullable().optional() });
const costHistorySchema = history(costSchema).extend({ cost_id: z.string() });
const leadDetailSchema = history(leadSchema).extend({
  lead_id: z.string(),
  touches: z.array(history(touchSchema).extend({ touch_id: z.string() })).nullable().optional(),
});
const dealDetailSchema = history(dealSchema).extend({
  deal_id: z.string(),
  adjustments: z.array(history(adjustmentSchema).extend({ adjustment_id: z.string() })).nullable().optional(),
  attributions: z.array(attributionSchema).nullable().optional(),
});

function toCost(wire: z.infer<typeof costSchema>): RoiCost {
  return {
    costId: wire.cost_id,
    revision: wire.revision,
    voided: wire.voided ?? false,
    category: wire.category ?? "",
    pricing: wire.pricing ?? "",
    // `?? null`, never `?? "0"`: absent means not computable, not zero.
    amountMinor: wire.amount_minor ?? null,
    amount: wire.amount ?? null,
    amountStatus: wire.amount_status ?? "",
    currency: wire.currency ?? "",
    laborMinutes: wire.labor_minutes ?? null,
    laborRateMinor: wire.labor_rate_minor ?? null,
    laborRate: wire.labor_rate ?? null,
    incurredAt: wire.incurred_at ?? "",
    adSpend: wire.ad_spend ?? false,
    accountId: wire.account_id ?? "",
    workId: wire.work_id ?? "",
    campaignLabel: wire.campaign_label ?? "",
    evidenceNote: wire.evidence_note ?? "",
    note: wire.note ?? "",
    notDuplicateOf: wire.not_duplicate_of ?? [],
    sourceType: wire.source_type ?? "",
    importBatchId: wire.import_batch_id ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
    allocations: (wire.allocations ?? []).map((share) => ({
      targetKind: share.target_kind,
      targetId: share.target_id,
      method: share.method ?? "",
      weight: share.weight ?? null,
      allocatedMinor: share.allocated_minor,
      allocated: share.allocated,
    })),
  };
}

function toAttribution(wire: z.infer<typeof attributionSchema>): RoiAttribution {
  return {
    dealId: wire.deal_id,
    revision: wire.revision,
    voided: wire.voided ?? false,
    judgement: wire.judgement ?? "",
    touchIds: wire.touch_ids ?? [],
    weights: wire.weights ?? [],
    note: wire.note ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

function toLead(wire: z.infer<typeof leadSchema>): RoiLead {
  return {
    leadId: wire.lead_id,
    revision: wire.revision,
    voided: wire.voided ?? false,
    customerRef: wire.customer_ref ?? "",
    stage: wire.stage ?? "",
    qualified: wire.qualified ?? false,
    firstSeenAt: wire.first_seen_at ?? "",
    mergedInto: wire.merged_into ?? "",
    note: wire.note ?? "",
    notDuplicateOf: wire.not_duplicate_of ?? [],
    sourceType: wire.source_type ?? "",
    importBatchId: wire.import_batch_id ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

function toTouch(wire: z.infer<typeof touchSchema>): RoiTouch {
  return {
    touchId: wire.touch_id,
    revision: wire.revision,
    voided: wire.voided ?? false,
    leadId: wire.lead_id ?? "",
    evidenceType: wire.evidence_type ?? "",
    platform: wire.platform ?? "",
    accountId: wire.account_id ?? "",
    workId: wire.work_id ?? "",
    publicationRecordId: wire.publication_record_id ?? "",
    role: wire.role ?? "",
    paid: wire.paid ?? false,
    occurredAt: wire.occurred_at ?? "",
    evidenceNote: wire.evidence_note ?? "",
    note: wire.note ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

function toDeal(wire: z.infer<typeof dealSchema>): RoiDeal {
  return {
    dealId: wire.deal_id,
    revision: wire.revision,
    voided: wire.voided ?? false,
    leadId: wire.lead_id ?? "",
    orderRef: wire.order_ref ?? "",
    amountMinor: wire.amount_minor,
    amount: wire.amount,
    currency: wire.currency ?? "",
    closedAt: wire.closed_at ?? "",
    grossBasis: wire.gross_basis ?? "",
    grossProfitMinor: wire.gross_profit_minor ?? null,
    grossProfit: wire.gross_profit ?? null,
    cogsMinor: wire.cogs_minor ?? null,
    cogs: wire.cogs ?? null,
    note: wire.note ?? "",
    notDuplicateOf: wire.not_duplicate_of ?? [],
    sourceType: wire.source_type ?? "",
    importBatchId: wire.import_batch_id ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

function toAdjustment(wire: z.infer<typeof adjustmentSchema>): RoiAdjustment {
  return {
    adjustmentId: wire.adjustment_id,
    revision: wire.revision,
    voided: wire.voided ?? false,
    dealId: wire.deal_id ?? "",
    kind: wire.kind ?? "",
    revenueDeltaMinor: wire.revenue_delta_minor,
    revenueDelta: wire.revenue_delta,
    grossDeltaMinor: wire.gross_delta_minor ?? null,
    grossDelta: wire.gross_delta ?? null,
    currency: wire.currency ?? "",
    occurredAt: wire.occurred_at ?? "",
    note: wire.note ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

// A single record that fails the schema reads as null: the caller shows "not
// available", never a record with made-up fields.

export function parseRoiCost(data: unknown): RoiCost | null {
  const parsed = parseWithFallback<z.infer<typeof costSchema> | null>(data, costSchema, null, { endpoint: "content-roi/cost" });
  return parsed ? toCost(parsed) : null;
}

export function parseRoiCostList(data: unknown): RoiCost[] {
  const parsed = parseWithFallback<z.infer<typeof costListSchema>>(data, costListSchema, { costs: [] }, { endpoint: "content-roi/costs" });
  return (parsed.costs ?? []).map(toCost);
}

export function parseRoiCostHistory(data: unknown): RoiCostHistory | null {
  const parsed = parseWithFallback<z.infer<typeof costHistorySchema> | null>(data, costHistorySchema, null, { endpoint: "content-roi/cost-history" });
  if (!parsed) return null;
  return { costId: parsed.cost_id, current: toCost(parsed.current), revisions: parsed.revisions.map(toCost) };
}

export function parseRoiLead(data: unknown): RoiLead | null {
  const parsed = parseWithFallback<z.infer<typeof leadSchema> | null>(data, leadSchema, null, { endpoint: "content-roi/lead" });
  return parsed ? toLead(parsed) : null;
}

export function parseRoiLeadList(data: unknown): RoiLead[] {
  const parsed = parseWithFallback<z.infer<typeof leadListSchema>>(data, leadListSchema, { leads: [] }, { endpoint: "content-roi/leads" });
  return (parsed.leads ?? []).map(toLead);
}

export function parseRoiLeadDetail(data: unknown): RoiLeadDetail | null {
  const parsed = parseWithFallback<z.infer<typeof leadDetailSchema> | null>(data, leadDetailSchema, null, { endpoint: "content-roi/lead-detail" });
  if (!parsed) return null;
  return {
    leadId: parsed.lead_id,
    current: toLead(parsed.current),
    revisions: parsed.revisions.map(toLead),
    touches: (parsed.touches ?? []).map((touch) => ({
      touchId: touch.touch_id,
      current: toTouch(touch.current),
      revisions: touch.revisions.map(toTouch),
    })),
  };
}

export function parseRoiTouch(data: unknown): RoiTouch | null {
  const parsed = parseWithFallback<z.infer<typeof touchSchema> | null>(data, touchSchema, null, { endpoint: "content-roi/touch" });
  return parsed ? toTouch(parsed) : null;
}

export function parseRoiDeal(data: unknown): RoiDeal | null {
  const parsed = parseWithFallback<z.infer<typeof dealSchema> | null>(data, dealSchema, null, { endpoint: "content-roi/deal" });
  return parsed ? toDeal(parsed) : null;
}

export function parseRoiDealList(data: unknown): RoiDeal[] {
  const parsed = parseWithFallback<z.infer<typeof dealListSchema>>(data, dealListSchema, { deals: [] }, { endpoint: "content-roi/deals" });
  return (parsed.deals ?? []).map(toDeal);
}

export function parseRoiDealDetail(data: unknown): RoiDealDetail | null {
  const parsed = parseWithFallback<z.infer<typeof dealDetailSchema> | null>(data, dealDetailSchema, null, { endpoint: "content-roi/deal-detail" });
  if (!parsed) return null;
  return {
    dealId: parsed.deal_id,
    current: toDeal(parsed.current),
    revisions: parsed.revisions.map(toDeal),
    adjustments: (parsed.adjustments ?? []).map((adjustment) => ({
      adjustmentId: adjustment.adjustment_id,
      current: toAdjustment(adjustment.current),
      revisions: adjustment.revisions.map(toAdjustment),
    })),
    attributions: (parsed.attributions ?? []).map(toAttribution),
  };
}

export function parseRoiAttribution(data: unknown): RoiAttribution | null {
  const parsed = parseWithFallback<z.infer<typeof attributionSchema> | null>(data, attributionSchema, null, { endpoint: "content-roi/attribution" });
  return parsed ? toAttribution(parsed) : null;
}

export function parseRoiAdjustment(data: unknown): RoiAdjustment | null {
  const parsed = parseWithFallback<z.infer<typeof adjustmentSchema> | null>(data, adjustmentSchema, null, { endpoint: "content-roi/adjustment" });
  return parsed ? toAdjustment(parsed) : null;
}

/** Builds a path under /api/content-roi with every segment encoded, so an id
 *  can never step into a neighbouring route. */
export function roiPath(...parts: string[]): string {
  return parts.map(encodeURIComponent).join("/");
}

// ---------------------------------------------------------------- report result (PR 2)
//
// A computed report, contract §5.2. The page shows the server's strings and
// does no arithmetic: every value, numerator, denominator and amount here is
// a string, and a response that sends one as a JSON number fails the schema.
// A status this build has not heard of reads as "not_computable" - never as
// a number shown on trust.

export type RoiMetricStatus = "ok" | "not_computable";

export interface RoiRecordRef {
  kind: string;
  id: string;
  revision: number;
  amountMinor: string | null;
  currency: string;
  convertedMinor: string | null;
}

export interface RoiMetric {
  status: RoiMetricStatus;
  /** Minor units, a count, or a reduced fraction ("2", "-1/2"). Not for display. */
  value: string;
  /** What the page shows, as the server wrote it ("200.00%", "10.00 倍", "−50.00%"). */
  display: string;
  unit: string;
  numerator: string;
  denominator: string;
  /** Set when not computable. */
  reason: string;
  formula: string;
  records: RoiRecordRef[];
}

export interface RoiBreakdownAmount {
  status: RoiMetricStatus;
  value: string;
  display: string;
  reason: string;
}

export interface RoiBreakdownRow {
  kind: string;
  id: string;
  attributedNetRevenue: RoiBreakdownAmount;
  attributedGrossProfit: RoiBreakdownAmount;
  /** The same deal can appear under several works; these do not add up. */
  dealsTouched: number;
}

export interface RoiResult {
  calcVersion: string;
  window: { start: string; end: string; timezone: string };
  reportCurrency: string;
  attributionMethod: string;
  generatedAt: string;
  metrics: Partial<Record<string, RoiMetric>>;
  breakdown: {
    byWork: RoiBreakdownRow[];
    byAccount: RoiBreakdownRow[];
    accountLevelUnknownWork: RoiBreakdownRow | null;
    brandLevelUnknownAccount: RoiBreakdownRow | null;
    unattributed: RoiBreakdownRow | null;
    evenSplitFallback: string[];
  };
  rules: string[];
}

const statusSchema = z.enum(["ok", "not_computable"]).catch("not_computable");

const recordRefSchema = z.object({
  kind: z.string(),
  id: z.string(),
  revision: z.number().int(),
  amount_minor: z.string().nullable().optional(),
  currency: text,
  converted_minor: z.string().nullable().optional(),
});

const metricSchema = z.object({
  status: statusSchema,
  value: text,
  display: text,
  unit: text,
  numerator: text,
  denominator: text,
  reason: text,
  formula: text,
  records: z.array(recordRefSchema).nullable().optional(),
});

const breakdownAmountSchema = z.object({
  status: statusSchema,
  value: text,
  display: text,
  reason: text,
});

const breakdownRowSchema = z.object({
  kind: text,
  id: text,
  attributed_net_revenue: breakdownAmountSchema,
  attributed_gross_profit: breakdownAmountSchema,
  deals_touched: z.number().int(),
});

const resultSchema = z.object({
  calc_version: z.string(),
  window: z.object({ start: text, end: text, timezone: text }),
  report_currency: text,
  attribution_method: text,
  generated_at: text,
  metrics: z.record(z.string(), metricSchema),
  breakdown: z.object({
    by_work: z.array(breakdownRowSchema).nullable().optional(),
    by_account: z.array(breakdownRowSchema).nullable().optional(),
    account_level_unknown_work: breakdownRowSchema.nullable().optional(),
    brand_level_unknown_account: breakdownRowSchema.nullable().optional(),
    unattributed: breakdownRowSchema.nullable().optional(),
    even_split_fallback: z.array(z.string()).nullable().optional(),
  }),
  rules: z.array(z.string()).nullable().optional(),
});

function toAmount(wire: z.infer<typeof breakdownAmountSchema>): RoiBreakdownAmount {
  // An amount that says ok but carries no value is not shown as zero.
  const status = wire.status === "ok" && wire.value ? "ok" : "not_computable";
  return { status, value: wire.value ?? "", display: wire.display ?? "", reason: wire.reason ?? "" };
}

function toRow(wire: z.infer<typeof breakdownRowSchema>): RoiBreakdownRow {
  return {
    kind: wire.kind ?? "",
    id: wire.id ?? "",
    attributedNetRevenue: toAmount(wire.attributed_net_revenue),
    attributedGrossProfit: toAmount(wire.attributed_gross_profit),
    dealsTouched: wire.deals_touched,
  };
}

function toMetric(wire: z.infer<typeof metricSchema>): RoiMetric {
  const status = wire.status === "ok" && wire.display ? "ok" : "not_computable";
  return {
    status,
    value: wire.value ?? "",
    display: status === "ok" ? (wire.display ?? "") : "",
    unit: wire.unit ?? "",
    numerator: wire.numerator ?? "",
    denominator: wire.denominator ?? "",
    reason: wire.reason ?? "",
    formula: wire.formula ?? "",
    records: (wire.records ?? []).map((record) => ({
      kind: record.kind,
      id: record.id,
      revision: record.revision,
      amountMinor: record.amount_minor ?? null,
      currency: record.currency ?? "",
      convertedMinor: record.converted_minor ?? null,
    })),
  };
}

/** A computed report (POST preview). null when the response is malformed. */
export function parseRoiResult(data: unknown): RoiResult | null {
  const parsed = parseWithFallback<z.infer<typeof resultSchema> | null>(data, resultSchema, null, { endpoint: "content-roi/result" });
  if (!parsed) return null;
  const metrics: Partial<Record<string, RoiMetric>> = {};
  for (const [id, metric] of Object.entries(parsed.metrics)) {
    metrics[id] = toMetric(metric);
  }
  const breakdown = parsed.breakdown;
  return {
    calcVersion: parsed.calc_version,
    window: {
      start: parsed.window.start ?? "",
      end: parsed.window.end ?? "",
      timezone: parsed.window.timezone ?? "",
    },
    reportCurrency: parsed.report_currency ?? "",
    attributionMethod: parsed.attribution_method ?? "",
    generatedAt: parsed.generated_at ?? "",
    metrics,
    breakdown: {
      byWork: (breakdown.by_work ?? []).map(toRow),
      byAccount: (breakdown.by_account ?? []).map(toRow),
      accountLevelUnknownWork: breakdown.account_level_unknown_work ? toRow(breakdown.account_level_unknown_work) : null,
      brandLevelUnknownAccount: breakdown.brand_level_unknown_account ? toRow(breakdown.brand_level_unknown_account) : null,
      unattributed: breakdown.unattributed ? toRow(breakdown.unattributed) : null,
      evenSplitFallback: breakdown.even_split_fallback ?? [],
    },
    rules: parsed.rules ?? [],
  };
}

// ---------------------------------------------------------------- import (PR 3)
//
// An import batch, contract §1.8: what a paste was, and what happened to each
// row. Counts are plain numbers (they are row counts, not money); every id is
// a string. A row outcome this build has not heard of reads as "unknown" -
// never as "written", which would claim a record exists.

/** What one import holds (Go: ImportRecordKinds). */
export const ROI_IMPORT_KINDS = ["cost", "lead", "deal"] as const;
export type RoiImportKind = (typeof ROI_IMPORT_KINDS)[number];

/** What happened to one row (Go: ImportOutcomes). */
export const ROI_IMPORT_OUTCOMES = ["written", "duplicate", "confirmed_not_duplicate"] as const;
export type RoiImportOutcome = (typeof ROI_IMPORT_OUTCOMES)[number] | "unknown";

export interface RoiImportRow {
  /** 1-based, counting data rows. */
  row: number;
  outcome: RoiImportOutcome;
  /** The record the row wrote; "" when it was held back or in a dry run. */
  recordId: string;
  /** The current records the row matched. */
  duplicateOf: string[];
  /** Dry run only: earlier rows of the same paste it matched. */
  duplicateOfRows: number[];
}

export interface RoiImportResult {
  dryRun: boolean;
  /** "" for a dry run: nothing was stored. */
  importBatchId: string;
  recordKind: string;
  rowCount: number;
  writtenCount: number;
  skippedCount: number;
  rows: RoiImportRow[];
  recordedBy: string;
  /** null for a dry run. */
  createdAt: string | null;
}

export interface RoiImportBatch {
  importBatchId: string;
  recordKind: string;
  rowCount: number;
  writtenCount: number;
  skippedCount: number;
  rows: RoiImportRow[];
  idempotencyKey: string;
  recordedBy: string;
  createdAt: string;
}

const count = z.number().int().nonnegative();

const importRowSchema = z.object({
  row: z.number().int().positive(),
  outcome: z.enum(ROI_IMPORT_OUTCOMES).or(z.literal("unknown")).catch("unknown"),
  record_id: z.string(),
  duplicate_of: z.array(z.string()).nullable().optional(),
  duplicate_of_rows: z.array(z.number().int().positive()).nullable().optional(),
});

const importResultSchema = z.object({
  dry_run: z.boolean(),
  import_batch_id: z.string(),
  record_kind: z.string(),
  row_count: count,
  written_count: count,
  skipped_count: count,
  rows: z.array(importRowSchema),
  recorded_by: text,
  created_at: z.string().nullable().optional(),
});

const importBatchSchema = z.object({
  import_batch_id: z.string(),
  record_kind: z.string(),
  row_count: count,
  written_count: count,
  skipped_count: count,
  rows: z.array(importRowSchema),
  idempotency_key: text,
  recorded_by: text,
  created_at: text,
});

const importListSchema = z.object({ imports: z.array(importBatchSchema).nullable().optional() });

function toImportRow(wire: z.infer<typeof importRowSchema>): RoiImportRow {
  return {
    row: wire.row,
    outcome: wire.outcome,
    recordId: wire.record_id,
    duplicateOf: wire.duplicate_of ?? [],
    duplicateOfRows: wire.duplicate_of_rows ?? [],
  };
}

function toImportBatch(wire: z.infer<typeof importBatchSchema>): RoiImportBatch {
  return {
    importBatchId: wire.import_batch_id,
    recordKind: wire.record_kind,
    rowCount: wire.row_count,
    writtenCount: wire.written_count,
    skippedCount: wire.skipped_count,
    rows: wire.rows.map(toImportRow),
    idempotencyKey: wire.idempotency_key ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

/** POST imports' answer, or null when it does not fit the contract. */
export function parseRoiImportResult(data: unknown): RoiImportResult | null {
  const parsed = parseWithFallback<z.infer<typeof importResultSchema> | null>(data, importResultSchema, null, { endpoint: "content-roi/import" });
  if (!parsed) return null;
  return {
    dryRun: parsed.dry_run,
    importBatchId: parsed.import_batch_id,
    recordKind: parsed.record_kind,
    rowCount: parsed.row_count,
    writtenCount: parsed.written_count,
    skippedCount: parsed.skipped_count,
    rows: parsed.rows.map(toImportRow),
    recordedBy: parsed.recorded_by ?? "",
    createdAt: parsed.created_at ?? null,
  };
}

export function parseRoiImportBatch(data: unknown): RoiImportBatch | null {
  const parsed = parseWithFallback<z.infer<typeof importBatchSchema> | null>(data, importBatchSchema, null, { endpoint: "content-roi/import-batch" });
  return parsed ? toImportBatch(parsed) : null;
}

export function parseRoiImportList(data: unknown): RoiImportBatch[] {
  const parsed = parseWithFallback<z.infer<typeof importListSchema>>(data, importListSchema, { imports: [] }, { endpoint: "content-roi/imports" });
  return (parsed.imports ?? []).map(toImportBatch);
}
