CREATE INDEX CONCURRENTLY IF NOT EXISTS content_search_theme_revision_time_idx
    ON content_search_theme_revision (workspace_id, created_at);
