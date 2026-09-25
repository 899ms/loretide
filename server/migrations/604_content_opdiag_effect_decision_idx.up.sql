CREATE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_effect_decision_idx
    ON content_opdiag_effect (workspace_id, decision_id, created_at);
