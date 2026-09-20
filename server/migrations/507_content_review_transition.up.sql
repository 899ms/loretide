-- Every status change of a review request or a delivery task, one row each.
-- Append-only: never UPDATEd, never DELETEd except with its workspace.
--
-- One table for two subject kinds, because the shape of a transition is
-- identical for both - who, when, from what, to what, why. Two tables would
-- only make "list everything that happened to this piece" two queries.
--
-- Why this is not the audit log: audit is the diagnostics read model. It passes
-- through Sanitize, it is cleaned on a retention schedule, and its component
-- allowlist did not carry content module names until #126. Product history -
-- "who held this, when, and why" - has to be kept verbatim and indefinitely.
-- Two different jobs should not share one pipe.
--
-- reason is required by the module for held / cancelled / failed / postponed
-- (SOP 9.2 "并注明原因") and for the explicit "keep delivering the old approved
-- snapshot" choice (SOP 9.3). It is not a column constraint: the same column is
-- required or not depending on which transition this is, and a CHECK cannot say
-- "the thing you left out is the reason".
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_review_transition (
    transition_id text NOT NULL,
    workspace_id  text NOT NULL,
    subject_kind  text NOT NULL CHECK (subject_kind IN ('review_request', 'delivery_task')),
    subject_id    text NOT NULL,
    -- '' means the subject was created. A real state: there is no status to
    -- come from on the first row.
    from_status   text NOT NULL DEFAULT '',
    to_status     text NOT NULL,
    reason        text NOT NULL DEFAULT '',
    actor_id      text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
