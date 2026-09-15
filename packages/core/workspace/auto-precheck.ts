// The brand's automatic-precheck switch: where it lives, what it defaults to,
// and how to write it back without destroying the rest of the settings blob.
//
// It is a key inside the workspace's settings JSON rather than a column, so
// there is no migration and no change to the upstream workspace table - the
// same shape LT-009 used for the timezone. The "loretide." prefix keeps it
// clear of any upstream key called "auto_precheck".
//
// This module is CONFIGURATION ONLY. Nothing here runs a precheck, decides when
// one is due, or reports a result; that is EP-06. What this settles is what the
// brand has asked for.
//
// One thing differs from timezone.ts and it is the whole reason this is a
// separate file: false is the boolean zero value. "Which value is stored" and
// "has a value been stored at all" cannot be the same question here, because
// answering the second by truthiness turns "switched off" into "never chosen"
// and the default then flips it back on.
//
// Contract: specs/019-lt015-brand-precheck-switch/contracts/auto-precheck.md

export const AUTO_PRECHECK_SETTINGS_KEY = "loretide.auto_precheck";

/** Brands created before this feature, and any created without choosing, read
 *  as this. The SOP is explicit that automatic precheck is on by default
 *  (docs/01, v0.5 baseline); a brand that never chose has not opted out. */
export const DEFAULT_AUTO_PRECHECK = true;

/** Narrows a settings blob to something indexable, or null if it is not.
 *  Arrays are excluded on purpose: typeof [] is "object", and an array would
 *  otherwise index as undefined rather than being recognised as unusable. */
function settingsObject(settings: unknown): Record<string, unknown> | null {
  if (typeof settings !== "object" || settings === null || Array.isArray(settings)) {
    return null;
  }
  return settings as Record<string, unknown>;
}

/**
 * Whether automatic precheck is on for this brand.
 *
 * Takes a workspace and nothing else. The first version of the SOP states that
 * every account uses the brand-wide switch with no account-level override, so
 * there is deliberately no overload, no second argument and no account-shaped
 * field read anywhere below - an account cannot reach this decision.
 *
 * Every fallback is reachable from a real response: a brand that predates this
 * feature, a server that sent settings as null, or a blob written by code that
 * did not know this key.
 */
export function isAutoPrecheckEnabled(workspace: {settings?: unknown} | undefined): boolean {
  const settings = settingsObject(workspace?.settings);
  if (settings === null) return DEFAULT_AUTO_PRECHECK;
  const value = settings[AUTO_PRECHECK_SETTINGS_KEY];
  return typeof value === "boolean" ? value : DEFAULT_AUTO_PRECHECK;
}

/**
 * Whether the brand has stored a choice of its own.
 *
 * Separate from isAutoPrecheckEnabled, which always returns something. A brand
 * that switched the precheck off and one that never chose both read as a usable
 * boolean there; only this function tells them apart. Deciding it from the
 * value would report a stored false as "not chosen" (FR-004).
 */
export function hasStoredAutoPrecheck(workspace: {settings?: unknown} | undefined): boolean {
  const settings = settingsObject(workspace?.settings);
  if (settings === null) return false;
  return typeof settings[AUTO_PRECHECK_SETTINGS_KEY] === "boolean";
}

/**
 * Merges the switch into an existing settings object and returns a new one.
 *
 * The server replaces settings wholesale, so sending this key on its own would
 * silently wipe every other setting the brand has - its timezone included. This
 * function exists so that no call site has to remember that.
 */
export function withAutoPrecheck(settings: unknown, enabled: boolean): Record<string, unknown> {
  return {...(settingsObject(settings) ?? {}), [AUTO_PRECHECK_SETTINGS_KEY]: enabled};
}
