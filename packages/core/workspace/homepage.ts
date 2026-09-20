// An account's public page link (SOP 3.2: "渠道设置保存账号名称或主页链接以便
// 标识").
//
// It lives at settings["loretide.homepage"] on the ACCOUNT, next to
// LT-014's loretide.scope. The name half is the account's display name and has
// existed since LT-011; this is the other half.
//
// The link is stored and never followed. Nothing in this file, in the module,
// or on the page fetches it - a guard test on the Go side scans for anything
// that could.

export const HOMEPAGE_SETTINGS_KEY = "loretide.homepage";

/** Whether a link is one a person can safely be handed.
 *
 *  Only http and https. `javascript:` and `data:` are the reason this is a
 *  check rather than a passthrough: the link goes into an anchor, and those
 *  two turn an anchor into code. `file:` is refused for the same reason it is
 *  in 028 - it points at the reader's own machine, not at an account. */
export function isWebLink(link: string): boolean {
  if (!link) return false;
  let parsed: URL;
  try {
    parsed = new URL(link);
  } catch {
    return false;
  }
  return (parsed.protocol === "http:" || parsed.protocol === "https:") && parsed.host !== "";
}

/**
 * The link stored on an account, and whether one was ever stored.
 *
 * Two fields for the reason every reader on this card has two: a link somebody
 * cleared and a link nobody ever entered both read as "", and a page that
 * wants to say "not set yet" has to be able to tell them apart.
 */
export function readHomepage(settings: unknown): { link: string; stored: boolean } {
  if (typeof settings !== "object" || settings === null || Array.isArray(settings)) {
    return { link: "", stored: false };
  }
  const value = (settings as Record<string, unknown>)[HOMEPAGE_SETTINGS_KEY];
  if (typeof value !== "string") return { link: "", stored: false };
  return { link: value, stored: true };
}

/** What the homepage endpoint takes. There is no merge helper here on purpose:
 *  the server merges, so a caller cannot forget to. */
export function homepageToRequest(link: string): Record<string, unknown> {
  return { homepage: link };
}
