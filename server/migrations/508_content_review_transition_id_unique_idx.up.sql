CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_review_transition_id_unique_idx
    ON content_review_transition (transition_id);
