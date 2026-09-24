CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_roi_cost_revision_key_idx
    ON content_roi_cost_revision (workspace_id, cost_id, revision);
