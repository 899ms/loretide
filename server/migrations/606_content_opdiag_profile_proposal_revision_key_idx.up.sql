CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_profile_proposal_revision_key_idx
    ON content_opdiag_profile_proposal_revision (workspace_id, proposal_id, revision);
