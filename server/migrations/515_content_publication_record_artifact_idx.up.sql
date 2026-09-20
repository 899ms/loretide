-- "The current status is the latest row" reads through this index, and so does
-- workspace deletion. 514 leads with publication_record_id, so this is the only
-- workspace_id-leading index on the table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_publication_record_artifact_idx
    ON content_publication_record (workspace_id, artifact_id, created_at DESC);
