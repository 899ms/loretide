import { roiCurrencyDigits } from "./display";
import type { RoiAdjustment, RoiDeal } from "./contract";

// Rules the ROI review page's forms apply before sending (specs/034 PR 5).
// The server applies every one of them again and is the authority; these are
// here so a button can be disabled with a reason instead of the person
// finding out from a 400.
//
// Money stays exact: an amount is read into integer minor units with BigInt,
// never into a JS number, and written back as a decimal string. The only sum
// taken here is "do the shares a person typed add up to the cost" (FR-031) -
// no report metric is computed on the page (FR-040).

// ---------------------------------------------------------------- touches (FR-019)

export type RoiTouchFieldRule = "required" | "optional" | "forbidden";

export interface RoiTouchRules {
  /** Work or publication record: "required" means at least one of the two. */
  content: RoiTouchFieldRule;
  account: RoiTouchFieldRule;
  platform: RoiTouchFieldRule;
}

/** FR-019's table, one row per evidence type. An unknown type allows
 *  nothing to be required and leaves the server to name the problem. */
export function roiTouchRules(evidenceType: string): RoiTouchRules {
  switch (evidenceType) {
    case "platform_linked_content":
    case "content_comment":
      return { content: "required", account: "optional", platform: "required" };
    case "account_only":
      return { content: "forbidden", account: "required", platform: "required" };
    case "unknown":
      return { content: "forbidden", account: "forbidden", platform: "forbidden" };
    default:
      return { content: "optional", account: "optional", platform: "optional" };
  }
}

export interface RoiTouchFields {
  evidenceType: string;
  platform: string;
  accountId: string;
  workId: string;
  publicationRecordId: string;
}

/** The fields FR-019 says are wrong for this evidence type, in form order. */
export function roiTouchProblems(fields: RoiTouchFields): ("work_id" | "account_id" | "platform")[] {
  const rules = roiTouchRules(fields.evidenceType);
  const problems: ("work_id" | "account_id" | "platform")[] = [];
  const hasContent = fields.workId !== "" || fields.publicationRecordId !== "";
  if ((rules.content === "required" && !hasContent) || (rules.content === "forbidden" && hasContent)) {
    problems.push("work_id");
  }
  const hasAccount = fields.accountId !== "";
  if ((rules.account === "required" && !hasAccount) || (rules.account === "forbidden" && hasAccount)) {
    problems.push("account_id");
  }
  const hasPlatform = fields.platform !== "";
  if ((rules.platform === "required" && !hasPlatform) || (rules.platform === "forbidden" && hasPlatform)) {
    problems.push("platform");
  }
  return problems;
}

/** Clears what a newly chosen evidence type forbids, so a hidden value is
 *  never sent. */
export function roiTouchForEvidence<T extends RoiTouchFields>(fields: T, evidenceType: string): T {
  const rules = roiTouchRules(evidenceType);
  return {
    ...fields,
    evidenceType,
    workId: rules.content === "forbidden" ? "" : fields.workId,
    publicationRecordId: rules.content === "forbidden" ? "" : fields.publicationRecordId,
    accountId: rules.account === "forbidden" ? "" : fields.accountId,
    platform: rules.platform === "forbidden" ? "" : fields.platform,
  };
}

// ---------------------------------------------------------------- exact amounts

/** "3000.00" in CNY is 300000n. null when it is not a plain decimal with at
 *  most the currency's places (the server refuses those; nothing is rounded). */
export function roiMinorOf(text: string, currency: string): bigint | null {
  const digits = roiCurrencyDigits(currency);
  const match = /^(-?)(\d+)(?:\.(\d+))?$/.exec(text.trim());
  if (digits === null || !match) return null;
  const fraction = match[3] ?? "";
  if (fraction.length > digits) return null;
  const minor = BigInt(match[2]! + fraction.padEnd(digits, "0"));
  return match[1] === "-" ? -minor : minor;
}

/** 300000n in CNY is "3000.00". */
export function roiDecimalOf(minor: bigint, currency: string): string {
  const digits = roiCurrencyDigits(currency) ?? 0;
  const negative = minor < 0n;
  const text = (negative ? -minor : minor).toString().padStart(digits + 1, "0");
  const whole = digits === 0 ? text : text.slice(0, -digits);
  const fraction = digits === 0 ? "" : `.${text.slice(-digits)}`;
  return `${negative ? "-" : ""}${whole}${fraction}`;
}

/** A minor-unit string from the server ("300000") written in the
 *  currency's decimal form ("3000.00"). Anything else is returned as is. */
export function roiMinorText(minor: string, currency: string): string {
  if (!/^-?\d+$/.test(minor) || roiCurrencyDigits(currency) === null) return minor;
  return roiDecimalOf(BigInt(minor), currency);
}

export interface RoiShareCheck {
  /** The shares' total, as a decimal string. */
  total: string;
  /** The cost's own amount. */
  original: string;
  /** original − total; "0.00" when they agree. */
  gap: string;
  matches: boolean;
}

/**
 * Shares typed as amounts (method "amounts") against the cost. null when the
 * cost amount or any share is not a valid amount yet - the form names that
 * separately. FR-031: they must add up exactly, or the server refuses.
 */
export function roiCheckShares(shares: string[], original: string, currency: string): RoiShareCheck | null {
  const originalMinor = roiMinorOf(original, currency);
  if (originalMinor === null || shares.length === 0) return null;
  let total = 0n;
  for (const share of shares) {
    const minor = roiMinorOf(share, currency);
    if (minor === null) return null;
    total += minor;
  }
  return {
    total: roiDecimalOf(total, currency),
    original: roiDecimalOf(originalMinor, currency),
    gap: roiDecimalOf(originalMinor - total, currency),
    matches: total === originalMinor,
  };
}

/** The stored shares of one revision (server figures, minor-unit strings)
 *  against its amount - shown beside the shares so a person can see they
 *  add up. null when the cost has no amount (not computable). */
export function roiCheckStoredShares(
  allocatedMinor: string[],
  amountMinor: string | null,
  currency: string,
): RoiShareCheck | null {
  if (amountMinor === null || allocatedMinor.length === 0) return null;
  if (!/^-?\d+$/.test(amountMinor) || !allocatedMinor.every((minor) => /^-?\d+$/.test(minor))) return null;
  const total = allocatedMinor.reduce((sum, minor) => sum + BigInt(minor), 0n);
  const original = BigInt(amountMinor);
  return {
    total: roiDecimalOf(total, currency),
    original: roiDecimalOf(original, currency),
    gap: roiDecimalOf(original - total, currency),
    matches: total === original,
  };
}

// ---------------------------------------------------------------- a deal's gross profit

export type RoiDealGrossState =
  /** gross_basis "none": the deal has no gross profit to state. */
  | { kind: "no_basis" }
  /** A refund or adjustment in effect did not say how gross profit moved (FR-025). */
  | { kind: "refund_without_gross_delta"; adjustmentIds: string[] }
  /** Stated directly and nothing moves it: the stated figure is the gross profit. */
  | { kind: "stated"; grossProfit: string }
  /** Anything else is computed by the server in a report, not on this page. */
  | { kind: "in_report" };

/** Which gross-profit line a deal's detail shows. Reads statuses and the
 *  server's strings; adds nothing up. */
export function roiDealGrossState(deal: RoiDeal, adjustments: RoiAdjustment[]): RoiDealGrossState {
  if (deal.grossBasis === "none") return { kind: "no_basis" };
  const active = adjustments.filter((adjustment) => !adjustment.voided);
  const missing = active.filter((adjustment) => adjustment.grossDeltaMinor === null);
  if (missing.length > 0) {
    return { kind: "refund_without_gross_delta", adjustmentIds: missing.map((item) => item.adjustmentId) };
  }
  if (deal.grossBasis === "stated_gross_profit" && active.length === 0 && deal.grossProfit !== null) {
    return { kind: "stated", grossProfit: deal.grossProfit };
  }
  return { kind: "in_report" };
}
