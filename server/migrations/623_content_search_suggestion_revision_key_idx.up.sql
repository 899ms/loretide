CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_search_suggestion_revision_key_idx
    ON content_search_suggestion_revision (workspace_id, suggestion_id, revision);
