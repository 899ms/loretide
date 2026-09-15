// @vitest-environment node
// Canonical layer for the brand's automatic-precheck switch.
// FR map (specs/019-lt015-brand-precheck-switch/spec.md):
//   FR-002 an absent key reads as enabled, including for pre-existing brands
//   FR-004 a stored false survives a read - it is not "unset"
//   FR-009 an account cannot override the brand's value
// Contract: specs/019-lt015-brand-precheck-switch/contracts/auto-precheck.md
import {describe, it, expect} from "vitest";
import {
  AUTO_PRECHECK_SETTINGS_KEY,
  DEFAULT_AUTO_PRECHECK,
  hasStoredAutoPrecheck,
  isAutoPrecheckEnabled,
  withAutoPrecheck,
} from "./auto-precheck";

const KEY = "loretide.auto_precheck";

describe("the brand's automatic precheck switch", () => {
  it("lives under a loretide-prefixed key and defaults to on", () => {
    // The constant is mirrored by workspaceAutoPrecheckKey in
    // server/internal/handler/workspace.go. Spelling it out literally here is
    // the only thing that catches the two drifting apart.
    expect(AUTO_PRECHECK_SETTINGS_KEY).toBe(KEY);
    expect(DEFAULT_AUTO_PRECHECK).toBe(true);
  });

  // FR-002
  it("reads as on for a brand that has never set it", () => {
    expect(isAutoPrecheckEnabled({settings: {}})).toBe(true);
    expect(isAutoPrecheckEnabled({})).toBe(true);
    expect(isAutoPrecheckEnabled(undefined)).toBe(true);
  });

  it("reads as on for every shape a settings blob can arrive in", () => {
    // Each of these is reachable from a real response: a server that sent null,
    // a blob written by code that did not know this key, a value of the wrong
    // type. Per constitution principle VI the caller must not guard each one.
    for (const settings of [null, [], "settings", 7, {[KEY]: "false"}, {[KEY]: 0}, {[KEY]: null}]) {
      expect(isAutoPrecheckEnabled({settings})).toBe(true);
    }
  });

  // FR-004. The trap this feature has and the timezone one does not: false is
  // the boolean zero value. Deciding "is it set" by looking at truthiness - or
  // at whether the value is present *and* truthy - turns "switched off" into
  // "never chosen", and the default then flips it back on.
  it("reads a stored false as off, not as unset", () => {
    expect(isAutoPrecheckEnabled({settings: {[KEY]: false}})).toBe(false);
    expect(hasStoredAutoPrecheck({settings: {[KEY]: false}})).toBe(true);
  });

  it("tells a stored true apart from never having chosen", () => {
    expect(hasStoredAutoPrecheck({settings: {[KEY]: true}})).toBe(true);
    expect(hasStoredAutoPrecheck({settings: {}})).toBe(false);
    expect(hasStoredAutoPrecheck({settings: {[KEY]: "true"}})).toBe(false);
    expect(hasStoredAutoPrecheck(undefined)).toBe(false);
  });

  it("merges into the existing settings instead of replacing them", () => {
    // The endpoint replaces settings wholesale, so sending this key alone would
    // wipe the brand's timezone. This function exists so no call site has to
    // remember that.
    const before = {"loretide.timezone": "Asia/Shanghai", other: 1};
    const after = withAutoPrecheck(before, false);
    expect(after).toEqual({"loretide.timezone": "Asia/Shanghai", other: 1, [KEY]: false});
    expect(before).not.toHaveProperty(KEY);
  });

  it("starts from an empty object when the existing settings are unusable", () => {
    for (const settings of [null, undefined, [], "settings"]) {
      expect(withAutoPrecheck(settings, true)).toEqual({[KEY]: true});
    }
  });

  it("round-trips both values", () => {
    for (const enabled of [true, false]) {
      expect(isAutoPrecheckEnabled({settings: withAutoPrecheck({}, enabled)})).toBe(enabled);
    }
  });
});

// FR-009 / SC-006. The SOP is explicit: every account uses the brand-wide
// switch, with no account-level override. An account's settings blob is a
// z.record(z.string(), z.unknown()) - it will hold any key you put in it - so
// "the field does not exist" proves only that nobody has added one yet. What
// has to hold is that writing the key onto an account changes nothing.
describe("an account cannot override the brand", () => {
  it("ignores the same key written onto an account", () => {
    const brand = {settings: {[KEY]: true}};
    const account = {
      account_id: "acc-1",
      workspace_id: "ws-1",
      platform: "xiaohongshu",
      display_name: "An account",
      settings: {[KEY]: false},
    };
    // The account is not an input to the decision at all: the effective value
    // is whatever the brand says, and the account's copy is inert.
    expect(isAutoPrecheckEnabled(brand)).toBe(true);
    expect(isAutoPrecheckEnabled({settings: account.settings})).toBe(false);
    // ...which is exactly why nothing may pass an account here. The line above
    // only reads a bare settings object; no caller in the product does that for
    // an account, and the type of isAutoPrecheckEnabled does not invite it.
  });

  it("takes a workspace, so an account is not an accepted argument", () => {
    // A compile-time guarantee needs a compile-time check; this is the runtime
    // half. An account carries account_id and platform, a workspace does not -
    // and the function reads neither, so no account-shaped field can reach the
    // result.
    const source = isAutoPrecheckEnabled.toString();
    expect(source).not.toContain("account");
    expect(source).not.toContain("platform");
  });
});
