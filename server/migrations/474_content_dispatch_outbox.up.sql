CREATE TABLE IF NOT EXISTS content_dispatch_outbox (
 sequence bigint GENERATED ALWAYS AS IDENTITY, item_id text NOT NULL, kind text NOT NULL, payload bytea NOT NULL,
 idempotency_key text NOT NULL DEFAULT '', workspace_id text NOT NULL DEFAULT '',
 attempt_count integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(),
 claimed_until timestamptz, delivered_at timestamptz, dead_lettered_at timestamptz, last_error text,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
