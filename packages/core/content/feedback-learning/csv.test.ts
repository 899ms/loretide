// @vitest-environment node
import { describe, expect, it } from "vitest";

import { CSV_COLUMNS, parsePastedMetrics, toImportBody } from "./csv";

// The first version of CSV import is a PASTE: somebody copies cells out of a
// spreadsheet and drops them in. File upload arrives with W-03 and nothing
// here touches a file.
//
// The empty-cell rule has its own file (empty-is-not-zero.test.ts); this one
// covers the rest.

const header = CSV_COLUMNS.join(",");

describe("parsePastedMetrics", () => {
  it("reads a paste with a header in any column order", () => {
    const parsed = parsePastedMetrics(
      "metric,platform,value,sampled_at,account_id,unit,stat_window\n" +
        "read,xiaohongshu,1200,2026-09-20T10:00:00Z,acct-1,次,首日",
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows[0]).toEqual({
      platform: "xiaohongshu", accountId: "acct-1", metric: "read",
      value: 1200, unit: "次", statWindow: "首日",
      sampledAt: "2026-09-20T10:00:00Z",
    });
  });

  it("reads a paste with no header in the documented order", () => {
    const parsed = parsePastedMetrics("xiaohongshu,acct-1,read,1200,次,首日,2026-09-20T10:00:00Z");
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows[0]!.metric).toBe("read");
  });

  it("accepts tabs, which is what a spreadsheet paste actually produces", () => {
    const parsed = parsePastedMetrics(
      "xiaohongshu\tacct-1\tread\t1200\t次\t首日\t2026-09-20T10:00:00Z",
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows[0]!.value).toBe(1200);
  });

  it("keeps a quoted comma inside its cell", () => {
    const parsed = parsePastedMetrics(
      `${header}\nxiaohongshu,acct-1,read,1200,次,"首日, 含预热",2026-09-20T10:00:00Z`,
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows[0]!.statWindow).toBe("首日, 含预热");
  });

  it("reads a thousands separator as one number", () => {
    const parsed = parsePastedMetrics(`${header}\nxiaohongshu,acct-1,read,"12,400",次,首日,2026-09-20T10:00:00Z`);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows[0]!.value).toBe(12400);
  });

  it("ignores blank lines", () => {
    const parsed = parsePastedMetrics(
      `${header}\n\nxiaohongshu,acct-1,read,1,次,首日,2026-09-20T10:00:00Z\n\n`,
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows).toHaveLength(1);
  });

  it("reads an empty paste as no rows rather than an error", () => {
    expect(parsePastedMetrics("")).toEqual({ ok: true, rows: [] });
    expect(parsePastedMetrics("\n  \n")).toEqual({ ok: true, rows: [] });
  });
});

// All or nothing, and the problem names the line. "Some row is wrong" in a
// paste of forty rows is not an answer anybody can act on.
describe("a paste with something wrong in it", () => {
  it.each([
    [
      "a metric outside the set",
      `${header}\nxiaohongshu,acct-1,read,1,次,首日,2026-09-20T10:00:00Z\nxiaohongshu,acct-1,engagement,2,次,首日,2026-09-20T10:00:00Z`,
      { row: 2, column: "metric", reason: "outside-the-set" },
    ],
    [
      "a platform outside the set",
      `${header}\nweibo,acct-1,read,1,次,首日,2026-09-20T10:00:00Z`,
      { row: 1, column: "platform", reason: "outside-the-set" },
    ],
    [
      "a value that is not a number",
      `${header}\nxiaohongshu,acct-1,read,about a thousand,次,首日,2026-09-20T10:00:00Z`,
      { row: 1, column: "value", reason: "not-a-number" },
    ],
    [
      "no sample time",
      `${header}\nxiaohongshu,acct-1,read,1,次,首日,`,
      { row: 1, column: "sampled_at", reason: "missing" },
    ],
    [
      "an empty platform cell",
      `${header}\n,acct-1,read,1,次,首日,2026-09-20T10:00:00Z`,
      { row: 1, column: "platform", reason: "missing" },
    ],
  ])("stops on %s and says which line", (_name, text, problem) => {
    const parsed = parsePastedMetrics(text);
    expect(parsed.ok).toBe(false);
    if (parsed.ok) return;
    expect(parsed.problem).toEqual(problem);
  });

  it("counts data rows, not lines - the header is not row 1", () => {
    const parsed = parsePastedMetrics(`${header}\nweibo,acct-1,read,1,次,首日,2026-09-20T10:00:00Z`);
    expect(parsed.ok).toBe(false);
    if (parsed.ok) return;
    expect(parsed.problem.row).toBe(1);
  });
});

describe("toImportBody", () => {
  it("hangs every row off the same publication record", () => {
    const parsed = parsePastedMetrics(
      `${header}\nxiaohongshu,acct-1,read,1,次,首日,2026-09-20T10:00:00Z\nxiaohongshu,acct-1,like,2,次,首日,2026-09-20T10:00:00Z`,
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    const body = toImportBody("pub-1", parsed.rows);
    expect(body).toHaveLength(2);
    expect(body.every((row) => row.publication_record_id === "pub-1")).toBe(true);
  });

  // The origin is the server's to record, from which endpoint was called.
  it("never sends a source type", () => {
    const body = toImportBody("pub-1", [
      { platform: "xiaohongshu", accountId: "", metric: "read", value: 1, unit: "", statWindow: "", sampledAt: "2026-09-20T10:00:00Z" },
    ]);
    expect(body[0]).not.toHaveProperty("source_type");
    expect(body[0]).not.toHaveProperty("recorded_by");
  });
});
