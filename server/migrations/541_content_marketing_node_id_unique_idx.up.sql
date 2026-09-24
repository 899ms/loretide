-- Stable node identifiers must be distinct. Built separately so creating the
-- table never hides a non-concurrent key index build (R5).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_marketing_node_id_unique_idx
    ON content_marketing_node (node_id);
