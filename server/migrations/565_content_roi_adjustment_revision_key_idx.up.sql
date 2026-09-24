CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_roi_adjustment_revision_key_idx
    ON content_roi_adjustment_revision (workspace_id, adjustment_id, revision);
