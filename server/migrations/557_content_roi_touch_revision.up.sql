-- One revision of one touch: a piece of EVIDENCE about where a lead came from
-- (specs/034, R-061 第 5-7 条). It is not a judgement. The operator's
-- attribution judgement is a separate table (PR 2), so recording a judgement
-- never produces a new touch revision that looks like the evidence changed.
--
-- evidence_type is exactly six values with no "other" (FR-018). work_id may be
-- empty: "只知道是从这个账号来的" is legitimate evidence, and nothing here fills
-- a work in for the operator (FR-021).
--
-- The field-combination rules (FR-019) are enforced in Go. The CHECK below is
-- only the backstop for 'unknown', which must name nothing at all.
--
-- Append-only, revision-based. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY,
-- no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_touch_revision (
    workspace_id          text NOT NULL,
    touch_id              text NOT NULL,
    revision              integer NOT NULL CHECK (revision >= 1),
    voided                boolean NOT NULL DEFAULT false,
    lead_id               text NOT NULL,
    evidence_type         text NOT NULL CHECK (evidence_type IN (
                              'platform_linked_content', 'content_comment',
                              'customer_statement', 'dedicated_channel',
                              'account_only', 'unknown')),
    platform              text NOT NULL DEFAULT '' CHECK (platform IN (
                              '', 'xiaohongshu', 'douyin', 'wechat_mp', 'bilibili',
                              'zhihu', 'weibo', 'kuaishou', 'shipinhao')),
    account_id            text NOT NULL DEFAULT '',
    work_id               text NOT NULL DEFAULT '',
    publication_record_id text NOT NULL DEFAULT '',
    role                  text NOT NULL CHECK (role IN ('first_touch', 'pre_booking', 'other')),
    paid                  boolean NOT NULL DEFAULT false,
    occurred_at           timestamptz NOT NULL,
    evidence_note         text NOT NULL DEFAULT '',
    note                  text NOT NULL DEFAULT '',
    recorded_by           text NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    CHECK (evidence_type <> 'unknown' OR (platform = '' AND account_id = '' AND work_id = '' AND publication_record_id = ''))
);
