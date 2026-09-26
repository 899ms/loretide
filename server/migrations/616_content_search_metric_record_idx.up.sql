-- Reading one publication record's search metrics, newest sample first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_search_metric_record_idx
    ON content_search_metric (workspace_id, publication_record_id, sampled_at);
