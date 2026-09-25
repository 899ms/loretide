CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_decision_suggestion_idx
    ON content_opdiag_decision (workspace_id, suggestion_id, suggestion_revision);
