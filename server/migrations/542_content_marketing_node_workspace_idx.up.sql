-- A brand's node list, newest first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_marketing_node_workspace_idx
    ON content_marketing_node (workspace_id, created_at DESC);
