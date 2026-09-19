CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_brief_revision_topic_revision_idx ON content_brief_revision (topic_card_id, revision);
