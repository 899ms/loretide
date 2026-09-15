// The brand workspace's timezone: where it lives, what it defaults to, and how
// to write it back without destroying the rest of the settings blob.
//
// It is a key inside the workspace's settings JSON rather than a column, so
// there is no migration and no change to the upstream workspace table. The key
// carries a "loretide." prefix so a future upstream setting called "timezone"
// cannot collide with it (spec FR-007, clarify 2026-09-14).
//
// Timestamps stay in UTC. This value says how to DISPLAY them and, later, how
// to schedule against them; it never rewrites stored data.
//
// Contract: specs/004-lt009-brand-workspace/contracts/workspace-timezone.md

export const TIMEZONE_SETTINGS_KEY = "loretide.timezone";

/** Workspaces created before this feature, and any created without choosing,
 *  read as this. Never an empty string: the panel would have nothing to show
 *  and the caller nothing to schedule against (FR-002). */
export const DEFAULT_TIMEZONE = "Asia/Shanghai";

/**
 * True only for names the runtime recognises as IANA zones.
 *
 * Intl throws RangeError for an unknown zone, which is the only way to ask
 * without shipping a zone table. A fixed offset like "+08:00" is rejected on
 * purpose: the server validates with time.LoadLocation, which does not accept
 * offsets either, so accepting it here would let the client offer a value the
 * server refuses.
 */
export function isValidTimezone(name: string): boolean {
  if (typeof name !== "string" || name.trim() === "") return false;
  // Intl accepts a fixed offset such as "+08:00" as a time zone; Go's
  // time.LoadLocation, which the server validates with, does not. Accepting it
  // here would let the picker offer a value the server answers 400 to, so the
  // narrower of the two rules wins. Verified against both runtimes.
  if (/^[+-]/.test(name.trim())) return false;
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: name });
    return true;
  } catch {
    return false;
  }
}

/**
 * Reads the timezone off a workspace, falling back to the default for anything
 * unreadable.
 *
 * Every fallback case here is reachable from a real response: a workspace that
 * predates this feature, a server that sent settings as null, or a blob written
 * by code that did not know about this key. Per constitution principle VI the
 * caller must not have to guard each of those itself.
 */
export function getWorkspaceTimezone(workspace: {settings?: unknown} | undefined): string {
  const settings = workspace?.settings;
  if (typeof settings !== "object" || settings === null || Array.isArray(settings)) {
    return DEFAULT_TIMEZONE;
  }
  const value = (settings as Record<string, unknown>)[TIMEZONE_SETTINGS_KEY];
  return typeof value === "string" && isValidTimezone(value) ? value : DEFAULT_TIMEZONE;
}

/**
 * Merges a timezone into an existing settings object and returns a new one.
 *
 * The server replaces settings wholesale (handler/workspace.go marshals the
 * request's settings straight over the stored value), so sending just the
 * timezone key would silently wipe every other setting the workspace has. This
 * function exists so that no call site has to remember that.
 *
 * Throws on an invalid zone rather than writing it: a settings blob carrying a
 * zone nothing can resolve is worse than a rejected edit.
 */
export function withWorkspaceTimezone(settings: unknown, timezone: string): Record<string, unknown> {
  if (!isValidTimezone(timezone)) {
    throw new Error(`invalid timezone: ${timezone}`);
  }
  const base =
    typeof settings === "object" && settings !== null && !Array.isArray(settings)
      ? (settings as Record<string, unknown>)
      : {};
  return {...base, [TIMEZONE_SETTINGS_KEY]: timezone};
}

/**
 * The zones a picker may offer.
 *
 * Intl.supportedValuesOf is the runtime's own list, so it can never drift from
 * what isValidTimezone accepts. Older runtimes do not have it; the fallback is
 * a short list of common zones plus the default, which keeps the picker usable
 * rather than empty. Filtered through isValidTimezone so the two rules cannot
 * disagree even if the runtime offers something the server would refuse.
 */
export function supportedTimezones(): string[] {
  const intl = Intl as unknown as {supportedValuesOf?: (key: string) => string[]};
  let zones: string[];
  try {
    zones = intl.supportedValuesOf?.("timeZone") ?? [];
  } catch {
    zones = [];
  }
  if (zones.length === 0) {
    zones = [
      "Asia/Shanghai", "Asia/Hong_Kong", "Asia/Taipei", "Asia/Tokyo", "Asia/Seoul",
      "Asia/Singapore", "Europe/London", "Europe/Paris", "Europe/Berlin",
      "America/New_York", "America/Chicago", "America/Los_Angeles", "UTC",
    ];
  }
  const usable = zones.filter(isValidTimezone);
  return usable.includes(DEFAULT_TIMEZONE) ? usable : [DEFAULT_TIMEZONE, ...usable];
}

/**
 * Whether the workspace has a usable timezone of its own.
 *
 * Distinct from getWorkspaceTimezone, which always returns something: a
 * workspace that never chose a zone and one that deliberately chose
 * Asia/Shanghai read the same there, and the settings page has to tell them
 * apart to label one of them "default" (FR-002).
 */
export function hasStoredTimezone(workspace: {settings?: unknown} | undefined): boolean {
  const settings = workspace?.settings;
  if (typeof settings !== "object" || settings === null || Array.isArray(settings)) {
    return false;
  }
  const value = (settings as Record<string, unknown>)[TIMEZONE_SETTINGS_KEY];
  return typeof value === "string" && isValidTimezone(value);
}
