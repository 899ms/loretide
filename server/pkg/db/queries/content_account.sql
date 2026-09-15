-- Brand content accounts. Every statement here carries workspace_id, including
-- the ones that already have the primary key: isolation is a property of the
-- query, not something the caller is trusted to remember. A Get or Update that
-- matched on account_id alone would return or modify another brand's row the
-- moment a caller passed an id it found somewhere else.

-- name: CreateContentAccount :one
INSERT INTO content_account (account_id, workspace_id, platform, display_name, settings)
VALUES ($1, $2, $3, $4, $5)
RETURNING account_id, workspace_id, platform, display_name, settings, created_at, updated_at;

-- name: GetContentAccount :one
SELECT account_id, workspace_id, platform, display_name, settings, created_at, updated_at
FROM content_account
WHERE account_id = $1 AND workspace_id = $2;

-- name: ListContentAccounts :many
-- Ordered by created_at then account_id: created_at alone is not unique enough
-- to be stable when two accounts are created in the same transaction.
SELECT account_id, workspace_id, platform, display_name, settings, created_at, updated_at
FROM content_account
WHERE workspace_id = $1
ORDER BY created_at ASC, account_id ASC;

-- name: UpdateContentAccount :one
-- COALESCE keeps a partial update partial: a field the caller did not send
-- stays as it was rather than being blanked.
UPDATE content_account
SET platform     = COALESCE(sqlc.narg('platform'), platform),
    display_name = COALESCE(sqlc.narg('display_name'), display_name),
    settings     = COALESCE(sqlc.narg('settings'), settings),
    updated_at   = now()
WHERE account_id = sqlc.arg('account_id') AND workspace_id = sqlc.arg('workspace_id')
RETURNING account_id, workspace_id, platform, display_name, settings, created_at, updated_at;

-- name: CountContentAccounts :one
-- Used by the workspace-deletion test to prove the rows are actually gone.
SELECT count(*) FROM content_account WHERE workspace_id = $1;
