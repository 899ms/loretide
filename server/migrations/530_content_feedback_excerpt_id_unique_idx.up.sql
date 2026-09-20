CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_feedback_excerpt_id_unique_idx
    ON content_feedback_excerpt (feedback_excerpt_id);
