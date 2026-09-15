-- Account configuration revisions. Append-only by construction: there is no
-- UPDATE and no DELETE in this file, and a test asserts that stays true. The
-- only statement that removes rows lives in workspace_delete.sql.
--
-- Every statement carries workspace_id, including the ones that already have a
-- primary key. Isolation belongs to the query: a Get that matched on
-- revision_id alone would hand another brand's persona to anyone holding an id.

-- name: InsertContentAccountRevision :one
-- The unique index on (account_id, revision) is what makes a concurrent pair of
-- writers land as two revisions instead of one: the loser gets 23505 and retries.
INSERT INTO content_account_revision
    (revision_id, account_id, workspace_id, revision, persona_prompt)
VALUES ($1, $2, $3, $4, $5)
RETURNING revision_id, account_id, workspace_id, revision, persona_prompt, created_at;

-- name: GetContentAccountRevision :one
SELECT revision_id, account_id, workspace_id, revision, persona_prompt, created_at
FROM content_account_revision
WHERE revision_id = $1 AND workspace_id = $2;

-- name: GetCurrentContentAccountRevision :one
-- The current revision is simply the highest one. No pointer column exists to
-- disagree with this.
SELECT revision_id, account_id, workspace_id, revision, persona_prompt, created_at
FROM content_account_revision
WHERE account_id = $1 AND workspace_id = $2
ORDER BY revision DESC
LIMIT 1;

-- name: ListContentAccountRevisions :many
SELECT revision_id, account_id, workspace_id, revision, persona_prompt, created_at
FROM content_account_revision
WHERE account_id = $1 AND workspace_id = $2
ORDER BY revision ASC;

-- name: NextContentAccountRevision :one
-- COALESCE so the first revision for an account is 1 rather than NULL.
SELECT COALESCE(max(revision), 0) + 1
FROM content_account_revision
WHERE account_id = $1 AND workspace_id = $2;

-- name: CountContentAccountRevisions :one
SELECT count(*) FROM content_account_revision WHERE workspace_id = $1;
