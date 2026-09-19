-- Stable card identifiers must be unique. Build the backing index separately
-- so creating the table never hides a non-concurrent PRIMARY KEY index build.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_topic_card_id_unique_idx
    ON content_topic_card (topic_card_id);
