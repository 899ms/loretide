CREATE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_work_mark_work_idx
    ON content_opdiag_work_mark (workspace_id, work_id, kind, item, created_at);
