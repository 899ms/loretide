CREATE INDEX CONCURRENTLY IF NOT EXISTS content_audit_scope ON content_operation_audit (workspace_id, account_id, sequence);
