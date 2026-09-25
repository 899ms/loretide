CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_judgement_revision_key_idx
    ON content_opdiag_judgement_revision (workspace_id, judgement_id, revision);
