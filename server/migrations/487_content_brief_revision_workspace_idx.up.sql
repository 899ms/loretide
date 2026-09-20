CREATE INDEX CONCURRENTLY IF NOT EXISTS content_brief_revision_workspace_idx ON content_brief_revision (workspace_id, topic_card_id, revision DESC);
