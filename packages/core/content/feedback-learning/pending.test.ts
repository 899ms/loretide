// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import type { PendingFeedback } from "./contract";
import {
  DASHBOARD_PREVIEW,
  describePending,
  feedbackBlockIsAvailable,
  pendingFeedbackSummary,
} from "./pending";

// SOP 2's fifth workbench item. specs/026 wrote this block as "暂不可用" back
// when this module did not exist; the ruling on Q6 made turning it on part of
// this card, and this is the function the dashboard calls.

function pending(over: Partial<PendingFeedback> = {}): PendingFeedback {
  return {
    publicationRecordId: "pub-1", workId: "w", artifactId: "a",
    channel: "xiaohongshu", status: "verified_published",
    publishedAt: "2026-09-20T10:00:00Z", createdAt: "2026-09-20T10:00:00Z",
    ...over,
  };
}

describe("pendingFeedbackSummary", () => {
  it("counts everything and previews a few", () => {
    const rows = Array.from({ length: 8 }, (_unused, index) =>
      pending({ publicationRecordId: `pub-${index}` }));
    const summary = pendingFeedbackSummary(rows);
    expect(summary.count).toBe(8);
    expect(summary.items).toHaveLength(DASHBOARD_PREVIEW);
    expect(summary.empty).toBe(false);
  });

  // Nothing to do is a result, not an empty state to apologise for.
  it("says empty when there is nothing waiting", () => {
    const summary = pendingFeedbackSummary([]);
    expect(summary.count).toBe(0);
    expect(summary.items).toEqual([]);
    expect(summary.empty).toBe(true);
  });
});

// 026's FR-002: every row says which thing it is, not just a number.
describe("describePending", () => {
  it("names the channel and when it went out", () => {
    expect(describePending(pending())).toContain("xiaohongshu");
    expect(describePending(pending())).toContain("2026-09-20T10:00:00Z");
  });

  it("still names something when the publication time is unknown", () => {
    const described = describePending(pending({ publishedAt: null }));
    expect(described).toContain("xiaohongshu");
    expect(described).not.toContain("null");
  });

  it("does not leave a row blank when the channel is unknown", () => {
    expect(describePending(pending({ channel: "" }))).not.toBe("");
  });
});

// The whole point of this card for the dashboard.
describe("the dashboard block", () => {
  it("is available now that the module exists", () => {
    expect(feedbackBlockIsAvailable()).toBe(true);
  });

  // 026's FR-004 listed feedback-learning among the modules the dashboard must
  // not read, and FR-005b required the "暂不可用" wording. The ruling made
  // updating both part of this card - this test is what fails if the spec is
  // left saying the old thing.
  it("is no longer listed as unlanded in specs/026", () => {
    const spec = readFileSync(
      join(__dirname, "../../../../specs/026-today-dashboard/spec.md"),
      "utf8",
    );
    // The parenthesised list of modules the dashboard may not read - not the
    // prose around it, which legitimately names the module while explaining
    // that it moved out of the list.
    const list = /MUST NOT 读取未落地模块（([^）]*)）/.exec(spec);
    expect(list, "FR-004's unlanded-module list not found in specs/026").not.toBeNull();
    expect(list![1]).not.toContain("feedback-learning");
    // And the block no longer claims to be unavailable.
    const fr005b = /- \*\*FR-005b\*\*:([^\n]*)/.exec(spec);
    expect(fr005b, "FR-005b not found in specs/026").not.toBeNull();
    expect(fr005b![1]).not.toContain("暂不可用");
  });
});
