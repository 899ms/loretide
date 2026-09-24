CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_cost_revision_dedupe_idx
    ON content_roi_cost_revision (workspace_id, dedupe_key);
