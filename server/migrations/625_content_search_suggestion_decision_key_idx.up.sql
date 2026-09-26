CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_search_suggestion_decision_key_idx
    ON content_search_suggestion_decision (workspace_id, suggestion_id);
