CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_roi_touch_revision_key_idx
    ON content_roi_touch_revision (workspace_id, touch_id, revision);
