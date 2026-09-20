CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_delivery_task_id_unique_idx
    ON content_delivery_task (delivery_task_id);
