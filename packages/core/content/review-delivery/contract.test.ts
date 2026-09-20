// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  DELIVERY_STATUSES,
  HANDOFF_METHODS,
  PUBLICATION_STATUSES,
  REVIEW_CHANNELS,
  REVIEW_STATUSES,
  TRANSITION_SUBJECTS,
  VERSION_MATCHES,
  currentPublication,
  handoffMeansPublished,
  parseDeliveries,
  parsePublications,
  parseReview,
  parseReviewDetail,
  parseReviews,
} from "./contract";

// The controlled sets are the server's. Restating them here is unavoidable - a
// component has to switch on them - so they are compared against the Go source
// rather than against my memory of it, the same way work-editor's are.
const goSource = readFileSync(
  join(__dirname, "../../../../server/internal/content/review-delivery/contract.go"),
  "utf8",
);

function goSetValues(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(goSource);
  if (!declaration) throw new Error(`${varName} not found in contract.go`);
  const constants = declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean);
  return constants.map((name) => {
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goSource);
    if (!value) throw new Error(`${name} has no literal in contract.go`);
    return value[1]!;
  });
}

describe("the controlled sets match the Go source", () => {
  it.each([
    ["Channels", REVIEW_CHANNELS],
    ["ReviewStatuses", REVIEW_STATUSES],
    ["DeliveryStatuses", DELIVERY_STATUSES],
    ["PublicationStatuses", PUBLICATION_STATUSES],
    ["HandoffMethods", HANDOFF_METHODS],
    ["VersionMatches", VERSION_MATCHES],
    ["SubjectKinds", TRANSITION_SUBJECTS],
  ])("%s", (goName, tsValues) => {
    expect([...tsValues]).toEqual(goSetValues(goName));
  });

  // Ruling Q5: SOP 9.2 enumerates neither the declarer nor the verification
  // method, so this card defines neither. The guard is for the day someone
  // reads two free text fields and decides that "completing" them is helpful.
  it("has no controlled set for who declared a publication or how it was checked", () => {
    const sets = [...goSource.matchAll(/^var ([A-Z][A-Za-z]*) = \[\]/gm)].map((match) => match[1]);
    expect(sets.sort()).toEqual([
      "Channels", "DeliveryStatuses", "HandoffMethods", "PublicationStatuses",
      "ReviewStatuses", "SnapshotKeys", "SubjectKinds", "VersionMatches",
    ]);
  });
});

// SOP 8's eight keys, read out of the Go struct rather than restated.
describe("the delivery snapshot", () => {
  it("has exactly the eight keys SOP 8 names", () => {
    const declaration = /var SnapshotKeys = \[\]string\{([^}]*)\}/s.exec(goSource);
    if (!declaration) throw new Error("SnapshotKeys not found");
    const keys = [...declaration[1]!.matchAll(/"([a-z_]+)"/g)].map((match) => match[1]);
    expect(keys).toEqual([
      "channel", "work_id", "artifact_id", "version_id",
      "account_id", "start_snapshot_id", "attachments", "delivery_config",
    ]);
  });
});

const reviewWire = {
  review_request_id: "review-1",
  workspace_id: "ws-1",
  work_id: "work-1",
  artifact_id: "artifact-1",
  version_id: "v3",
  account_id: "acct-1",
  channel: "xiaohongshu",
  snapshot: {
    channel: "xiaohongshu",
    work_id: "work-1",
    artifact_id: "artifact-1",
    version_id: "v3",
    account_id: "acct-1",
    start_snapshot_id: "",
    attachments: [],
    delivery_config: {},
  },
  status: "pending",
  requested_by: "user-1",
  requested_at: "2026-09-20T00:00:00Z",
  decided_by: "",
  decided_at: null,
  decision_note: "",
  created_at: "2026-09-20T00:00:00Z",
  updated_at: "2026-09-20T00:00:00Z",
};

describe("parseReview", () => {
  it("reads an undecided request as undecided, not as missing data", () => {
    const review = parseReview(reviewWire);
    expect(review.reviewRequestId).toBe("review-1");
    expect(review.decidedBy).toBe("");
    expect(review.decidedAt).toBe("");
    expect(review.snapshot.startSnapshotId).toBe("");
  });

  it("degrades a malformed response instead of throwing", () => {
    expect(parseReview({ nonsense: true }).reviewRequestId).toBe("");
    expect(parseReviews({ reviews: null })).toEqual([]);
    expect(parseReviews("not an object")).toEqual([]);
    expect(parseReviewDetail(null).transitions).toEqual([]);
  });

  // An installed desktop build talks to whatever backend is deployed. A status
  // this build has not heard of must reach the page, not be blanked: the page
  // can show an unknown status, but it cannot show one it never received.
  it("keeps a status it has never heard of", () => {
    const review = parseReview({ ...reviewWire, status: "escalated" });
    expect(review.status).toBe("escalated");
  });

  it("reads a null snapshot as the empty one rather than throwing", () => {
    const review = parseReview({ ...reviewWire, snapshot: null });
    expect(review.snapshot.attachments).toEqual([]);
    expect(review.snapshot.deliveryConfig).toEqual({});
  });

  // W-03 has not landed. null and absent both have to arrive as [], because a
  // caller should not have to know which of the two it got before mapping.
  it("reads null attachments as an empty array", () => {
    const review = parseReview({
      ...reviewWire,
      snapshot: { ...reviewWire.snapshot, attachments: null, delivery_config: null },
    });
    expect(review.snapshot.attachments).toEqual([]);
    expect(review.snapshot.deliveryConfig).toEqual({});
  });
});

describe("parseDeliveries", () => {
  const wire = {
    delivery_task_id: "task-1",
    artifact_id: "artifact-1",
    channel: "xiaohongshu",
    status: "scheduled",
    scheduled_at: "2026-09-25T02:00:00Z",
    handoff_method: "",
    due: true,
    pending_registration: false,
  };

  it("reads the two derived flags as booleans", () => {
    const [task] = parseDeliveries({ deliveries: [wire] });
    expect(task!.due).toBe(true);
    expect(task!.pendingRegistration).toBe(false);
  });

  // A backend that stops sending them must read as "not due" and "nothing
  // pending" - never as an undefined that a later `!` flips into the opposite.
  it("treats a missing flag as false rather than truthy-unknown", () => {
    const [task] = parseDeliveries({
      deliveries: [{ delivery_task_id: "task-2", artifact_id: "a" }],
    });
    expect(task!.due).toBe(false);
    expect(task!.pendingRegistration).toBe(false);
  });

  it("reads a null planned time as no planned time", () => {
    const [task] = parseDeliveries({
      deliveries: [{ ...wire, scheduled_at: null }],
    });
    expect(task!.scheduledAt).toBe("");
  });

  it("degrades a malformed response instead of throwing", () => {
    expect(parseDeliveries({ deliveries: null })).toEqual([]);
    expect(parseDeliveries(42)).toEqual([]);
  });
});

describe("publication records", () => {
  const rows = {
    publications: [
      { publication_record_id: "p3", status: "removed", receipt_note: "平台删了" },
      { publication_record_id: "p2", status: "verified_published" },
      { publication_record_id: "p1", status: "reported_published" },
    ],
  };

  // FR-019: the current status is the latest row, not a stored field.
  it("takes the current status from the newest record", () => {
    const records = parsePublications(rows);
    expect(currentPublication(records)?.status).toBe("removed");
  });

  it("says there is no current status when nothing was recorded", () => {
    expect(currentPublication([])).toBeNull();
    expect(currentPublication(parsePublications({ publications: null }))).toBeNull();
  });

  // SOP 9.1: none of the three handover actions means published.
  it.each([...HANDOFF_METHODS])("does not treat %s as a publication", (method) => {
    expect(handoffMeansPublished(method)).toBe(false);
  });
});
