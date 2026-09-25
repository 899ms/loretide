CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_work_mark_key_idx
    ON content_opdiag_work_mark (workspace_id, mark_id);
