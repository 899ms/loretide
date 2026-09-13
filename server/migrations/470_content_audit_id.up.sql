CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_audit_event_id ON content_operation_audit (event_id);
