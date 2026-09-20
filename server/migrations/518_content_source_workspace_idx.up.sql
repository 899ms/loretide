-- "List this brand's inbox, newest first, filtered by status" and workspace
-- deletion. 517 leads with source_id, so this is the workspace_id-leading index
-- on the table.
--
-- Tag filtering rides this index and then scans within the brand: tags is a
-- text[] matched with = ANY(...), and a GIN index would be one more migration
-- and one more cleanup registration with no evidence yet that it is needed.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_source_workspace_idx
    ON content_source (workspace_id, status, captured_at DESC);
