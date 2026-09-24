import { splitLine } from "../csv";
import { ROI_CURRENCIES, ROI_GROSS_BASES, ROI_PRICINGS, type RoiImportKind } from "./contract";

// Turning a pasted block of costs, leads or deals into import rows (specs/034
// PR 3, FR-027). A paste, not a file upload, as in 027's csv.ts.
//
// Three rules this file keeps:
//
// - Every amount stays a STRING, exactly as typed ("3000.00"). This file never
//   makes a number of one; the server parses it into minor units, and refuses
//   it naming the row if it is not a valid amount for the currency.
// - An empty cell is absent, never 0. An empty labor rate is "no rate yet",
//   which the server stores as not computable; an empty gross profit is "not
//   given".
// - The first bad row stops the whole paste and says which row and column,
//   counting data rows (the header is not row 1). The server checks again and
//   is the authority; this is so a person sees the problem before sending.

/** The columns each kind's paste may carry, in the order a paste with no
 *  header line is read. Names are the server's field names. */
export const ROI_IMPORT_COLUMNS = {
  cost: [
    "category", "pricing", "amount", "currency", "labor_minutes", "labor_rate", "incurred_at",
    "ad_spend", "account_id", "work_id", "campaign_label", "evidence_note", "note",
  ],
  lead: ["customer_ref", "stage", "qualified", "first_seen_at", "note"],
  deal: [
    "lead_id", "order_ref", "amount", "currency", "closed_at", "gross_basis", "gross_profit",
    "cogs", "note",
  ],
} as const satisfies Record<RoiImportKind, readonly string[]>;

/** Columns a row may not leave empty. */
const REQUIRED: Record<RoiImportKind, readonly string[]> = {
  cost: ["category", "pricing", "currency", "incurred_at"],
  lead: ["first_seen_at"],
  deal: ["amount", "currency", "closed_at", "gross_basis"],
};

/** Amount columns: kept as typed, left out when empty. */
const AMOUNTS = new Set(["amount", "labor_rate", "gross_profit", "cogs"]);
const BOOLEANS = new Set(["ad_spend", "qualified"]);
const SETS: Record<string, readonly string[]> = {
  pricing: ROI_PRICINGS,
  gross_basis: ROI_GROSS_BASES,
  currency: ROI_CURRENCIES.map((currency) => currency.code),
};

const TRUE = new Set(["true", "1", "是", "yes"]);
const FALSE = new Set(["false", "0", "否", "no", ""]);

export interface RoiCsvProblem {
  /** 1-based, counting data rows. */
  row: number;
  column: string;
  reason: "unknown-column" | "missing" | "outside-the-set" | "not-a-whole-number" | "not-yes-or-no";
}

/** One row in the server's field names, ready to send. */
export type RoiImportRowBody = Record<string, string | number | boolean>;

export type RoiCsvParse =
  | { ok: true; rows: RoiImportRowBody[] }
  | { ok: false; problem: RoiCsvProblem };

/**
 * Reads a pasted block into import rows, or reports the first problem.
 *
 * A first line whose cells are all column names of this kind is a header and
 * decides the order; one that names a column this kind does not have is
 * refused, naming it. Otherwise the kind's column order is used.
 */
export function parsePastedRoiRows(kind: RoiImportKind, text: string): RoiCsvParse {
  const lines = text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== "");
  if (lines.length === 0) return { ok: true, rows: [] };

  const columns: readonly string[] = ROI_IMPORT_COLUMNS[kind];
  const known = new Set(columns);
  let order: readonly string[] = columns;
  let start = 0;
  const firstCells = splitLine(lines[0]!);
  const looksLikeHeader = firstCells.some((cell) => known.has(cell));
  if (looksLikeHeader) {
    const unknown = firstCells.find((cell) => !known.has(cell));
    if (unknown !== undefined) {
      return { ok: false, problem: { row: 0, column: unknown, reason: "unknown-column" } };
    }
    order = firstCells;
    start = 1;
  }

  const rows: RoiImportRowBody[] = [];
  for (let index = start; index < lines.length; index += 1) {
    const row = index - start + 1;
    const cells = splitLine(lines[index]!);
    const body: RoiImportRowBody = {};
    for (let position = 0; position < order.length; position += 1) {
      const column = order[position]!;
      const cell = (cells[position] ?? "").trim();
      if (cell === "") {
        if (REQUIRED[kind].includes(column)) {
          return { ok: false, problem: { row, column, reason: "missing" } };
        }
        if (BOOLEANS.has(column)) body[column] = false;
        // Empty amount and labor_minutes cells are left out: unknown, not 0.
        else if (!AMOUNTS.has(column) && column !== "labor_minutes") body[column] = "";
        continue;
      }
      const allowed = SETS[column];
      if (allowed && !allowed.includes(cell)) {
        return { ok: false, problem: { row, column, reason: "outside-the-set" } };
      }
      if (column === "labor_minutes") {
        if (!/^\d+$/.test(cell)) {
          return { ok: false, problem: { row, column, reason: "not-a-whole-number" } };
        }
        // A count of minutes, not money; the server bounds it.
        body[column] = Number(cell);
        continue;
      }
      if (BOOLEANS.has(column)) {
        const lower = cell.toLowerCase();
        if (!TRUE.has(lower) && !FALSE.has(lower)) {
          return { ok: false, problem: { row, column, reason: "not-yes-or-no" } };
        }
        body[column] = TRUE.has(lower);
        continue;
      }
      body[column] = cell;
    }
    for (const column of REQUIRED[kind]) {
      if (!(column in body)) {
        return { ok: false, problem: { row, column, reason: "missing" } };
      }
    }
    rows.push(body);
  }
  return { ok: true, rows };
}

/**
 * The POST imports body. confirmations maps a 1-based row to the record ids
 * the person said it is NOT a duplicate of; only those rows carry
 * not_duplicate_of.
 */
export function toRoiImportBody(
  kind: RoiImportKind,
  rows: RoiImportRowBody[],
  options: { dryRun?: boolean; confirmations?: Record<number, string[]> } = {},
): Record<string, unknown> {
  return {
    record_kind: kind,
    dry_run: options.dryRun ?? false,
    rows: rows.map((row, index) => {
      const confirmed = options.confirmations?.[index + 1];
      return confirmed && confirmed.length > 0 ? { ...row, not_duplicate_of: [...confirmed] } : { ...row };
    }),
  };
}
