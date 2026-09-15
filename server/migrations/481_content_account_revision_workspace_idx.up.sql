-- Workspace deletion removes every revision in one statement; without this it
-- would be a sequential scan of the whole table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_account_revision_workspace_idx
    ON content_account_revision (workspace_id);
