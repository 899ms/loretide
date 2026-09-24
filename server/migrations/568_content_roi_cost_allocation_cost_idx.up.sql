CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_cost_allocation_cost_idx
    ON content_roi_cost_allocation (workspace_id, cost_id, cost_revision);
