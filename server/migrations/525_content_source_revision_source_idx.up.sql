-- "How was this item organised", oldest first, and workspace deletion.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_source_revision_source_idx
    ON content_source_revision (workspace_id, source_id, created_at);
