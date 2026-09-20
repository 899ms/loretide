-- One saved version of one document. Append-only: never UPDATEd, never
-- DELETEd except with its workspace. SOP 7.1 requires that approved, handed
-- over and published versions are always kept, and a history that can be
-- rewritten promises nothing of the sort.
--
-- source and action are TWO columns, because 7.1 says "来源与动作记录" - where
-- the content came from, and what was done. Restoring is not a fourth source:
-- what comes back is still what a person wrote, so the source stays 'edited'
-- and 'restored' is the action, with restored_from naming the version it came
-- from. Adopting sets both to 'adopted'. Collapsing the two would force a
-- reader to consult the other column to learn what happened.
--
-- 'generated' is in the source set but nothing in this phase can produce it:
-- real executors stay disabled (constitution IX), and a test asserts that no
-- path writes it. It is here so that EP-08 does not have to change the
-- controlled set - and with it every stored row's validity - to land.
--
-- revision is a per-document counter for people to read and to order by. It is
-- deliberately NOT the cross-table key: version 3 means a different thing for
-- every document. version_id is what anything else references.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_artifact_version (
    version_id    text NOT NULL,
    artifact_id   text NOT NULL,
    -- work_id is carried here as well so that listing or deleting by work does
    -- not have to go back through content_artifact.
    work_id       text NOT NULL,
    workspace_id  text NOT NULL,
    revision      bigint NOT NULL,
    source        text NOT NULL CHECK (source IN ('generated', 'edited', 'adopted')),
    action        text NOT NULL CHECK (action IN ('saved', 'restored', 'adopted')),
    body          text NOT NULL,
    -- '' unless the action was restored / adopted. A real state: most versions
    -- came from neither.
    restored_from text NOT NULL DEFAULT '',
    adopted_from  text NOT NULL DEFAULT '',
    actor_id      text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
