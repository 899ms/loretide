CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_roi_lead_revision_key_idx
    ON content_roi_lead_revision (workspace_id, lead_id, revision);
