-- "The body of this one item", for the detail read.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_source_snapshot_source_idx
    ON content_source_snapshot (workspace_id, source_id);
