-- The brand's content account: the handle that actually publishes on a
-- platform. Not a login (that is "user"), and not an IM bot installation
-- (that is channel_installation, which is keyed by agent and holds app
-- credentials). Account and ChannelAccount are the same entity, so there is
-- one table and not two layers.
--
-- Ids are text to match the columns that were already waiting for this entity:
-- content_diagnostic_run, content_operation_audit and content_technical_log
-- have carried account_id text since migration 468.
--
-- No foreign keys and no cascades. Workspace isolation is enforced in the
-- application: every query filters on workspace_id, and every request is
-- decided by content/workspace-core first.
CREATE TABLE IF NOT EXISTS content_account (
    account_id   text NOT NULL PRIMARY KEY,
    workspace_id text NOT NULL,
    -- Controlled vocabulary. The Go enum is the authority and produces the 400
    -- diagnostic error; this CHECK is the backstop for anything that reaches
    -- the table without passing through the API. Adding a platform therefore
    -- costs one migration - a deliberate trade for keeping the grouping key
    -- clean, recorded in specs/015 Assumptions.
    platform     text NOT NULL CHECK (platform IN (
        'xiaohongshu', 'douyin', 'wechat_mp', 'bilibili',
        'zhihu', 'weibo', 'kuaishou', 'shipinhao'
    )),
    display_name text NOT NULL,
    -- Expression and content-form settings. JSON so the full SOP 3.1 field
    -- list can land later without another column.
    settings     jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
