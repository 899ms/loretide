-- The stable key of a search metric. R5 keeps uniqueness out of the
-- create-table migration, so it is a concurrent unique index here.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_search_metric_id_idx
    ON content_search_metric (workspace_id, search_metric_id);
