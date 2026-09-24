CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_adjustment_revision_deal_idx
    ON content_roi_adjustment_revision (workspace_id, deal_id);
