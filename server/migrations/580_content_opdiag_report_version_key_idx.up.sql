CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_report_version_key_idx
    ON content_opdiag_report_version (workspace_id, report_id, version_no);
