CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_opdiag_effect_key_idx
    ON content_opdiag_effect (workspace_id, effect_id);
