CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_source_snapshot_id_unique_idx
    ON content_source_snapshot (snapshot_id);
