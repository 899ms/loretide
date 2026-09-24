-- Stable revision identifiers must be distinct (R5: built separately).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_marketing_node_revision_id_unique_idx
    ON content_marketing_node_revision (revision_id);
