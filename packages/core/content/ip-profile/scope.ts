// The account's material scope preference: which sources a run starts from.
//
// The single source of truth is Go: `Scopes` in
// `server/internal/content/ip-profile/scope.go`. This file is the frontend's
// copy, and `scope.test.ts` reads that file and compares, so the two cannot
// drift apart silently.
//
// The value lives at settings["loretide.scope"] on the account, the same shape
// the brand workspace's timezone uses (packages/core/workspace/timezone.ts):
// the default is filled in on READ and never written back, so "never chose" and
// "chose all" stay different states.

export const CONTENT_SCOPES = ["local", "web", "all"] as const;

export type ContentScope = (typeof CONTENT_SCOPES)[number];

/** Where the preference lives inside the account's settings JSON. The
 *  "loretide." prefix keeps it clear of any upstream key called "scope". */
export const SCOPE_SETTINGS_KEY = "loretide.scope";

/** What an account reads as when it has never chosen — including every account
 *  created before this feature. Never an empty string: the start screen renders
 *  a radio group from this value. */
export const DEFAULT_SCOPE: ContentScope = "all";

export function isContentScope(value: unknown): value is ContentScope {
  return (
    typeof value === "string" &&
    (CONTENT_SCOPES as readonly string[]).includes(value)
  );
}

/**
 * Reads the preference off an account, falling back to the default for anything
 * unreadable: no settings, settings that are not an object, a missing key, or a
 * value this build does not recognise.
 *
 * Every fallback here is reachable from a real response — an account created
 * before this feature, a server that sent settings as null, a blob written by
 * code that did not know the key — so per constitution principle VI the caller
 * does not have to guard each one.
 */
export function getAccountScope(
  account: { settings?: unknown } | undefined,
): ContentScope {
  const settings = account?.settings;
  if (typeof settings !== "object" || settings === null || Array.isArray(settings)) {
    return DEFAULT_SCOPE;
  }
  const value = (settings as Record<string, unknown>)[SCOPE_SETTINGS_KEY];
  return isContentScope(value) ? value : DEFAULT_SCOPE;
}

/**
 * Whether the account has a stored preference of its own.
 *
 * Distinct from getAccountScope, which always returns something: an account
 * that never chose and one that deliberately chose "all" read the same there.
 */
export function hasStoredScope(account: { settings?: unknown } | undefined): boolean {
  const settings = account?.settings;
  if (typeof settings !== "object" || settings === null || Array.isArray(settings)) {
    return false;
  }
  return isContentScope((settings as Record<string, unknown>)[SCOPE_SETTINGS_KEY]);
}

/**
 * Merges a scope into an existing settings object and returns a new one.
 *
 * The dedicated endpoint merges server-side, so nothing in this package needs
 * this to write. It exists for a caller that has an account in hand and wants
 * the settings it WOULD have — and it merges rather than replaces for the same
 * reason the server does: PATCH replaces the blob wholesale, so a partial
 * object sent there would delete the account's other settings.
 *
 * Throws on an unrecognised scope rather than producing a blob no reader
 * accepts.
 */
export function withAccountScope(
  settings: unknown,
  scope: string,
): Record<string, unknown> {
  if (!isContentScope(scope)) {
    throw new Error(`invalid material scope: ${scope}`);
  }
  const base =
    typeof settings === "object" && settings !== null && !Array.isArray(settings)
      ? (settings as Record<string, unknown>)
      : {};
  return { ...base, [SCOPE_SETTINGS_KEY]: scope };
}
