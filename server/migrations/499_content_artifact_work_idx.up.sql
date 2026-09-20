-- Listing a work's documents in their author's order, and the leading column
-- serves workspace deletion. 498 leads with artifact_id and can do neither.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_artifact_work_idx
    ON content_artifact (workspace_id, work_id, position);
