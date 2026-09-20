# Issue #98 — account expression profile backend

- Date: 2026-09-19
- Issue: `#98`
- Base: `app-main` at `5d6d47f2954d2605c05c0acf1c3c813c2be3ef34`
Starting implementation: `claude/spec-021-account-expression-profile` at
`d1b9d3df63906811869d8c3539ec65283270e1be`
- Draft PR: https://github.com/899ms/loretide/pull/100
- Issue state after handoff: open, assignee `899ms`, label `review-needed`

## Scope delivered

- Migration 482 adds the profile JSON document to the existing append-only
  content-account revision.
- The Go domain type covers all SOP 3.1 fields, per-field status,
  normalization, shape validation, readiness, and neutral expression.
- Persona and profile writes carry the unchanged half forward. Only
  `ErrNotFound` means there is no history; other read failures stop the write.
- Carry-forward is refreshed after selecting each candidate revision number so
  a concurrent write is observed or forces a bounded retry instead of silently
  restoring stale configuration.
- `POST /api/content-accounts/{id}/profile` confirms a profile.
- `GET /api/content-accounts/{id}/profile` reads the current profile and
  derived decisions.
- Route-table, real-middleware, handler, and pure module tests cover the new
  contract. The content-boundary adapter allowlist includes the new handler.

## Boundaries kept

- No UI or TypeScript production code.
- No model calls, profile extraction, or real executor.
- No service restart and no business-database migration, cleanup, or test.
- No new table, foreign key, index, update query, or delete query.
- The UI remains a follow-up PR and requires manual acceptance there.

## Generated and related files

- `server/migrations/482_content_account_revision_profile.*.sql`: reversible
  schema change.
- `server/pkg/db/queries/content_account_revision.sql` and generated sqlc
  outputs: store and retrieve the profile with every revision.
- `server/internal/content/ip-profile/profile.go`: domain contract and derived
  decisions.
- `server/internal/content/ip-profile/revision.go`: append and carry-forward
  behavior.
- `server/internal/handler/content_account_profile.go`: HTTP adapter.
- `server/cmd/server/router.go`: two route registrations.
- `docs/development/account-expression-profile.md`: developer/API handoff.

## Validation policy

Database-free tests and package compilation may run locally. Database-backed
handler and real-router tests are compiled but remain pending until a dedicated
synthetic test database with migration 482 is explicitly authorized and
available. Any skipped or unrun database test is reported as such; it is not
presented as acceptance evidence.

Completed local evidence:

- `go test ./internal/content/ip-profile -count=1`: PASS=74, SKIP=0, FAIL=0.
- Targeted `go vet`: pass.
- Handler and server test packages: compile pass.
- Content boundaries, diagnostics contract, diagnostics no-upload: pass.
- `pnpm typecheck --force`: 9/9 tasks, 0 cached.
- M1, M2, and M3 compile-safe mutations each made the intended test red and
  were immediately restored.

## Review correction: raw shape before normalization

PR review identified that `SetProfile` normalized before validation. An
invalid status on an empty text/list field, or too many blank list entries,
could therefore disappear before validation. The correction separates raw
shape validation from content normalization:

- Original statuses, field lengths, list sizes, and weekly-hour range are
  validated first.
- Valid blank text and lists are still normalized to `pending`; blank channel
  entries are still removed.
- Normalized controlled-channel values are validated before any revision read
  or insert.
- A top-level JSON `null` is rejected as a non-object profile.
- Pure service tests cover invalid blank text/list statuses, an oversized
  blank-only list, no-insert behavior, and the valid confirmed-empty downgrade.
- The database-backed handler case asserts HTTP 400 and an unchanged revision
  count for unknown channels, invalid blank statuses, and top-level `null`.

No local database was used. The handler and real-router packages were compiled
without executing `TestMain`; the corrected database assertions are reserved
for the already-approved run-specific GitHub CI database.
