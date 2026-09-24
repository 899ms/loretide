CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_import_batch_time_idx
    ON content_roi_import_batch (workspace_id, created_at DESC);
