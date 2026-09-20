// @vitest-environment node
//
// Canonical home for "is this profile the account's, or is it what we show
// when we could not read it". The component suites render the outcome; the
// matrix lives here.

import { describe, expect, it } from "vitest";
import { profileIsTrustworthy, profileReadState, readinessView } from "./profile-state";
import type { ProfileQueryLike } from "./profile-state";
import { emptyProfile } from "./profile";
import type { ProfileRead } from "./profile";

function read(canStart: boolean, missing: string[]): ProfileRead {
  return {
    revisionId: "rev-1",
    revision: 1,
    profile: emptyProfile(),
    readiness: { can_start: canStart, missing },
    usesNeutralExpression: true,
  };
}

function query(overrides: Partial<ProfileQueryLike> = {}): ProfileQueryLike {
  return { enabled: true, isPending: false, isError: false, ...overrides };
}

describe("profileReadState", () => {
  it("is idle before an account is selected", () => {
    // A disabled query stays pending forever, so without the enabled flag this
    // would render a spinner that never stops.
    expect(profileReadState(query({ enabled: false, isPending: true }))).toBe("idle");
  });

  it("is loading while the first read is in flight", () => {
    expect(profileReadState(query({ isPending: true }))).toBe("loading");
  });

  it("is ready once data arrives", () => {
    expect(profileReadState(query({ data: read(true, []) }))).toBe("ready");
  });

  // The whole point of the fix: a failed read must not look like an account
  // that nobody has filled in.
  it("is failed when the read errored", () => {
    expect(profileReadState(query({ isError: true }))).toBe("failed");
  });

  it("is failed when it settled with no data", () => {
    expect(profileReadState(query({ isPending: false, data: undefined }))).toBe("failed");
  });

  it("reports failed even while a retry is in flight", () => {
    // React Query keeps isError set while refetching after a failure. Reading
    // that as "loading" would blink the error away on every retry.
    expect(profileReadState(query({ isPending: true, isError: true }))).toBe("failed");
  });
});

describe("readinessView", () => {
  it("passes the server's decision through when the read succeeded", () => {
    const view = readinessView(query({ data: read(false, ["weekly_hours"]) }));
    expect(view).toEqual({ kind: "ready", readiness: { can_start: false, missing: ["weekly_hours"] } });
  });

  // A failed read used to arrive here as can_start=false with every field
  // missing, which sent someone to go fill in a form that was already filled
  // in. It must not arrive as can_start=false with NOTHING missing either -
  // that is a blocked start with no reason given.
  it("does not turn a failed read into a readiness verdict", () => {
    expect(readinessView(query({ isError: true }))).toEqual({ kind: "failed" });
  });

  it("reports loading and idle separately", () => {
    expect(readinessView(query({ isPending: true }))).toEqual({ kind: "loading" });
    expect(readinessView(query({ enabled: false, isPending: true }))).toEqual({ kind: "idle" });
  });
});

describe("profileIsTrustworthy", () => {
  it.each([
    ["ready", query({ data: read(true, []) }), true],
    ["failed", query({ isError: true }), false],
    ["loading", query({ isPending: true }), false],
    ["idle", query({ enabled: false, isPending: true }), false],
  ])("is %s", (_name, input, expected) => {
    expect(profileIsTrustworthy(input)).toBe(expected);
  });
});
