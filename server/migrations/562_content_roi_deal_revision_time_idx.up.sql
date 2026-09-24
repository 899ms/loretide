CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_deal_revision_time_idx
    ON content_roi_deal_revision (workspace_id, closed_at);
