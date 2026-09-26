-- Reading a publication record's observations, newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_search_rank_observation_record_idx
    ON content_search_rank_observation_revision (workspace_id, publication_record_id, observed_at);
