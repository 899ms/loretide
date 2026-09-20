// @vitest-environment node
import { describe, expect, it } from "vitest";

import { DELIVERY_STATUSES, type DeliveryTask, type PublicationRecord, type ReviewRequest } from "./contract";
import {
  APPROVE_AND_SCHEDULE_STEPS,
  REVIEW_AI_ENTRY_POINTS,
  advanceReady,
  canApproveAndSchedule,
  deliveryMoveOptions,
  isDecided,
  isShownAsDue,
  latestRequestFor,
  pendingRequestFor,
  publicationSummary,
  recordReady,
  reviewAIEntryPointEnabled,
  submitBlocker,
} from "./page-state";

function review(over: Partial<ReviewRequest> = {}): ReviewRequest {
  return {
    reviewRequestId: "r1", workspaceId: "ws", workId: "w", artifactId: "a",
    versionId: "v3", accountId: "acct", channel: "xiaohongshu",
    snapshot: {
      channel: "xiaohongshu", workId: "w", artifactId: "a", versionId: "v3",
      accountId: "acct", startSnapshotId: "", attachments: [], deliveryConfig: {},
    },
    status: "pending", requestedBy: "u", requestedAt: "", decidedBy: "",
    decidedAt: "", decisionNote: "", createdAt: "", updatedAt: "",
    ...over,
  };
}

function task(over: Partial<DeliveryTask> = {}): DeliveryTask {
  return {
    deliveryTaskId: "t1", workspaceId: "ws", workId: "w", artifactId: "a",
    reviewRequestId: "r1", channel: "xiaohongshu", status: "draft",
    scheduledAt: "", handoffMethod: "", createdAt: "", updatedAt: "",
    due: false, pendingRegistration: false,
    ...over,
  };
}

function record(over: Partial<PublicationRecord> = {}): PublicationRecord {
  return {
    publicationRecordId: "p1", workspaceId: "ws", workId: "w", artifactId: "a",
    deliveryTaskId: "", channel: "xiaohongshu", status: "reported_published",
    actorId: "u", declaredBy: "", pageUrlOrContentId: "", receiptNote: "",
    verificationNote: "", publishedAt: "", platformAccount: "",
    platformEdited: false, editNote: "", versionMatch: "unknown",
    versionId: "", historicalImport: false, createdAt: "",
    ...over,
  };
}

// SOP 8: the object of review is a specific channel's specific version. The
// page says why before the click; the server refuses it again after.
describe("submitBlocker", () => {
  it.each([
    ["a body", "body", 3, "acct", "notAChannelDraft"],
    ["a channel cut with no version yet", "channel_draft", 0, "acct", "noVersion"],
    ["a channel cut with no account chosen", "channel_draft", 3, "", "noAccount"],
    ["a channel cut that is ready", "channel_draft", 3, "acct", null],
  ])("%s", (_name, kind, versions, account, expected) => {
    expect(submitBlocker(kind as string, versions as number, account as string)).toBe(expected);
  });

  // A kind this build has not heard of is not submittable either: the safe
  // direction is refusing, because the server will refuse it anyway.
  it("refuses a kind it has never heard of", () => {
    expect(submitBlocker("research_note", 3, "acct")).toBe("notAChannelDraft");
  });
});

describe("isDecided", () => {
  it.each([
    ["pending", false],
    ["", false],
    ["approved", true],
    ["changes_requested", true],
    ["rejected", true],
    ["cancelled", true],
  ])("%s", (status, expected) => {
    expect(isDecided(status)).toBe(expected);
  });
});

// SOP 8: "系统不得把'已通过'自动套在新版本上". Per version, not per document.
describe("finding the request for a version", () => {
  const requests = [
    review({ reviewRequestId: "r2", versionId: "v3", status: "pending" }),
    review({ reviewRequestId: "r1", versionId: "v3", status: "rejected" }),
    review({ reviewRequestId: "r0", versionId: "v1", status: "approved" }),
  ];

  it("finds the pending one for that version", () => {
    expect(pendingRequestFor(requests, "v3")?.reviewRequestId).toBe("r2");
  });

  it("finds nothing for a version nobody submitted", () => {
    expect(pendingRequestFor(requests, "v5")).toBeNull();
    expect(latestRequestFor(requests, "v5")).toBeNull();
  });

  // The approval on v1 must not be read as covering v3.
  it("does not carry an approval from another version", () => {
    expect(latestRequestFor(requests, "v1")?.status).toBe("approved");
    expect(latestRequestFor(requests, "v3")?.status).not.toBe("approved");
  });
});

// SOP 8's personal mode: one click, two records. Collapsing them would make
// "who approved this" and "who scheduled it" the same fact, which they are not.
describe("approve and schedule", () => {
  it("is two steps, in order", () => {
    expect([...APPROVE_AND_SCHEDULE_STEPS]).toEqual(["approve", "createDeliveryTask"]);
  });

  it("is offered only while the request is still waiting", () => {
    expect(canApproveAndSchedule("pending")).toBe(true);
    for (const status of ["approved", "rejected", "changes_requested", "cancelled", ""]) {
      expect(canApproveAndSchedule(status)).toBe(false);
    }
  });
});

describe("publicationSummary", () => {
  it("takes the status from the newest record", () => {
    const summary = publicationSummary(
      [record({ status: "removed" }), record({ status: "verified_published" })],
      [],
    );
    expect(summary.status).toBe("removed");
    expect(summary.recordCount).toBe(2);
    expect(summary.pendingRegistration).toBe(false);
  });

  // SOP 9.2: the workbench keeps saying 待登记 and does not guess.
  it("says pending registration when a handover has nothing written down", () => {
    const summary = publicationSummary([], [task({ status: "handed_off", pendingRegistration: true })]);
    expect(summary.pendingRegistration).toBe(true);
    expect(summary.status).toBe("");
  });

  it("stops saying it once something is recorded", () => {
    const summary = publicationSummary(
      [record()],
      [task({ status: "handed_off", pendingRegistration: true })],
    );
    expect(summary.pendingRegistration).toBe(false);
  });

  it("does not say it for a piece that was never handed over", () => {
    expect(publicationSummary([], [task({ status: "ready" })]).pendingRegistration).toBe(false);
    expect(publicationSummary([], []).pendingRegistration).toBe(false);
  });
});

describe("deliveryMoveOptions", () => {
  it("offers every status and marks the illegal ones", () => {
    const options = deliveryMoveOptions("draft", DELIVERY_STATUSES);
    expect(options.map((option) => option.status)).toEqual([...DELIVERY_STATUSES]);
    expect(options.filter((option) => option.allowed).map((option) => option.status))
      .toEqual(["ready", "cancelled"]);
  });

  it("carries what each move needs", () => {
    const options = deliveryMoveOptions("ready", DELIVERY_STATUSES);
    const requires = Object.fromEntries(options.map((option) => [option.status, option.requires]));
    expect(requires).toEqual({
      draft: null, ready: null, scheduled: "scheduled_at",
      handed_off: "handoff_method", held: "reason", cancelled: "reason",
    });
  });

  it("offers nothing from a terminal status", () => {
    for (const from of ["handed_off", "cancelled"]) {
      expect(deliveryMoveOptions(from, DELIVERY_STATUSES).filter((o) => o.allowed)).toEqual([]);
    }
  });
});

// SOP 9.2: "运营者可标记实际发布失败、延后或取消，并注明原因".
describe("advanceReady", () => {
  it.each([
    ["scheduled without a time", "scheduled", {}, false],
    ["scheduled with a time", "scheduled", { scheduledAt: "2026-09-25T02:00" }, true],
    ["handed off without a method", "handed_off", {}, false],
    ["handed off with a method", "handed_off", { handoffMethod: "export" }, true],
    ["held without a reason", "held", {}, false],
    ["held with only whitespace", "held", { reason: "   " }, false],
    ["held with a reason", "held", { reason: "等配图" }, true],
    ["cancelled without a reason", "cancelled", {}, false],
    ["ready needs nothing", "ready", {}, true],
    ["no status chosen", "", {}, false],
  ])("%s", (_name, to, values, expected) => {
    expect(advanceReady(to as string, values as Record<string, string>)).toBe(expected);
  });
});

describe("recordReady", () => {
  it.each([
    ["reported without a link", "reported_published", {}, false],
    ["reported with a link", "reported_published", { pageUrlOrContentId: "https://x" }, true],
    ["reported with a content id", "reported_published", { pageUrlOrContentId: "mp-42" }, true],
    ["verified without a check", "verified_published", { pageUrlOrContentId: "https://x" }, false],
    [
      "verified with both",
      "verified_published",
      { pageUrlOrContentId: "https://x", verificationNote: "点开看过" },
      true,
    ],
    ["failed without a reason", "failed", {}, false],
    ["failed with a reason", "failed", { receiptNote: "平台拒了" }, true],
    ["removed without a reason", "removed", {}, false],
    ["unknown needs nothing", "unknown", {}, true],
    ["no status chosen", "", {}, false],
    // A status this build has not heard of is not blocked on a requirement it
    // cannot name.
    ["a status from a newer backend", "escalated", {}, true],
  ])("%s", (_name, status, values, expected) => {
    expect(recordReady(status as string, values as Record<string, string>)).toBe(expected);
  });
});

// Constitution IX. Present, disabled, with the reason written beside them -
// and nothing that fabricates a verification result.
describe("the AI entry points", () => {
  it("are exactly two and all disabled", () => {
    expect(REVIEW_AI_ENTRY_POINTS.map((entry) => entry.id))
      .toEqual(["autoSelfCheck", "autoVerifyPublication"]);
    for (const entry of REVIEW_AI_ENTRY_POINTS) {
      expect(reviewAIEntryPointEnabled(entry.id)).toBe(false);
    }
  });
});

// The planned time is never acted on. The page reads the flag the server
// derived; it does not compute one from the timestamp.
describe("isShownAsDue", () => {
  it("reads the server's flag and nothing else", () => {
    expect(isShownAsDue(task({ due: true }))).toBe(true);
    expect(isShownAsDue(task({ due: false, scheduledAt: "1999-01-01T00:00:00Z" }))).toBe(false);
  });
});
