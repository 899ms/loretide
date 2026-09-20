import { METRIC_NAMES, METRIC_PLATFORMS, type ManualMetric } from "./contract";

// Turning a pasted CSV into rows (SOP 10.1: "首版采用表单与 CSV 导入").
//
// This phase is a PASTE, not a file upload: the person copies the cells out of
// a spreadsheet and drops them in. File upload arrives with W-03, and nothing
// here touches a file.
//
// Two rules the shape of this file exists to keep:
//
//   - An empty cell parses to `null`, never to 0. SOP 10.1: "未知填空；0 只表示
//     已确认的零值". A `Number("")` is 0 in JavaScript, which is exactly the
//     silent corruption this card is most exposed to.
//   - All or nothing. The first bad row stops the whole paste and the error
//     says which line - "some row is wrong" in a paste of forty rows is not an
//     answer anybody can act on.

/** The columns a pasted row carries, in order. The header line may name them
 *  in any order; a paste with no header is read in this order. */
export const CSV_COLUMNS = [
  "platform", "account_id", "metric", "value", "unit", "stat_window", "sampled_at",
] as const;

export type CsvColumn = (typeof CSV_COLUMNS)[number];

export interface CsvRow {
  platform: string;
  accountId: string;
  metric: string;
  /** null when the cell was empty: that is "the platform does not show me
   *  this", and it is not zero. */
  value: number | null;
  unit: string;
  statWindow: string;
  sampledAt: string;
}

export interface CsvProblem {
  /** 1-based, counting data rows - the header is not row 1. */
  row: number;
  column: string;
  reason: "unknown-column" | "not-a-number" | "outside-the-set" | "missing";
}

export type CsvParse =
  | { ok: true; rows: CsvRow[] }
  | { ok: false; problem: CsvProblem };

/** Splits one CSV line, honouring double quotes so a quoted comma stays in its
 *  cell. Not a general CSV library: a pasted spreadsheet selection is the
 *  input, and this is what that produces. */
function splitLine(line: string): string[] {
  const cells: string[] = [];
  let current = "";
  let quoted = false;
  for (let index = 0; index < line.length; index += 1) {
    const character = line[index];
    if (character === '"') {
      if (quoted && line[index + 1] === '"') {
        current += '"';
        index += 1;
      } else {
        quoted = !quoted;
      }
      continue;
    }
    if ((character === "," || character === "\t") && !quoted) {
      cells.push(current);
      current = "";
      continue;
    }
    current += character;
  }
  cells.push(current);
  return cells.map((cell) => cell.trim());
}

/**
 * Reads a pasted block into rows, or reports the first problem.
 *
 * A first line whose cells are all known column names is taken as a header and
 * decides the order; otherwise CSV_COLUMNS is the order.
 */
export function parsePastedMetrics(text: string): CsvParse {
  const lines = text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== "");
  if (lines.length === 0) return { ok: true, rows: [] };

  let order: string[] = [...CSV_COLUMNS];
  let start = 0;
  const firstCells = splitLine(lines[0]!);
  const known = new Set<string>(CSV_COLUMNS);
  if (firstCells.every((cell) => known.has(cell))) {
    order = firstCells;
    start = 1;
  }

  const rows: CsvRow[] = [];
  for (let index = start; index < lines.length; index += 1) {
    const cells = splitLine(lines[index]!);
    const row = index - start + 1;
    const byColumn: Record<string, string> = {};
    for (let column = 0; column < order.length; column += 1) {
      byColumn[order[column]!] = cells[column] ?? "";
    }

    const platform = byColumn.platform ?? "";
    if (!(METRIC_PLATFORMS as readonly string[]).includes(platform)) {
      return { ok: false, problem: { row, column: "platform", reason: platform ? "outside-the-set" : "missing" } };
    }
    const metric = byColumn.metric ?? "";
    if (!(METRIC_NAMES as readonly string[]).includes(metric)) {
      return { ok: false, problem: { row, column: "metric", reason: metric ? "outside-the-set" : "missing" } };
    }
    const sampledAt = byColumn.sampled_at ?? "";
    if (!sampledAt) {
      return { ok: false, problem: { row, column: "sampled_at", reason: "missing" } };
    }

    const raw = (byColumn.value ?? "").trim();
    let value: number | null = null;
    if (raw !== "") {
      // Number("") is 0, so the empty case is handled above and never reaches
      // here. This branch only ever sees something somebody typed.
      const parsed = Number(raw.replace(/,/g, ""));
      if (!Number.isFinite(parsed)) {
        return { ok: false, problem: { row, column: "value", reason: "not-a-number" } };
      }
      value = parsed;
    }

    rows.push({
      platform,
      accountId: byColumn.account_id ?? "",
      metric,
      value,
      unit: byColumn.unit ?? "",
      statWindow: byColumn.stat_window ?? "",
      sampledAt,
    });
  }
  return { ok: true, rows };
}

/** Turns parsed rows into what the import endpoint takes. The publication
 *  record is the same for the whole paste: it is what the person is looking at. */
export function toImportBody(publicationRecordId: string, rows: CsvRow[]): Record<string, unknown>[] {
  return rows.map((row) => ({
    publication_record_id: publicationRecordId,
    platform: row.platform,
    account_id: row.accountId,
    metric: row.metric,
    // null stays null across the wire. JSON has a null, and this is what it
    // is for.
    value: row.value,
    unit: row.unit,
    stat_window: row.statWindow,
    sampled_at: row.sampledAt,
    evidence_note: "",
  }));
}

/** How many of a parsed paste carry no number. Shown before the import so the
 *  person can see that the blanks were read as blanks. */
export function unknownCount(rows: CsvRow[] | ManualMetric[]): number {
  return rows.filter((row) => row.value === null).length;
}
