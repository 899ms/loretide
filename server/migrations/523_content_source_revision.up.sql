-- One act of organising: who changed which fields, when.
--
-- It stores the field NAMES, not a copy of the row. The immutable half never
-- changes, so copying it into every revision would make the history fat while
-- answering nothing new; and a bulk tag over thirty items would store thirty
-- copies of thirty unchanged bodies.
--
-- One row per item per operation, including inside a bulk operation. A single
-- "bulk" row would make "when did this item get this tag" unanswerable.
--
-- Append-only, guarded the same way as the snapshot table.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_source_revision (
    revision_id    text NOT NULL,
    workspace_id   text NOT NULL,
    source_id      text NOT NULL,
    changed_fields text[] NOT NULL DEFAULT '{}',
    actor_id       text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
