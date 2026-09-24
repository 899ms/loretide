-- One revision number per node. The second line of defence against concurrent
-- edits: the write path locks the node row and compares base_revision first,
-- and even a path that skipped the lock could not insert the same revision
-- number twice.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_marketing_node_revision_node_revision_idx
    ON content_marketing_node_revision (node_id, revision);
