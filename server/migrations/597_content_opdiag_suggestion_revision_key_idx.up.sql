CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_suggestion_revision_key_idx
    ON content_opdiag_suggestion_revision (workspace_id, suggestion_id, revision);
