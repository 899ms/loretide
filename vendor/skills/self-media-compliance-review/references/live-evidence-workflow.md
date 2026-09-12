# Live Evidence Workflow

Read this file only when the user explicitly asks for current platform content,
recent enforcement cases, or live creator discussion. Live search is opt-in for
each request. An installed API key, browser session, plugin, or Skill is not
authorization to search.

## Channel Selection

Before searching, tell the user which channel will be used.

1. If the user names a provider or tool, use only that provider or tool. Do not
   silently substitute another source if it is unavailable.
2. Otherwise prefer a target-platform browser plugin or Skill when it can read
   the requested public content.
3. TikHub is an optional REST adapter. Use only `tools/tikhub/bin/tikhub`,
   `tools/tikhub/lib/tikhub_client.py`, or `tools/xhs_dynamic_evidence.py`.
   They call documented `https://api.tikhub.io/api/v1/...` endpoints. Never use
   `mcp.tikhub.io`, a TikHub MCP server, or a TikHub MCP tool.
4. A real browser tool may access public results. Respect login, CAPTCHA,
   rate-limit, access, and platform restrictions. Do not bypass them.

A lookup may consume quota, incur fees, or access a logged-in session. If the
requested channel is unavailable or fails, continue the static compliance
review and say that live search did not complete. Search failure is not evidence
that no recent cases exist.

## Evidence Handling

Put live findings in a separate `实时平台证据（可选）` section. Record:

- provider and target platform;
- search terms and sample time;
- content IDs or URLs and publish dates when available;
- whether the source is official, media reporting, or creator discussion;
- access, sample-size, freshness, and verification limitations.

Creator posts and comments are discussion samples, not binding platform rules.
They may identify symptoms or disputed enforcement edges, but cannot determine
severity by themselves.

Keep API responses, downloaded media, signed URLs, model outputs, logs, and
reports in a gitignored local evidence directory. Never commit API keys,
cookies, authorization tokens, signed media URLs, raw creator data, account
security identifiers, or paid results.

## TikHub Configuration

Use `TIKHUB_API_KEY` from the process environment or a gitignored `.env`. China
mainland networks may explicitly set
`TIKHUB_API_BASE_URL=https://api.tikhub.dev`. Never print or place credentials
in commands, reports, fixtures, or commits.
