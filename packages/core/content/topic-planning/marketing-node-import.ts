import type { LeadDays, MarketingNodeInput } from "./marketing-nodes";

// Pasted node list -> import rows (specs/033 US4, contract §2.1 and §6).
//
// The server only accepts JSON; this is the one place CSV is read. It checks
// what a row must look like - the column count, the dates, a whole-number lead
// time - and nothing else. It does not guess: no kind from the name, no year
// for a date without one, no time zone from anywhere but the row or the
// default the caller passes (the brand's zone).

export const MARKETING_NODE_CSV_COLUMNS = [
  "name",
  "kind",
  "starts_on",
  "ends_on",
  "timezone",
  "lead_days",
  "goal",
  "date_certainty",
  "date_basis",
] as const;
export type MarketingNodeCsvColumn = (typeof MARKETING_NODE_CSV_COLUMNS)[number];

const REQUIRED_COLUMNS: readonly MarketingNodeCsvColumn[] = [
  "name",
  "kind",
  "starts_on",
  "ends_on",
  "date_certainty",
];

export type CsvRowError =
  | { reason: "column_count"; expected: number; actual: number }
  | { reason: "invalid"; column: MarketingNodeCsvColumn };

export type ParsedCsvRow =
  | { line: number; ok: true; input: MarketingNodeInput }
  | { line: number; ok: false; error: CsvRowError };

export type ParsedCsv =
  | { ok: true; rows: ParsedCsvRow[] }
  | { ok: false; error: "empty" | "missing_columns"; missing: string[] };

/**
 * Split CSV text into records of fields. Double-quoted fields may contain
 * commas, newlines and doubled quotes. Line numbers are the 1-based line each
 * record starts on, so an error can point at the pasted text.
 */
export function splitCsv(text: string): { line: number; fields: string[] }[] {
  const source = text.startsWith("﻿") ? text.slice(1) : text;
  const records: { line: number; fields: string[] }[] = [];
  let fields: string[] = [];
  let field = "";
  let quoted = false;
  let line = 1;
  let recordLine = 1;
  let touched = false;

  const endRecord = () => {
    fields.push(field);
    const blank = fields.length === 1 && fields[0]?.trim() === "" && !touched;
    if (!blank) records.push({ line: recordLine, fields });
    fields = [];
    field = "";
    touched = false;
  };

  for (let i = 0; i < source.length; i++) {
    const ch = source[i];
    if (quoted) {
      if (ch === '"') {
        if (source[i + 1] === '"') {
          field += '"';
          i++;
        } else {
          quoted = false;
        }
      } else {
        if (ch === "\n") line++;
        field += ch;
      }
      continue;
    }
    if (ch === '"') {
      quoted = true;
      touched = true;
    } else if (ch === ",") {
      fields.push(field);
      field = "";
      touched = true;
    } else if (ch === "\r" || ch === "\n") {
      if (ch === "\r" && source[i + 1] === "\n") i++;
      endRecord();
      line++;
      recordLine = line;
    } else {
      field += ch;
    }
  }
  if (field !== "" || fields.length > 0 || touched) endRecord();
  return records;
}

function isCalendarDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [y, m, d] = value.split("-").map(Number) as [number, number, number];
  const date = new Date(Date.UTC(y, m - 1, d));
  return (
    date.getUTCFullYear() === y &&
    date.getUTCMonth() === m - 1 &&
    date.getUTCDate() === d
  );
}

/** "" is "not set"; "0" is zero; anything else must be a whole number. */
export function parseLeadDaysCell(value: string): LeadDays | null {
  const trimmed = value.trim();
  if (trimmed === "") return { set: false };
  if (!/^\d+$/.test(trimmed)) return null;
  return { set: true, value: Number(trimmed) };
}

/**
 * Parse pasted CSV with a header row. Columns may come in any order; name,
 * kind, starts_on, ends_on and date_certainty are required, the rest are
 * optional. Every data row yields either an input or the one reason it was
 * refused.
 */
export function parseMarketingNodeCsv(
  text: string,
  defaults: { timezone: string },
): ParsedCsv {
  const records = splitCsv(text);
  const header = records[0];
  if (!header) return { ok: false, error: "empty", missing: [] };
  const names = header.fields.map((name) => name.trim());
  const missing = REQUIRED_COLUMNS.filter((column) => !names.includes(column));
  if (missing.length > 0) return { ok: false, error: "missing_columns", missing };

  const index = new Map<string, number>();
  names.forEach((name, i) => {
    if (!index.has(name)) index.set(name, i);
  });

  const rows: ParsedCsvRow[] = records.slice(1).map(({ line, fields }) => {
    if (fields.length !== names.length) {
      return {
        line,
        ok: false,
        error: { reason: "column_count", expected: names.length, actual: fields.length },
      };
    }
    const cell = (column: MarketingNodeCsvColumn) => {
      const i = index.get(column);
      return i === undefined ? "" : (fields[i] ?? "").trim();
    };
    const invalid = (column: MarketingNodeCsvColumn): ParsedCsvRow => ({
      line,
      ok: false,
      error: { reason: "invalid", column },
    });

    const startsOn = cell("starts_on");
    if (!isCalendarDate(startsOn)) return invalid("starts_on");
    const endsOn = cell("ends_on");
    if (!isCalendarDate(endsOn)) return invalid("ends_on");
    const leadDays = parseLeadDaysCell(cell("lead_days"));
    if (leadDays === null) return invalid("lead_days");

    return {
      line,
      ok: true,
      input: {
        name: cell("name"),
        kind: cell("kind"),
        startsOn,
        endsOn,
        timezone: cell("timezone") || defaults.timezone,
        leadDays,
        accounts: [],
        goal: cell("goal"),
        materialSourceIds: [],
        dateCertainty: cell("date_certainty"),
        dateBasis: cell("date_basis"),
      },
    };
  });
  return { ok: true, rows };
}
