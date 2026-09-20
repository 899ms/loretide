-- Three questions, one index: "list this publication record's metrics",
-- "does this record have any metrics at all" (which is the whole of the
-- pending-registration derivation), and workspace deletion. 527 leads with
-- manual_metric_id, so without a workspace_id-leading index the delete scans
-- the whole table while holding its lock.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_manual_metric_record_idx
    ON content_manual_metric (workspace_id, publication_record_id, sampled_at DESC);
