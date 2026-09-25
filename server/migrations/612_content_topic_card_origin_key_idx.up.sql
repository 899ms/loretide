CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_topic_card_origin_key_idx
    ON content_topic_card (workspace_id, origin_key);
