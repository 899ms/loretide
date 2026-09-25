// @vitest-environment node
import { describe, expect, it } from "vitest";

import { parsePastedRoiRows, ROI_IMPORT_COLUMNS, toRoiImportBody } from "./csv";

// specs/034 PR 3 (T069): pasted costs, leads and deals.

describe("parsePastedRoiRows", () => {
  it("keeps every amount as the string that was typed", () => {
    const parsed = parsePastedRoiRows("cost", [
      "category,pricing,amount,currency,incurred_at",
      "拍摄,amount,3000.00,CNY,2026-09-10T02:00:00Z",
      "外景,amount,12345678901234.56,CNY,2026-09-11T02:00:00Z",
    ].join("\n"));
    expect(parsed).toEqual({
      ok: true,
      rows: [
        { category: "拍摄", pricing: "amount", amount: "3000.00", currency: "CNY", incurred_at: "2026-09-10T02:00:00Z" },
        { category: "外景", pricing: "amount", amount: "12345678901234.56", currency: "CNY", incurred_at: "2026-09-11T02:00:00Z" },
      ],
    });
  });

  it("leaves an empty amount out rather than making it 0", () => {
    const parsed = parsePastedRoiRows("cost", [
      "category,pricing,currency,labor_minutes,labor_rate,incurred_at",
      "剪辑,labor_time,CNY,360,,2026-09-10T02:00:00Z",
    ].join("\n"));
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows[0]).toEqual({
      category: "剪辑", pricing: "labor_time", currency: "CNY", labor_minutes: 360,
      incurred_at: "2026-09-10T02:00:00Z",
    });
    expect("labor_rate" in parsed.rows[0]!).toBe(false);
  });

  it("names the row and column of the first problem, counting data rows", () => {
    const text = [
      "order_ref,amount,currency,closed_at,gross_basis",
      "TB-1,100.00,CNY,2026-09-12T02:00:00Z,none",
      "TB-2,100.00,RMB,2026-09-12T02:00:00Z,none",
      "TB-3,100.00,USD,2026-09-12T02:00:00Z,guess",
    ].join("\n");
    expect(parsePastedRoiRows("deal", text)).toEqual({
      ok: false, problem: { row: 2, column: "currency", reason: "outside-the-set" },
    });
  });

  it.each([
    ["a missing required cell", "lead", "customer_ref,first_seen_at\n客户-1,", { row: 1, column: "first_seen_at", reason: "missing" }],
    ["a required column left out of the header", "deal", "order_ref,amount,closed_at,gross_basis\nTB-1,1.00,2026-09-12T02:00:00Z,none", { row: 1, column: "currency", reason: "missing" }],
    ["a column this kind does not have", "lead", "customer_ref,phone,first_seen_at\n客户-1,138,2026-09-10T02:00:00Z", { row: 0, column: "phone", reason: "unknown-column" }],
    ["minutes that are not a whole number", "cost", "category,pricing,currency,labor_minutes,incurred_at\n剪辑,labor_time,CNY,1.5,2026-09-10T02:00:00Z", { row: 1, column: "labor_minutes", reason: "not-a-whole-number" }],
    ["a yes/no that is neither", "lead", "customer_ref,qualified,first_seen_at\n客户-1,maybe,2026-09-10T02:00:00Z", { row: 1, column: "qualified", reason: "not-yes-or-no" }],
    ["a pricing outside the set", "cost", "category,pricing,amount,currency,incurred_at\n拍摄,fixed,1.00,CNY,2026-09-10T02:00:00Z", { row: 1, column: "pricing", reason: "outside-the-set" }],
  ] as const)("refuses %s", (_name, kind, text, problem) => {
    expect(parsePastedRoiRows(kind, text)).toEqual({ ok: false, problem });
  });

  it("reads a paste with no header in the kind's column order", () => {
    const parsed = parsePastedRoiRows("lead", "客户-0412\t咨询\t是\t2026-09-10T02:00:00Z\t");
    expect(parsed).toEqual({
      ok: true,
      rows: [{ customer_ref: "客户-0412", stage: "咨询", qualified: true, first_seen_at: "2026-09-10T02:00:00Z", note: "" }],
    });
    expect(ROI_IMPORT_COLUMNS.lead[0]).toBe("customer_ref");
  });

  it("keeps a quoted comma inside its cell", () => {
    const parsed = parsePastedRoiRows("deal", 'order_ref,amount,currency,closed_at,gross_basis,note\nTB-1,"1,000.00",CNY,2026-09-12T02:00:00Z,none,"a, b"');
    expect(parsed.ok && parsed.rows[0]).toMatchObject({ amount: "1,000.00", note: "a, b" });
  });

  it("has no customer identity column in any kind", () => {
    const all = Object.values(ROI_IMPORT_COLUMNS).flat();
    for (const token of ["name", "phone", "mobile", "wechat", "email", "id_card", "address"]) {
      expect(all.filter((column) => column.includes(token))).toEqual([]);
    }
  });
});

describe("toRoiImportBody", () => {
  it("puts not_duplicate_of only on the rows the person confirmed", () => {
    const body = toRoiImportBody("deal", [{ order_ref: "TB-1" }, { order_ref: "TB-2" }], {
      dryRun: true, confirmations: { 2: ["d-9"] },
    });
    expect(body).toEqual({
      record_kind: "deal",
      dry_run: true,
      rows: [{ order_ref: "TB-1" }, { order_ref: "TB-2", not_duplicate_of: ["d-9"] }],
    });
  });

  it("is a real import unless asked for a dry run", () => {
    expect(toRoiImportBody("cost", [])).toEqual({ record_kind: "cost", dry_run: false, rows: [] });
  });
});
