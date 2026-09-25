CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_roi_import_claim_key_idx
    ON content_roi_import_claim (workspace_id, record_kind, idempotency_key);
