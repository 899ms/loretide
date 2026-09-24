CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_touch_revision_lead_idx
    ON content_roi_touch_revision (workspace_id, lead_id);
