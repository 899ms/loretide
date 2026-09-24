-- One claim on an import's Idempotency-Key (specs/034 PR 3, FR-028, contract
-- §1.9). Built inside feedback-learning on purpose: the module may not import
-- the idempotency module, and its dependency list is not to change.
--
-- The shape follows content_import_idempotency (migrations 538/539) with one
-- difference: that table is claimed first and then UPDATEd with the response;
-- this one is insert-only. The claim carries, at insert time, the id of the
-- import batch the same transaction writes, and a replay rebuilds the original
-- response from that batch row.
--
-- The claim is inserted in the import's own write transaction, after the
-- workspace fence. A second request with the same key waits on the unique
-- index (575) until the first ends: if the first rolls back, its claim goes
-- with it and the second proceeds; if it commits, the second reads this row.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_import_claim (
    workspace_id        text NOT NULL,
    record_kind         text NOT NULL CHECK (record_kind IN ('cost', 'lead', 'deal')),
    idempotency_key     text NOT NULL CHECK (idempotency_key <> '' AND octet_length(idempotency_key) <= 255),
    request_fingerprint text NOT NULL,
    import_batch_id     text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);
