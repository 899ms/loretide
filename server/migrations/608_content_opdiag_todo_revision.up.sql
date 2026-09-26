-- A todo of the operating diagnosis (specs/035 PR 3, ruling Q4=A; FR-032,
-- FR-064): from an adopted suggestion, or a data gap of a report version a
-- person chose to add. The todo lives in this module; the today dashboard
-- takes it up in a later card.
--
-- Revisioned: a state, title or note change, and a void, is the next
-- revision of the same todo_id. A data gap todo names the report version and
-- the gap_key it came from, and one gap has at most one todo that is not
-- voided (the code checks under a per-gap lock).
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_opdiag_todo_revision (
    workspace_id       text NOT NULL,
    todo_id            text NOT NULL,
    revision           integer NOT NULL CHECK (revision >= 1),
    title              text NOT NULL CHECK (title <> ''),
    note               text NOT NULL DEFAULT '',
    account_id         text NOT NULL DEFAULT '',
    origin_kind        text NOT NULL CHECK (origin_kind IN ('suggestion', 'data_gap')),
    origin_decision_id text NOT NULL DEFAULT '',
    origin_report_id   text NOT NULL DEFAULT '',
    origin_version_no  integer NOT NULL DEFAULT 0,
    origin_gap_key     text NOT NULL DEFAULT '',
    state              text NOT NULL CHECK (state IN ('open', 'done', 'dropped')),
    voided             boolean NOT NULL DEFAULT false,
    recorded_by        text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (origin_kind = 'suggestion' AND origin_decision_id <> '')
        OR (origin_kind = 'data_gap' AND origin_report_id <> '' AND origin_version_no >= 1 AND origin_gap_key <> '')
    )
);
