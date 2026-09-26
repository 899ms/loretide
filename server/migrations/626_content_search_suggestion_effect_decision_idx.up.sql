CREATE INDEX CONCURRENTLY IF NOT EXISTS content_search_suggestion_effect_decision_idx
    ON content_search_suggestion_effect (workspace_id, decision_id, created_at);
