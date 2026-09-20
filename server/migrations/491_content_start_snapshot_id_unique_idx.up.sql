-- snapshot_id is the key a future run pins, so it must be unique. Built
-- CONCURRENTLY and alone in this file: PostgreSQL refuses a concurrent index
-- build inside a transaction or a multi-statement string.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_start_snapshot_id_unique_idx
    ON content_start_snapshot (snapshot_id);
