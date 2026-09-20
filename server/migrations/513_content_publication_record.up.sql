-- One person's statement about what this piece is doing out on a platform
-- (SOP 9.1, 9.2). Append-only: never UPDATEd, never DELETEd except with its
-- workspace.
--
-- There is no state machine here, and that is not an omission. Each row is one
-- independent observation, so any of the five statuses can be any row's value:
-- a removed after a verified_published is the ordinary case of a platform
-- taking the piece down. Forbidding it would stop people recording what
-- actually happened.
--
-- There is no "current publication status" column either. The current status is
-- the latest row. A stored one would be a second truth, and it would disagree
-- with the row stream after the first concurrent entry with nothing to notice.
--
-- There is no "待登记" value. SOP 9.2 says the workbench keeps showing 待登记
-- and "不推断平台状态" - so it is derived (a handed_off task with no row here),
-- because storing it would be the system claiming to know what the platform
-- did.
--
-- SOP 9.2 gives no enumeration for who declared it or how it was checked, so
-- this card invents none: actor_id is taken from the session, declared_by /
-- verification_note / receipt_note / platform_account / edit_note are free
-- text. The only controlled values are status and version_match, both of which
-- SOP 9.1 names. Which fields are required depends on the status and is
-- checked in code, because a CHECK cannot say "the thing you left out is the
-- page link".
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_publication_record (
    publication_record_id text NOT NULL,
    workspace_id          text NOT NULL,
    work_id               text NOT NULL,
    artifact_id           text NOT NULL,
    -- '' when someone is recording history that had no task.
    delivery_task_id      text NOT NULL DEFAULT '',
    channel               text NOT NULL CHECK (channel IN ('xiaohongshu', 'wechat_mp', 'douyin', 'shipinhao')),
    status                text NOT NULL CHECK (status IN ('reported_published', 'verified_published', 'failed', 'removed', 'unknown')),
    -- Who typed it, taken from the session. Never read from the request body:
    -- it is the one field on this row that must not be forgeable.
    actor_id              text NOT NULL,
    -- Who SAID it, free text. Often not the same person: "运营同事小王".
    declared_by           text NOT NULL DEFAULT '',
    -- SOP 9.1's "页面链接或内容 ID". Required for the two published statuses.
    page_url_or_content_id text NOT NULL DEFAULT '',
    -- SOP 9.1's "截图/回执". Also where a failed / removed row's reason goes -
    -- SOP 9.2 requires a reason and the ruled field list has no reason column.
    receipt_note          text NOT NULL DEFAULT '',
    -- How it was checked. Required for verified_published.
    verification_note     text NOT NULL DEFAULT '',
    published_at          timestamptz,
    platform_account      text NOT NULL DEFAULT '',
    platform_edited       boolean NOT NULL DEFAULT false,
    edit_note             text NOT NULL DEFAULT '',
    -- SOP 9.1 names unknown explicitly: "无法拿到完整正文时将版本匹配标为
    -- unknown". Recording "I do not know" as differs would later be read as
    -- "the platform changed it".
    version_match         text NOT NULL DEFAULT 'unknown' CHECK (version_match IN ('matched', 'differs', 'unknown')),
    created_at            timestamptz NOT NULL DEFAULT now()
);
