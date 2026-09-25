import { z } from "zod";
import { ApiError } from "@multica/core/api";
import { parseWithFallback } from "@multica/core/api/schema";
import { ROI_CURRENCIES, ROI_METRIC_IDS, ROI_REASON_CODES, type RoiMetric, type RoiMetricId } from "./contract";

// What the ROI review page shows for a server value, as a choice of i18n key
// (specs/034 PR 5, T085). No arithmetic happens here: a metric's `display` is
// passed through exactly as the server wrote it, and `value`, `numerator` and
// `denominator` are never read. The functions below only look at a status, a
// reason code, or the characters of a string.
//
// Three rules this file keeps:
//
// - "Not computable" is never shown as 0, blank, ∞ or NaN. A metric that is
//   not ok, or that says ok without a display string, is not computable.
// - A reason code this build has not heard of falls to "default" (a plain
//   "not computable"), never to a made-up reason.
// - A signed gross-profit change is stored with its sign (a reduction of 300
//   is "-300.00", contract §1.6 and §5.5). The page lets a person pick
//   "reduction" and type 300; this file turns that into "-300" and reads the
//   sign back, by adding or reading a leading minus sign, nothing more.

export type RoiReasonKey = (typeof ROI_REASON_CODES)[number] | "default";

/** The i18n key for a not-computable reason code. */
export function roiReasonKey(reason: string): RoiReasonKey {
  return (ROI_REASON_CODES as readonly string[]).includes(reason) ? (reason as RoiReasonKey) : "default";
}

export type RoiMetricView =
  | { kind: "value"; display: string; negative: boolean }
  | { kind: "not_computable"; reason: RoiReasonKey };

/** Starts with a minus sign: U+2212 as the calculator writes it, or ASCII. */
export function isNegativeDisplay(display: string): boolean {
  const first = display.trimStart().charAt(0);
  return first === "−" || first === "-";
}

/**
 * How one metric is shown. `negative` is read off the display string's first
 * character so the page can use the secondary text colour for a negative
 * return (FR-082) - the number itself is still shown as the server wrote it.
 */
export function roiMetricView(metric: RoiMetric | undefined | null): RoiMetricView {
  if (!metric || metric.status !== "ok" || metric.display === "") {
    return { kind: "not_computable", reason: roiReasonKey(metric?.reason ?? "") };
  }
  return { kind: "value", display: metric.display, negative: isNegativeDisplay(metric.display) };
}

/** The same choice for one breakdown amount (by work, by account, ...). */
export function roiAmountView(amount: { status: string; display: string; reason: string } | undefined | null): RoiMetricView {
  if (!amount || amount.status !== "ok" || amount.display === "") {
    return { kind: "not_computable", reason: roiReasonKey(amount?.reason ?? "") };
  }
  return { kind: "value", display: amount.display, negative: isNegativeDisplay(amount.display) };
}

export type RoiMetricLabelKey = RoiMetricId | "default";

/** The i18n key for a metric id. business_roi, revenue_to_spend and ad_roas
 *  are three different keys and never stand in for each other. */
export function roiMetricLabelKey(id: string): RoiMetricLabelKey {
  return (ROI_METRIC_IDS as readonly string[]).includes(id) ? (id as RoiMetricId) : "default";
}

// ---------------------------------------------------------------- amounts typed on the page

export type RoiAmountProblem = "empty" | "format" | "too_many_decimals" | "unknown_currency";

/**
 * Checks a typed amount against a currency's minor-unit places before it is
 * sent. The server checks again and is the authority (FR-003); this is only
 * so the button can be disabled with a reason. The amount stays a string and
 * is never rounded.
 */
export function roiAmountProblem(
  text: string,
  currency: string,
  options: { signed?: boolean } = {},
): RoiAmountProblem | null {
  const value = text.trim();
  if (value === "") return "empty";
  const digits = roiCurrencyDigits(currency);
  if (digits === null) return "unknown_currency";
  const pattern = options.signed ? /^-?\d+(\.(\d+))?$/ : /^\d+(\.(\d+))?$/;
  const match = pattern.exec(value);
  if (!match) return "format";
  const decimals = match[2]?.length ?? 0;
  if (decimals > digits) return "too_many_decimals";
  return null;
}

/** A currency's number of decimal places, or null when it is not one of the nine. */
export function roiCurrencyDigits(currency: string): number | null {
  return ROI_CURRENCIES.find((item) => item.code === currency)?.digits ?? null;
}

// ---------------------------------------------------------------- the signed gross-profit change

export type RoiGrossDeltaDirection = "reduction" | "increase" | "not_given";

/**
 * The gross_delta to send. The person picks a direction and types an
 * unsigned amount: "reduction" + "300" is "-300", "increase" + "300" is
 * "300", "not_given" is null (absent, never 0).
 */
export function signedGrossDelta(direction: RoiGrossDeltaDirection, amount: string): string | null {
  if (direction === "not_given") return null;
  const value = amount.trim();
  return direction === "reduction" ? `-${value}` : value;
}

export type RoiGrossDeltaShown = "reduction" | "increase" | "zero" | "not_given";

/** Which label goes beside a stored gross_delta, read off its characters. */
export function grossDeltaShown(value: string | null): RoiGrossDeltaShown {
  if (value === null || value.trim() === "") return "not_given";
  if (isNegativeDisplay(value)) return "reduction";
  return /^\+?0*(\.0*)?$/.test(value.trim()) ? "zero" : "increase";
}

// ---------------------------------------------------------------- what happened to a write

/**
 * A failed write, in the page's vocabulary. Each kind asks the person for
 * something different: `stale` - someone else saved first, reload; 
 * `duplicate` - look at these records and confirm or stop; `invalid` - fix
 * this field (and, in an import, this row); `not_found` - the record is not
 * there or not yours, answered exactly alike (contract §6).
 */
export type RoiWriteOutcome =
  | { kind: "saved" }
  | { kind: "stale" }
  | { kind: "duplicate"; matches: string[] }
  | { kind: "idempotency" }
  | { kind: "invalid"; field: string; reason: string; row: number | null }
  | { kind: "not_found" }
  | { kind: "failed"; nextAction: string; traceId: string };

export const ROI_SAVED: RoiWriteOutcome = { kind: "saved" };

const roiErrorBodySchema = z.object({
  code: z.string().optional(),
  field: z.string().optional(),
  reason: z.string().optional(),
  row: z.number().int().optional(),
  matches: z.array(z.string()).nullable().optional(),
  next_action: z.string().optional(),
  trace_id: z.string().optional(),
});

/** Never throws: the caller is already on a failure path. */
export function roiWriteOutcome(error: unknown): RoiWriteOutcome {
  if (!(error instanceof ApiError)) return { kind: "failed", nextAction: "", traceId: "" };
  const body = parseWithFallback<z.infer<typeof roiErrorBodySchema>>(
    error.body ?? {},
    roiErrorBodySchema,
    {},
    { endpoint: "content-roi/error" },
  );
  if (error.status === 409) {
    if (body.code === "possible_duplicate") return { kind: "duplicate", matches: body.matches ?? [] };
    if (body.field === "Idempotency-Key") return { kind: "idempotency" };
    if (body.field === "base_revision") return { kind: "stale" };
  }
  if (error.status === 400 && body.field) {
    return { kind: "invalid", field: body.field, reason: body.reason ?? "", row: body.row ?? null };
  }
  if (error.status === 404 || error.status === 403) return { kind: "not_found" };
  return { kind: "failed", nextAction: body.next_action ?? "", traceId: body.trace_id ?? "" };
}
