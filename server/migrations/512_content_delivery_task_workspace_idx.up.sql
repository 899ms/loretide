-- "List this brand's tasks, newest first, filtered by status" and workspace
-- deletion. 511 leads with delivery_task_id, so this is the only
-- workspace_id-leading index on the table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_delivery_task_workspace_idx
    ON content_delivery_task (workspace_id, status, created_at DESC);
