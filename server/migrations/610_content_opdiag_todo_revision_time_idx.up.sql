CREATE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_todo_revision_time_idx
    ON content_opdiag_todo_revision (workspace_id, created_at DESC);
