-- The candidate idempotency key: one candidate per brand, node and account
-- (empty string for brand level). Repeated or concurrent syncs insert with
-- ON CONFLICT (workspace_id, node_id, account_id) DO NOTHING, which this index
-- arbitrates; there is no table constraint behind it.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_marketing_node_candidate_key_idx
    ON content_marketing_node_candidate (workspace_id, node_id, account_id);
