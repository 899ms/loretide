// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  parseLeadDaysCell,
  parseMarketingNodeCsv,
  splitCsv,
} from "./marketing-node-import";

const HEADER =
  "name,kind,starts_on,ends_on,timezone,lead_days,goal,date_certainty,date_basis";
const defaults = { timezone: "Asia/Shanghai" };

function rowsOf(text: string) {
  const parsed = parseMarketingNodeCsv(text, defaults);
  if (!parsed.ok) throw new Error(`header refused: ${parsed.error}`);
  return parsed.rows;
}

describe("marketing node CSV import", () => {
  it("keeps a comma and a doubled quote inside a quoted field", () => {
    const rows = rowsOf(
      `${HEADER}\n"双十一, 大促",marketing,2026-11-11,2026-11-11,,14,"清库存 ""加"" 拉新",confirmed,`,
    );
    expect(rows).toHaveLength(1);
    const row = rows[0]!;
    expect(row.ok).toBe(true);
    if (row.ok) {
      expect(row.input.name).toBe("双十一, 大促");
      expect(row.input.goal).toBe('清库存 "加" 拉新');
    }
  });

  it("skips blank lines and reports the line a row started on", () => {
    const rows = rowsOf(
      `${HEADER}\n\n双十一,marketing,2026-11-11,2026-11-11,,,,confirmed,\n   \n年货节,holiday,2027-01-15,2027-01-20,,,,tentative,去年同期\n`,
    );
    expect(rows.map((row) => row.line)).toEqual([3, 5]);
    expect(rows.every((row) => row.ok)).toBe(true);
  });

  it("strips a byte order mark before the header", () => {
    const rows = rowsOf(
      `﻿${HEADER}\r\n双十一,marketing,2026-11-11,2026-11-11,,,,confirmed,\r\n`,
    );
    expect(rows).toHaveLength(1);
    expect(rows[0]!.ok).toBe(true);
  });

  it("refuses a row whose column count does not match the header", () => {
    const rows = rowsOf(`${HEADER}\n双十一,marketing,2026-11-11\n`);
    expect(rows[0]).toEqual({
      line: 2,
      ok: false,
      error: { reason: "column_count", expected: 9, actual: 3 },
    });
  });

  it("reads an empty lead time as not set and 0 as zero", () => {
    const rows = rowsOf(
      `${HEADER}\nA,marketing,2026-11-11,2026-11-11,,,,confirmed,\nB,marketing,2026-11-11,2026-11-11,,0,,confirmed,\nC,marketing,2026-11-11,2026-11-11,,1.5,,confirmed,`,
    );
    const [unset, zero, fractional] = rows;
    expect(unset?.ok && unset.input.leadDays).toEqual({ set: false });
    expect(zero?.ok && zero.input.leadDays).toEqual({ set: true, value: 0 });
    expect(fractional).toEqual({
      line: 4,
      ok: false,
      error: { reason: "invalid", column: "lead_days" },
    });
    expect(parseLeadDaysCell(" ")).toEqual({ set: false });
    expect(parseLeadDaysCell("-1")).toBeNull();
  });

  it("refuses an impossible date and guesses nothing else", () => {
    const rows = rowsOf(
      `${HEADER}\n双十一,,2026-02-30,2026-03-01,,,,confirmed,\n春节,,2027-02-06,2027-02-06,,,,,`,
    );
    expect(rows[0]).toEqual({
      line: 2,
      ok: false,
      error: { reason: "invalid", column: "starts_on" },
    });
    // An empty kind or certainty is passed through for the server to refuse;
    // nothing is inferred from the name.
    const second = rows[1]!;
    expect(second.ok).toBe(true);
    if (second.ok) {
      expect(second.input.kind).toBe("");
      expect(second.input.dateCertainty).toBe("");
      expect(second.input.timezone).toBe("Asia/Shanghai");
    }
  });

  it("accepts columns in any order and requires the essential ones", () => {
    const rows = rowsOf(
      "date_certainty,ends_on,starts_on,kind,name\nconfirmed,2026-11-12,2026-11-11,marketing,双十一",
    );
    const row = rows[0]!;
    expect(row.ok && row.input).toMatchObject({
      name: "双十一",
      startsOn: "2026-11-11",
      endsOn: "2026-11-12",
      leadDays: { set: false },
    });
    expect(parseMarketingNodeCsv("name,kind\nA,holiday", defaults)).toEqual({
      ok: false,
      error: "missing_columns",
      missing: ["starts_on", "ends_on", "date_certainty"],
    });
    expect(parseMarketingNodeCsv("\n\n", defaults)).toEqual({
      ok: false,
      error: "empty",
      missing: [],
    });
  });

  it("keeps a newline inside a quoted field in one record", () => {
    const records = splitCsv('a,"line 1\nline 2",c\nd,e,f');
    expect(records).toEqual([
      { line: 1, fields: ["a", "line 1\nline 2", "c"] },
      { line: 3, fields: ["d", "e", "f"] },
    ]);
  });
});
