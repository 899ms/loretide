// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import { describeMetricValue, parseManualMetric, parseManualMetrics, sameMetricValue } from "./contract";
import { parsePastedMetrics, toImportBody, unknownCount } from "./csv";

// SOP 10.1: "未知填空；0 只表示已确认的零值".
//
// This file exists on its own because the rule it defends is the one thing in
// this card that can go wrong SILENTLY. A number nobody observed, sitting in a
// column that looks like every other number, is not visible in any later
// report - it just makes the average wrong. Nothing downstream would flag it.
//
// So the rule is pinned at every point an empty could become a zero: parsing
// the response, parsing a pasted cell, building the import body, comparing,
// and rendering. The Go side pins the same rule in its own two places.

describe("a response with no number", () => {
  const base = { manual_metric_id: "m1", metric: "conversion" };

  it.each([
    ["an explicit null", { ...base, value: null }],
    ["an absent field", base],
  ])("reads %s as unknown, not as zero", (_name, wire) => {
    const metric = parseManualMetric(wire);
    expect(metric.value).toBeNull();
    expect(metric.value).not.toBe(0);
  });

  it("reads an explicit zero as zero", () => {
    expect(parseManualMetric({ ...base, value: 0 }).value).toBe(0);
  });

  it("keeps the two apart in a list", () => {
    const metrics = parseManualMetrics({
      metrics: [
        { manual_metric_id: "m1", metric: "conversion", value: null },
        { manual_metric_id: "m2", metric: "follow", value: 0 },
      ],
    });
    expect(metrics[0]!.value).toBeNull();
    expect(metrics[1]!.value).toBe(0);
    expect(sameMetricValue(metrics[0]!.value, metrics[1]!.value)).toBe(false);
  });

  // A build that could not parse the response has not learned that the number
  // is zero.
  it("degrades a malformed response to unknown, not to zero", () => {
    expect(parseManualMetric({ nonsense: true }).value).toBeNull();
  });
});

describe("a pasted cell with nothing in it", () => {
  // Number("") is 0 in JavaScript. That single fact is why this test exists.
  it("is not what Number() would make of it", () => {
    expect(Number("")).toBe(0);
    const parsed = parsePastedMetrics(
      "platform,account_id,metric,value,unit,stat_window,sampled_at\n" +
        "xiaohongshu,acct-1,conversion,,次,首日,2026-09-20T10:00:00Z",
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows[0]!.value).toBeNull();
    expect(parsed.rows[0]!.value).not.toBe(0);
  });

  it("keeps a typed zero as zero", () => {
    const parsed = parsePastedMetrics(
      "platform,account_id,metric,value,unit,stat_window,sampled_at\n" +
        "xiaohongshu,acct-1,follow,0,人,首日,2026-09-20T10:00:00Z",
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.rows[0]!.value).toBe(0);
  });

  it("carries the null across the wire rather than dropping the key", () => {
    const parsed = parsePastedMetrics(
      "xiaohongshu,acct-1,conversion,,次,首日,2026-09-20T10:00:00Z",
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    const [body] = toImportBody("pub-1", parsed.rows);
    expect(body).toHaveProperty("value");
    expect(body!.value).toBeNull();
    // A dropped key would arrive at a server that fills in a default, which is
    // the same corruption by a different route.
    expect(JSON.stringify(body)).toContain('"value":null');
  });

  it("can say how many of a paste carry no number", () => {
    const parsed = parsePastedMetrics(
      "xiaohongshu,acct-1,conversion,,次,首日,2026-09-20T10:00:00Z\n" +
        "xiaohongshu,acct-1,follow,0,人,首日,2026-09-20T10:00:00Z\n" +
        "xiaohongshu,acct-1,read,120,次,首日,2026-09-20T10:00:00Z",
    );
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(unknownCount(parsed.rows)).toBe(1);
  });
});

describe("rendering", () => {
  it("never shows an unknown as 0 and never as a blank", () => {
    const shown = describeMetricValue(null);
    expect(shown.known).toBe(false);
    expect(shown.text).toBe("unknown");
  });
});

// The Go side has to keep the same rule, and the column has to stay nullable.
// A plain int64 or a NOT NULL cannot express "unknown" at all.
describe("the storage layer keeps the same rule", () => {
  const goContract = readFileSync(
    join(__dirname, "../../../../server/internal/content/feedback-learning/contract.go"),
    "utf8",
  );
  const migration = readFileSync(
    join(__dirname, "../../../../server/migrations/526_content_manual_metric.up.sql"),
    "utf8",
  );

  it("declares the value as a pointer", () => {
    expect(goContract).toMatch(/Value\s+\*int64/);
  });

  it("leaves the column nullable", () => {
    expect(migration).toMatch(/value\s+bigint[,\s]/);
    expect(migration).not.toMatch(/value\s+bigint\s+NOT NULL/);
  });
});
