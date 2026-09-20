CREATE INDEX CONCURRENTLY IF NOT EXISTS content_topic_card_workspace_idx ON content_topic_card (workspace_id, created_at DESC);
