-- One index, two readers: workspace deletion removes every snapshot in one
-- statement and needs the leading column, and "how many times was this brief
-- revision started" reads all three.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_start_snapshot_workspace_idx
    ON content_start_snapshot (workspace_id, brief_revision_id, created_at DESC);
