CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_decision_key_idx
    ON content_opdiag_decision (workspace_id, decision_id);
