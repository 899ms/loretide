CREATE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_profile_proposal_revision_account_idx
    ON content_opdiag_profile_proposal_revision (workspace_id, account_id);
