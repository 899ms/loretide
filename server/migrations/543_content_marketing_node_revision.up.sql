-- specs/033 marketing nodes: the append-only revision history of a node. Each
-- row is a complete copy of the node's authored content at that revision, so
-- history needs no replay and a reschedule is the difference of two rows.
--
-- Insert only. The code has no UPDATE or DELETE statement for this table; the
-- workspace delete chain is the one place that removes rows, and a text guard
-- test (marketing_node_guards_test.go) pins that.
--
-- No foreign keys, cascades or inline key constraints (R1/R2/R5): revision_id
-- and (node_id, revision) are made distinct by two separate concurrent index
-- migrations that follow this one.
CREATE TABLE IF NOT EXISTS content_marketing_node_revision (
    revision_id          text NOT NULL,
    node_id              text NOT NULL,
    workspace_id         text NOT NULL,
    revision             bigint NOT NULL,
    change_kind          text NOT NULL
                         CHECK (change_kind IN ('create', 'edit', 'reschedule', 'confirm', 'cancel')),
    status_after         text NOT NULL,
    name                 text NOT NULL,
    kind                 text NOT NULL
                         CHECK (kind IN ('holiday', 'industry', 'brand_campaign', 'marketing')),
    -- Calendar dates in the node's own time zone, both days included.
    starts_on            date NOT NULL,
    ends_on              date NOT NULL,
    timezone             text NOT NULL,
    -- NULL means "not set" (nobody knows how long preparation takes); 0 means
    -- "no preparation needed". The two are different answers and must stay
    -- distinguishable, which is why this column is nullable.
    lead_days            integer,
    accounts             jsonb NOT NULL DEFAULT '[]'::jsonb,
    goal                 text NOT NULL DEFAULT '',
    material_source_ids  jsonb NOT NULL DEFAULT '[]'::jsonb,
    date_certainty       text NOT NULL
                         CHECK (date_certainty IN ('confirmed', 'tentative')),
    date_basis           text NOT NULL DEFAULT '',
    note                 text NOT NULL DEFAULT '',
    actor                text NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now()
);
