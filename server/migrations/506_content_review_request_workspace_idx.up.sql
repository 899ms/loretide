-- "List this document's review history" and workspace deletion, one index.
-- 505 leads with review_request_id, so without a workspace_id-leading index the
-- delete scans the whole table while holding its lock.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_review_request_workspace_idx
    ON content_review_request (workspace_id, artifact_id, created_at DESC);
