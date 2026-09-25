CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_search_theme_revision_key_idx
    ON content_search_theme_revision (workspace_id, theme_id, revision);
