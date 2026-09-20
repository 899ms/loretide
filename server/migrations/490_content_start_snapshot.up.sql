-- One "start": the configuration that was in force at the moment a brief
-- revision was taken up for work (SOP EP-04b). Append-only: a snapshot exists
-- so that changing the account, the preference, the brief or the brand switch
-- afterwards cannot rewrite what a run was started with.
--
-- snapshot_id is the stable key a future run pins; its uniqueness is a separate
-- CONCURRENTLY index (491) rather than a PRIMARY KEY, because a PRIMARY KEY
-- builds its index non-concurrently inside this statement (rule R5 from
-- migration 483 onward).
--
-- No revision counter. content_brief_revision needs one because SOP 5.3 shows
-- the reader "which version"; a snapshot has no such need, and adding one would
-- invite reading it as a cross-table key.
--
-- No FOREIGN KEY and no CASCADE: relationships to the card, the brief revision,
-- the account and the workspace are resolved in application code, and deletion
-- is explicit in the workspace delete transaction.
--
-- project_id is '' when there is no project. The empty string is a real state,
-- not a missing value, the same reading ResourceRef.Account already has.
CREATE TABLE IF NOT EXISTS content_start_snapshot (
    snapshot_id       text NOT NULL,
    workspace_id      text NOT NULL,
    topic_card_id     text NOT NULL,
    brief_revision_id text NOT NULL,
    account_id        text NOT NULL,
    project_id        text NOT NULL,
    actor_id          text NOT NULL,
    snapshot          jsonb NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);
