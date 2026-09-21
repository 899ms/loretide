CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_import_idempotency_scope_key_idx
    ON content_import_idempotency (workspace_id, operation, resource_scope, idempotency_key);
