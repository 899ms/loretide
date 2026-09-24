-- specs/033 marketing nodes: the current pointer of one brand's marketing
-- node. Only status, current_revision and updated_at ever change; every
-- authored field lives in content_marketing_node_revision, one full copy per
-- revision.
--
-- No foreign keys or cascades (R1/R2): workspace isolation and deletion are
-- application responsibilities, and the workspace delete chain removes these
-- rows explicitly. No inline key constraint either (R5): node_id is made
-- distinct by the separate concurrent index migration that follows this one
-- (content_marketing_node_id_unique_idx).
CREATE TABLE IF NOT EXISTS content_marketing_node (
    node_id           text NOT NULL,
    workspace_id      text NOT NULL,
    status            text NOT NULL
                      CHECK (status IN ('unconfirmed', 'active', 'cancelled')),
    -- Written by the server, never by the client: manual or import.
    origin            text NOT NULL
                      CHECK (origin IN ('manual', 'import')),
    -- Equal to the highest revision in content_marketing_node_revision.
    current_revision  bigint NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
