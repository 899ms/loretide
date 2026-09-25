-- Revision numbers under concurrency: of two writers of the same next
-- revision, one wins and the other is a 409 on base_revision.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_search_rank_observation_key_idx
    ON content_search_rank_observation_revision (workspace_id, observation_id, revision);
