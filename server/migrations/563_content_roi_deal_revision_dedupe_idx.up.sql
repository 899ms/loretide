CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_deal_revision_dedupe_idx
    ON content_roi_deal_revision (workspace_id, dedupe_key);
