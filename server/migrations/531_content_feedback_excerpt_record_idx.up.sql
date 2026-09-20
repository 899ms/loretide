-- "List this publication record's excerpts", newest first, and workspace
-- deletion. 530 leads with feedback_excerpt_id, so this is the only
-- workspace_id-leading index on the table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_feedback_excerpt_record_idx
    ON content_feedback_excerpt (workspace_id, publication_record_id, occurred_at DESC);
