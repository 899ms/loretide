-- One request to have a specific version of a specific channel draft reviewed
-- for a specific account (SOP 8).
--
-- status is MUTABLE, deliberately: "list everything still waiting" is the
-- question this table is asked most, and an append-only main row would make it
-- a per-request "fetch the latest" instead of a WHERE. How the status got there
-- lives in content_review_transition, which is append-only.
--
-- snapshot is the frozen delivery snapshot and is NEVER updated. SOP 8 says the
-- thing approved is "具体渠道、具体文档版本及附件、具体交付配置的快照", and that
-- an existing approval stays with its original snapshot when the content, the
-- attachments or the target account change. Nothing here can rewrite it: no
-- write path names this column after the insert, and a test asserts it is
-- byte-for-byte identical before and after a decision.
--
-- There is no "current version" pointer. version_id names the frozen version,
-- and content_artifact_version is append-only, so however many versions the
-- author saves afterwards none of them can reach this row. That is also how
-- SOP 8's "系统不得把已通过自动套在新版本上" is kept: approved is written on a
-- row bound to one version_id, and there is no second place to read it from.
--
-- account_id is a first-class column rather than only a snapshot key, because
-- SOP 9.3 makes changing the account one of the three things that sends a
-- delivery task back to held - a question asked of many rows at once.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_review_request (
    review_request_id text NOT NULL,
    workspace_id      text NOT NULL,
    work_id           text NOT NULL,
    artifact_id       text NOT NULL,
    -- The frozen version. Only channel drafts may be submitted (SOP 8), which
    -- the module checks inside the fence; the kind is not duplicated here.
    version_id        text NOT NULL,
    account_id        text NOT NULL,
    channel           text NOT NULL CHECK (channel IN ('xiaohongshu', 'wechat_mp', 'douyin', 'shipinhao')),
    snapshot          jsonb NOT NULL,
    status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'changes_requested', 'approved', 'rejected', 'cancelled')),
    requested_by      text NOT NULL,
    requested_at      timestamptz NOT NULL DEFAULT now(),
    -- '' and the zero time until someone decides. A real state, not a
    -- placeholder: most requests spend their first hours undecided.
    decided_by        text NOT NULL DEFAULT '',
    decided_at        timestamptz,
    decision_note     text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
