CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_roi_attribution_revision_key_idx
    ON content_roi_attribution_revision (workspace_id, deal_id, revision);
