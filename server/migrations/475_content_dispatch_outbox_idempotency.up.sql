CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_dispatch_outbox_idempotency ON content_dispatch_outbox (idempotency_key) WHERE idempotency_key <> '';
