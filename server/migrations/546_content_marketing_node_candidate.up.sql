-- specs/033 marketing nodes: one topic candidate per (brand, node, account).
-- Created here with the other two node tables so the migrations stay
-- contiguous and the delete chain changes once; the code that reads and writes
-- it arrives in a later PR.
--
-- No foreign keys, cascades or inline key constraints (R1/R2/R5). The
-- candidate's idempotency key (workspace_id, node_id, account_id) is enforced
-- by its own concurrent index migration, content_marketing_node_candidate_key_idx,
-- not by a table constraint.
CREATE TABLE IF NOT EXISTS content_marketing_node_candidate (
    candidate_id              text NOT NULL,
    workspace_id              text NOT NULL,
    node_id                   text NOT NULL,
    -- The empty string is the brand-level candidate. Not NULL: NULLs never
    -- compare equal in an index, so a NULL account would let brand-level
    -- candidates duplicate under the idempotency key.
    account_id                text NOT NULL DEFAULT '',
    angle                     text NOT NULL DEFAULT '',
    status                    text NOT NULL DEFAULT 'open'
                              CHECK (status IN ('open', 'adopted', 'dismissed')),
    dismiss_reason            text NOT NULL DEFAULT '',
    topic_card_id             text NOT NULL DEFAULT '',
    adopted_revision          bigint,
    impact_decision           text NOT NULL DEFAULT ''
                              CHECK (impact_decision IN ('', 'kept', 'handled')),
    impact_decision_note      text NOT NULL DEFAULT '',
    impact_decision_revision  bigint,
    impact_decided_by         text NOT NULL DEFAULT '',
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now()
);
