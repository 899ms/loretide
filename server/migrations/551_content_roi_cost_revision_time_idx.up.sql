CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_cost_revision_time_idx
    ON content_roi_cost_revision (workspace_id, incurred_at);
