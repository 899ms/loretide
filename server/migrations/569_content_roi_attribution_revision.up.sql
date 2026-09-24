-- One revision of the operator's attribution judgement on one deal (specs/034
-- PR 2, R-061 "归因记录区分可确认来源、用户判断、多触点及来源不明").
--
-- This is the judgement, kept apart from the evidence: writing one never
-- writes a touch, and no touch column says which deal it was credited to
-- (FR-020). The stable key is deal_id - one deal, one judgement, revised.
--
-- touch_ids are the touches the operator accepted; 'unknown' accepts none.
-- weights is empty (no manual weights) or one positive integer per touch id;
-- both are checked in Go, and the two CHECKs below are only the backstop.
--
-- Append-only, revision-based. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY,
-- no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_attribution_revision (
    workspace_id text NOT NULL,
    deal_id      text NOT NULL,
    revision     integer NOT NULL CHECK (revision >= 1),
    voided       boolean NOT NULL DEFAULT false,
    judgement    text NOT NULL CHECK (judgement IN ('confirmed', 'operator_judgement', 'multi_touch', 'unknown')),
    touch_ids    text[] NOT NULL DEFAULT '{}',
    weights      integer[] NOT NULL DEFAULT '{}',
    note         text NOT NULL DEFAULT '',
    recorded_by  text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (judgement <> 'unknown' OR cardinality(touch_ids) = 0),
    CHECK (cardinality(weights) = 0 OR cardinality(weights) = cardinality(touch_ids))
);
