CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_todo_revision_key_idx
    ON content_opdiag_todo_revision (workspace_id, todo_id, revision);
