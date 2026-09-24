-- Stable candidate identifiers must be distinct (R5: built separately).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_marketing_node_candidate_id_unique_idx
    ON content_marketing_node_candidate (candidate_id);
