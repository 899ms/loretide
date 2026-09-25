CREATE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_suggestion_revision_report_idx
    ON content_opdiag_suggestion_revision (workspace_id, report_id, version_no);
