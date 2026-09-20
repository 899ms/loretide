-- One piece of work: the container for everything produced from one topic card
-- (SOP 7). A work holds documents; the documents hold their own version
-- history. It exists so that "the post, its channel cut and its notes" are one
-- thing rather than three unrelated rows.
--
-- No status column. SOP 7.1's "document editing" row gives exactly two states,
-- working and saved, and those belong to a document's EDITING COPY, not to the
-- work. drafting / in_review / approved / handed_off / published come from
-- other rows of 7.1 and are the business of review-delivery and the handover
-- card; adding a column later is easier than changing one, so they add it.
--
-- No PRIMARY KEY and no UNIQUE: both build their index inside this statement
-- and non-concurrently (rule R5, from migration 483 onward). work_id's
-- uniqueness is migration 495.
--
-- No FOREIGN KEY and no CASCADE: the links to the topic card, the start
-- snapshot and the workspace are resolved in application code, and deletion is
-- explicit in the workspace delete transaction.
CREATE TABLE IF NOT EXISTS content_work (
    work_id       text NOT NULL,
    workspace_id  text NOT NULL,
    topic_card_id text NOT NULL,
    -- '' when the work was not started from a snapshot. A real state, not a
    -- missing value: SOP 6.2 requires that writing works with nothing else in
    -- place, so a work that never went through "start" is ordinary.
    snapshot_id   text NOT NULL,
    title         text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
