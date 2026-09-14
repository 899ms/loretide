CREATE INDEX CONCURRENTLY IF NOT EXISTS content_dispatch_outbox_due ON content_dispatch_outbox (next_attempt_at, created_at) WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;
