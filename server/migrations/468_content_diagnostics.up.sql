CREATE TABLE IF NOT EXISTS content_diagnostic_run (
 run_id text NOT NULL, workspace_id text NOT NULL, account_id text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), payload jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS content_operation_audit (
 sequence bigint GENERATED ALWAYS AS IDENTITY, event_id text NOT NULL,
 workspace_id text NOT NULL, account_id text NOT NULL DEFAULT '',
 received_at timestamptz NOT NULL DEFAULT now(), payload jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS content_technical_log (
 sequence bigint GENERATED ALWAYS AS IDENTITY, event_id text NOT NULL,
 workspace_id text NOT NULL, account_id text NOT NULL DEFAULT '',
 received_at timestamptz NOT NULL DEFAULT now(), payload jsonb NOT NULL
);
