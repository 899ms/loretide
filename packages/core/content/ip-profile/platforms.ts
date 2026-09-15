// The controlled set of publishing platforms an account can belong to.
//
// The single source of truth is Go: `Platforms` in
// `server/internal/content/ip-profile/account.go`, backed by a CHECK
// constraint in migration 477. This file is the frontend's copy, and
// `platforms.test.ts` reads the Go source and compares value by value, so the
// two cannot drift apart silently.
//
// A copy rather than a runtime fetch is deliberate: the list changes only with
// a migration, and paying a request on every page load to learn eight constants
// buys nothing. What it would buy - "impossible to diverge" - the comparison
// test already provides.
export const CONTENT_PLATFORMS = [
  "xiaohongshu",
  "douyin",
  "wechat_mp",
  "bilibili",
  "zhihu",
  "weibo",
  "kuaishou",
  "shipinhao",
] as const;

export type ContentPlatform = (typeof CONTENT_PLATFORMS)[number];

export function isContentPlatform(value: string): value is ContentPlatform {
  return (CONTENT_PLATFORMS as readonly string[]).includes(value);
}
