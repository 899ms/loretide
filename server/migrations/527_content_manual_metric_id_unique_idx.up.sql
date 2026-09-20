-- The stable key. R5 keeps uniqueness out of the create-table migration, so it
-- is a concurrent unique index here.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_manual_metric_id_unique_idx
    ON content_manual_metric (manual_metric_id);
