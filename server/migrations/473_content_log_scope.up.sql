CREATE INDEX CONCURRENTLY IF NOT EXISTS content_log_scope ON content_technical_log (workspace_id, account_id, sequence);
