-- One confirmed configuration snapshot for a brand content account.
--
-- Append-only: a row here is never updated and never deleted except when its
-- account's workspace is deleted. That is not tidiness - the acceptance
-- requires that old revisions stay readable and that a run which pinned one
-- keeps seeing what it pinned. A history that can be UPDATEd cannot promise
-- either; any read-modify-write would quietly rewrite the past.
--
-- revision_id is what other tables and runs reference. revision is a per-account
-- counter for people to read ("this is the third version") and for ordering; it
-- is deliberately NOT the cross-table key, because version 3 means a different
-- thing for every account.
--
-- The UNIQUE (account_id, revision) constraint lives in its own migration as a
-- CREATE UNIQUE INDEX CONCURRENTLY rather than being declared here, so building
-- it never takes a table lock.
--
-- No foreign keys and no cascades. Isolation is the application's job: every
-- query carries workspace_id, and every request is decided by
-- content/workspace-core first.
CREATE TABLE IF NOT EXISTS content_account_revision (
    revision_id    text NOT NULL PRIMARY KEY,
    account_id     text NOT NULL,
    workspace_id   text NOT NULL,
    revision       bigint NOT NULL,
    -- May be the empty string. SOP 3.1 keeps unconfirmed items pending: the
    -- creator confirmed, the persona is still blank, and that is a real
    -- revision rather than an error.
    persona_prompt text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
