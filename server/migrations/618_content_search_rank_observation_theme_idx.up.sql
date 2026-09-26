-- Reading a theme's observations, newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_search_rank_observation_theme_idx
    ON content_search_rank_observation_revision (workspace_id, theme_id, observed_at);
