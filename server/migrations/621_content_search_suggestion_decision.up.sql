-- The one decision on a search optimization suggestion (specs/036 PR 2,
-- R-060 "由用户采用或放弃"; FR-050). Insert-only: a decision is written once
-- and never changed or withdrawn. A suggestion has at most one decision; the
-- application checks first and the unique index on (workspace_id,
-- suggestion_id) - its own concurrent migration - settles a race.
--
-- suggestion_revision is the revision the person decided on. decision
-- repeats SuggestionDecisions as a backstop. PR 2 writes abandon only; adopt
-- opens in PR 3.
--
-- No PRIMARY KEY, no UNIQUE constraint (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_search_suggestion_decision (
    workspace_id        text NOT NULL,
    decision_id         text NOT NULL,
    suggestion_id       text NOT NULL,
    suggestion_revision integer NOT NULL CHECK (suggestion_revision >= 1),
    decision            text NOT NULL CHECK (decision IN ('adopt', 'abandon')),
    note                text NOT NULL DEFAULT '',
    decided_by          text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);
