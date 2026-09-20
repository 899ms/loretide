// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  MISSING_AUDIENCE,
  MISSING_CHANNELS,
  MISSING_PILLARS,
  MISSING_WEEKLY_HOURS,
  emptyProfile,
  isPendingWithValue,
  parseProfileRead,
  profileReadiness,
  usesNeutralExpression,
  type ExpressionProfile,
} from "./profile";

// The TypeScript half of the readiness parity contract.
//
// The Go half is server/internal/content/ip-profile/profile_parity_test.go.
// Both run against the same committed matrix, so a rule changed on one side
// without the other turns one of the two red. The matrix compares DECISIONS,
// not field names: names agreeing while answers differ is the drift that
// matters.

const HERE = dirname(fileURLToPath(import.meta.url));
const MATRIX = resolve(
  HERE,
  "../../../../specs/021-account-expression-profile/contracts/readiness-parity.json",
);

interface ParityCase {
  name: string;
  profile: ExpressionProfile;
  can_start: boolean;
  missing: string[];
  neutral: boolean;
}

function parityCases(): ParityCase[] {
  const doc = JSON.parse(readFileSync(MATRIX, "utf8")) as { cases: ParityCase[] };
  if (!doc.cases?.length) throw new Error("the parity matrix is empty, so it proves nothing");
  return doc.cases;
}

describe("readiness parity with Go", () => {
  it.each(parityCases())("$name", (tc) => {
    const readiness = profileReadiness(tc.profile);
    expect(readiness.can_start).toBe(tc.can_start);
    // Order is part of the contract: a page renders the list in order.
    expect(readiness.missing).toEqual(tc.missing);
    expect(usesNeutralExpression(tc.profile)).toBe(tc.neutral);
  });

  // Without this the matrix could agree with a rule inverted on both sides.
  it("covers both outcomes of both decisions", () => {
    const cases = parityCases();
    expect(cases.some((c) => c.can_start)).toBe(true);
    expect(cases.some((c) => !c.can_start)).toBe(true);
    expect(cases.some((c) => c.neutral)).toBe(true);
    expect(cases.some((c) => !c.neutral)).toBe(true);

    const reported = new Set(cases.flatMap((c) => c.missing));
    for (const key of [MISSING_AUDIENCE, MISSING_PILLARS, MISSING_CHANNELS, MISSING_WEEKLY_HOURS]) {
      expect(reported.has(key)).toBe(true);
    }
  });
});

describe("an untouched profile", () => {
  it("is every field pending and carries nothing", () => {
    const profile = emptyProfile();

    for (const [name, field] of Object.entries(profile)) {
      expect(field.status, `${name} status`).toBe("pending");
      if ("values" in field) expect(field.values, `${name} values`).toEqual([]);
      else if (typeof field.value === "number") expect(field.value, name).toBe(0);
      else expect(field.value, name).toBe("");
    }
  });

  it("cannot start and says all four things are missing", () => {
    expect(profileReadiness(emptyProfile())).toEqual({
      can_start: false,
      missing: [MISSING_AUDIENCE, MISSING_PILLARS, MISSING_CHANNELS, MISSING_WEEKLY_HOURS],
    });
  });
});

describe("parsing a profile response", () => {
  const complete = {
    revision_id: "rev-1",
    revision: 3,
    profile: {
      audience: { value: "设计师", status: "confirmed" },
      primary_channels: { values: ["xiaohongshu"], status: "confirmed" },
      weekly_hours: { value: 6, status: "confirmed" },
    },
    readiness: { can_start: false, missing: ["content_pillars"] },
    uses_neutral_expression: true,
  };

  it("reads what the server sent", () => {
    const read = parseProfileRead(complete);

    expect(read.revisionId).toBe("rev-1");
    expect(read.revision).toBe(3);
    expect(read.profile.audience).toEqual({ value: "设计师", status: "confirmed" });
    expect(read.profile.primary_channels.values).toEqual(["xiaohongshu"]);
    // The server's own decision is preferred over recomputing it.
    expect(read.readiness).toEqual({ can_start: false, missing: ["content_pillars"] });
  });

  it("fills every absent field as pending rather than leaving it undefined", () => {
    const read = parseProfileRead({ profile: {} });

    expect(read.profile.experience).toEqual({ value: "", status: "pending" });
    expect(read.profile.style_samples).toEqual({ values: [], status: "pending" });
    expect(read.profile.weekly_hours).toEqual({ value: 0, status: "pending" });
  });

  // Every one of these is reachable from a real response: an older backend, a
  // null, a field written by code that did not know the shape.
  it("degrades a malformed response to an all-pending profile", () => {
    for (const body of [undefined, null, "nope", 42, [], { profile: "nope" }]) {
      const read = parseProfileRead(body);
      expect(read.profile).toEqual(emptyProfile());
      expect(read.readiness.can_start).toBe(false);
      expect(read.usesNeutralExpression).toBe(true);
    }
  });

  // A status this build has never heard of must not count as confirmed. The
  // only thing confirmation unlocks is "you may start", so failing towards
  // "not yet" is the safe direction.
  it("treats an unknown status as pending", () => {
    const read = parseProfileRead({
      profile: { audience: { value: "设计师", status: "probably" } },
    });

    expect(read.profile.audience.status).toBe("pending");
    expect(read.readiness.missing).toContain(MISSING_AUDIENCE);
  });

  it("reads a null channel list as empty", () => {
    const read = parseProfileRead({
      profile: { primary_channels: { values: null, status: "confirmed" } },
    });

    expect(read.profile.primary_channels.values).toEqual([]);
    expect(read.readiness.missing).toContain(MISSING_CHANNELS);
  });

  // When the server sent no decision, the same rule is applied locally rather
  // than reporting a readiness nobody computed.
  it("computes readiness itself when the response carried none", () => {
    const read = parseProfileRead({
      profile: {
        audience: { value: "设计师", status: "confirmed" },
        content_pillars: { value: "工具评测", status: "confirmed" },
        primary_channels: { values: ["weibo"], status: "confirmed" },
        weekly_hours: { value: 4, status: "confirmed" },
      },
    });

    expect(read.readiness).toEqual({ can_start: true, missing: [] });
    expect(read.usesNeutralExpression).toBe(true);
  });
});

describe("pending with a value", () => {
  it("is how a page knows to show 'to be confirmed' rather than 'empty'", () => {
    expect(isPendingWithValue({ value: "设计师", status: "pending" })).toBe(true);
    expect(isPendingWithValue({ value: "", status: "pending" })).toBe(false);
    expect(isPendingWithValue({ value: "   ", status: "pending" })).toBe(false);
    expect(isPendingWithValue({ value: "设计师", status: "confirmed" })).toBe(false);
    expect(isPendingWithValue({ values: ["weibo"], status: "pending" })).toBe(true);
    expect(isPendingWithValue({ values: ["", " "], status: "pending" })).toBe(false);
    expect(isPendingWithValue({ value: 6, status: "pending" })).toBe(true);
    expect(isPendingWithValue({ value: 0, status: "pending" })).toBe(false);
  });
});
