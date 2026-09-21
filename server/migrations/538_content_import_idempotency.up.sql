-- One completed historical-import write response, scoped to the workspace,
-- operation and resource that produced it. No foreign keys: workspace teardown
-- explicitly removes these rows in its own transaction.
CREATE TABLE IF NOT EXISTS content_import_idempotency (
    workspace_id text NOT NULL,
    operation text NOT NULL,
    resource_scope text NOT NULL,
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL,
    response_body jsonb NOT NULL DEFAULT '{}'::jsonb,
    completed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
