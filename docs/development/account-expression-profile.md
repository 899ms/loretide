# Account expression profile API

Issue #98 extends each append-only content-account revision with the complete
SOP 3.1 expression profile. It does not add a second profile entity: the profile
and `persona_prompt` are one snapshot and are always carried forward together.

## Endpoints

Both endpoints are workspace-member protected and use the account id from the
`{id}` path segment.

- `POST /api/content-accounts/{id}/profile` accepts an `ExpressionProfile`
  object and appends one revision. It returns that revision with HTTP 201.
- `GET /api/content-accounts/{id}/profile` returns the current profile plus
  derived `readiness` and `uses_neutral_expression` fields. A real account with
  no revisions returns an empty, all-`pending` profile with HTTP 200.

The request body is capped at 256 KiB. Unknown JSON fields, wrong JSON shapes,
unsupported channels, invalid statuses, out-of-range hours, and oversized
fields return the standard diagnostic 400 response without inserting a row.
Missing and cross-workspace accounts use the same 404 refusal shape.

## Revision semantics

- A profile confirmation appends a revision and carries forward the current
  `persona_prompt`.
- A persona confirmation appends a revision and carries forward the current
  profile.
- Only an explicit “no current revision” result starts with an empty other
  half. Storage or context errors abort the write.
- The candidate revision number is chosen before the other half is read. A
  concurrent writer is therefore either observed by that read or forces a
  unique-key retry; a successful later revision cannot carry stale data.
- Pre-migration rows use the database default `{}` and read as an empty,
  all-`pending` profile.

`readiness` and neutral expression are calculated from the current snapshot;
they are not stored. No model or real executor is called.

## Migration

Migration 482 adds one `jsonb NOT NULL DEFAULT '{}'::jsonb` column. It adds no
foreign key and no index because no query filters on profile fields. Workspace
deletion already removes the parent revision rows, so its manifest is unchanged.

## Validation boundary

The database-backed handler and real-router cases require a dedicated synthetic
test database with migration 482. Do not point them at a business database. The
module tests are database-free and cover normalization, validation, carry-forward,
read-error propagation, readiness, neutral expression, and bounded revision
retries.
