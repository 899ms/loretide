-- Every read is scoped to one workspace, so this is the index every query uses.
-- CONCURRENTLY, and alone in this file: PostgreSQL refuses a concurrent index
-- build inside a transaction or a multi-statement string.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_account_workspace_idx
    ON content_account (workspace_id);
