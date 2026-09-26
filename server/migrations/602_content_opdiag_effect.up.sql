-- What adopting a suggestion produced (specs/035 PR 3, FR-062 to FR-065,
-- ruling Q3=A): done with the topic card, todo or proposal it made, or
-- failed with a code. Adopting into a topic card records the decision
-- first, then creates the card through topic-planning, then records this;
-- a failure writes failed, and a retry adds another row. A decision has at
-- most one done row: the code checks under a per-decision lock before it
-- writes one.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_opdiag_effect (
    workspace_id text NOT NULL,
    effect_id    text NOT NULL,
    decision_id  text NOT NULL,
    outcome      text NOT NULL CHECK (outcome IN ('done', 'failed')),
    target_kind  text NOT NULL CHECK (target_kind IN ('topic_card', 'todo', 'profile_proposal')),
    target_id    text NOT NULL DEFAULT '',
    failure_code text NOT NULL DEFAULT '' CHECK (failure_code IN ('', 'target_refused', 'target_not_found', 'storage')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (outcome = 'done' AND target_id <> '' AND failure_code = '')
        OR (outcome = 'failed' AND target_id = '' AND failure_code <> '')
    )
);
