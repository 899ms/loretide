-- One document inside a work: the body, a channel cut, and so on. A work has
-- several, which is why documents are their own table rather than columns on
-- the work.
--
-- draft_body is the MUTABLE editing copy. Autosave writes it and produces no
-- version; versions live in content_artifact_version and are append-only. The
-- two cannot share a table: a history that can be UPDATEd promises neither
-- that old versions stay readable nor that a run which pinned one keeps seeing
-- it. This row is already mutable (title and position change), so a mutable
-- column here breaks no guard.
--
-- draft_status is SOP 7.1's pair and only that pair: saved means the editing
-- copy is byte-for-byte the latest version, working means there are changes
-- not yet saved as one. It is stored rather than recomputed because
-- recomputing means fetching the latest version's full body - N of them when
-- listing a work's documents - and the cost of storing it is an assertion on
-- each of the three paths that write it.
--
-- kind is a controlled set. The Go enum is authoritative and produces a 400
-- diagnostic error; this CHECK is the backstop for anything that reaches the
-- database another way, the same split migration 477 uses for platform.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_artifact (
    artifact_id    text NOT NULL,
    work_id        text NOT NULL,
    workspace_id   text NOT NULL,
    kind           text NOT NULL CHECK (kind IN ('body', 'channel_draft')),
    title          text NOT NULL,
    -- Explicit ordering. Two documents of the same kind under one work are
    -- legitimate, and creation time cannot say which order their author wants.
    position       bigint NOT NULL,
    draft_body     text NOT NULL DEFAULT '',
    draft_status   text NOT NULL DEFAULT 'saved' CHECK (draft_status IN ('working', 'saved')),
    draft_saved_at timestamptz NOT NULL DEFAULT now(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
