CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_roi_import_batch_key_idx
    ON content_roi_import_batch (workspace_id, import_batch_id);
