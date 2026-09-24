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
  /** null: the operator did not say how gross profit moved. Not 0. */
  grossDeltaMinor: string | null;
  grossDelta: string | null;
  currency: string;
  occurredAt: string;
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
}

// Lenient where the page only reads (a controlled value this build has not
// heard of stays a string), strict where it matters: every amount is a
// string or null, never a number.
const amount = z.string();
const optionalAmount = z.string().nullable().optional();
const text = z.string().optional();
const flag = z.boolean().optional();
const ids = z.array(z.string()).nullable().optional();

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
  };
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
