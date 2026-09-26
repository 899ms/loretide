CREATE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_judgement_revision_report_idx
    ON content_opdiag_judgement_revision (workspace_id, report_id, version_no);
