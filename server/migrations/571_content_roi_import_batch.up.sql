-- One import of pasted cost, lead or deal rows (specs/034 PR 3, R-061 "重复
-- 导入幂等"; FR-029, D14-V11 "可识别重复并留痕").
--
-- rows is the per-row outcome, in row order: {row, outcome, record_id,
-- duplicate_of}, outcome one of 'written', 'duplicate' (held back: it matched
-- a current record and was not confirmed) or 'confirmed_not_duplicate' (the
-- operator said it is a different record; written). Nothing is merged or
-- dropped silently - a held-back row is named here with what it matched.
--
-- A replayed request (same Idempotency-Key, same input) is answered by
-- rebuilding the original response from this row, so the row is the response
-- and is never changed afterwards. idempotency_key is kept for looking a batch
-- up; uniqueness of the key lives in content_roi_import_claim.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_import_batch (
    import_batch_id text NOT NULL,
    workspace_id    text NOT NULL,
    record_kind     text NOT NULL CHECK (record_kind IN ('cost', 'lead', 'deal')),
    row_count       integer NOT NULL CHECK (row_count >= 0),
    written_count   integer NOT NULL CHECK (written_count >= 0),
    skipped_count   integer NOT NULL CHECK (skipped_count >= 0),
    rows            jsonb NOT NULL,
    idempotency_key text NOT NULL DEFAULT '',
    recorded_by     text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (written_count + skipped_count = row_count)
);
