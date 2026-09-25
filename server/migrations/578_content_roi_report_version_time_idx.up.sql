CREATE INDEX CONCURRENTLY IF NOT EXISTS content_roi_report_version_time_idx
    ON content_roi_report_version (workspace_id, created_at DESC);
