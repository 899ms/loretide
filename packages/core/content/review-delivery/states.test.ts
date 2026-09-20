// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import { DELIVERY_STATUSES, REVIEW_STATUSES } from "./contract";
import {
  canTransitionDelivery,
  canTransitionReview,
  deliveryMoves,
  publicationRequiredFields,
  requiredFieldFor,
  requiresApprovedReview,
} from "./states";

// The page's copy of the state machines exists so a button can be disabled with
// a reason rather than hidden. Two hand-written copies drift, so this one is
// compared against the Go table on every run.
const goSource = readFileSync(
  join(__dirname, "../../../../server/internal/content/review-delivery/states.go"),
  "utf8",
);

function goTransitions(varName: string, prefix: string): Record<string, string[]> {
  const block = new RegExp(`var ${varName} = map\\[[A-Za-z]+\\]\\[\\][A-Za-z]+\\{(.*?)\\n\\}`, "s")
    .exec(goSource);
  if (!block) throw new Error(`${varName} not found in states.go`);
  const table: Record<string, string[]> = {};
  const constantValue = (name: string): string => {
    const literal = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(
      readFileSync(
        join(__dirname, "../../../../server/internal/content/review-delivery/contract.go"),
        "utf8",
      ),
    );
    if (!literal) throw new Error(`${name} has no literal`);
    return literal[1]!;
  };
  for (const line of block[1]!.split("\n")) {
    const entry = new RegExp(`^\\s*(${prefix}[A-Za-z]+):\\s*\\{([^}]*)\\}`).exec(line);
    if (!entry) continue;
    table[constantValue(entry[1]!)] = entry[2]!
      .split(",")
      .map((name) => name.trim())
      .filter(Boolean)
      .map(constantValue);
  }
  return table;
}

describe("the transition tables match the Go source", () => {
  it("review requests", () => {
    const table = goTransitions("reviewTransitions", "Review");
    expect(Object.keys(table).sort()).toEqual([...REVIEW_STATUSES].sort());
    for (const from of REVIEW_STATUSES) {
      for (const to of REVIEW_STATUSES) {
        expect([from, to, canTransitionReview(from, to)]).toEqual([
          from, to, (table[from] ?? []).includes(to),
        ]);
      }
    }
  });

  it("delivery tasks", () => {
    const table = goTransitions("deliveryTransitions", "Delivery");
    expect(Object.keys(table).sort()).toEqual([...DELIVERY_STATUSES].sort());
    for (const from of DELIVERY_STATUSES) {
      for (const to of DELIVERY_STATUSES) {
        expect([from, to, canTransitionDelivery(from, to)]).toEqual([
          from, to, (table[from] ?? []).includes(to),
        ]);
      }
    }
  });
});

describe("what the page offers", () => {
  // Every status is offered and the illegal ones are disabled. Hiding them
  // would say the product does not have the move, which is a different and
  // wrong message.
  it("lists all six delivery statuses whatever the current one", () => {
    for (const from of DELIVERY_STATUSES) {
      expect(deliveryMoves(from).map((move) => move.status)).toEqual([...DELIVERY_STATUSES]);
    }
  });

  it("offers nothing from a terminal status", () => {
    for (const from of ["handed_off", "cancelled"]) {
      expect(deliveryMoves(from).filter((move) => move.allowed)).toEqual([]);
    }
  });

  // A status this build has not heard of offers nothing rather than everything.
  it("offers nothing from a status it does not know", () => {
    expect(deliveryMoves("escalated").filter((move) => move.allowed)).toEqual([]);
    expect(canTransitionReview("escalated", "approved")).toBe(false);
  });
});

describe("what a move needs", () => {
  it.each([
    ["scheduled", "scheduled_at"],
    ["handed_off", "handoff_method"],
    ["held", "reason"],
    ["cancelled", "reason"],
    ["ready", null],
    ["draft", null],
    // Default branch: an unknown status asks for nothing rather than blocking
    // the form on a requirement it cannot name.
    ["escalated", null],
  ])("%s needs %s", (status, field) => {
    expect(requiredFieldFor(status)).toBe(field);
  });

  it.each([
    ["reported_published", ["page_url_or_content_id"]],
    ["verified_published", ["page_url_or_content_id", "verification_note"]],
    ["failed", ["receipt_note"]],
    ["removed", ["receipt_note"]],
    ["unknown", []],
    ["something_new", []],
  ])("a %s record needs %s", (status, fields) => {
    expect(publicationRequiredFields(status)).toEqual(fields);
  });
});

describe("the approved-review precondition", () => {
  it("exempts draft and nothing else", () => {
    expect(requiresApprovedReview("draft")).toBe(false);
    for (const status of DELIVERY_STATUSES.filter((value) => value !== "draft")) {
      expect(requiresApprovedReview(status)).toBe(true);
    }
  });
});
