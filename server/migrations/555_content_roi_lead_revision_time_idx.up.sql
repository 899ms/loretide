CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_lead_revision_time_idx
    ON content_roi_lead_revision (workspace_id, first_seen_at);
