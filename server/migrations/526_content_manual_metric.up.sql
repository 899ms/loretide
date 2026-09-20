-- One observation a person copied off a platform (SOP 10.1).
--
-- Ten columns, because 10.1 names ten things to save: "保存平台、账号、指标名、
-- 数值、单位、统计窗口、采样时间、录入人、证据附件和数据来源类型". Each column
-- below is one of them.
--
-- value is NULLABLE and that is the whole point. 10.1: "未知填空；0 只表示已确认
-- 的零值". A platform that does not show a number leaves it NULL; a confirmed
-- zero is 0. Folding the two would put invented data into every later
-- aggregate, and nothing would report it. NULL is exactly what this column
-- needs and why it is a bigint rather than text: a text "" would make every
-- reader parse, and decide for itself what a parse failure means.
--
-- Append-only: never UPDATEd, never DELETEd except with its workspace.
-- R-044 says "更新保留历史" - a correction is a NEW row, and the reader takes
-- the latest. Rewriting the first row would erase "I copied it down wrong",
-- which is itself part of the history.
--
-- stat_window rather than `window`: WINDOW is a reserved word in PostgreSQL
-- and the column would have to be quoted at every use. Same field, legal name.
--
-- source_type is written by the SERVER, never read from a request body. Which
-- of the two endpoints was called decides it; a body field would make "where
-- did this data come from" something the caller declares, and then it is not a
-- source any more.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_manual_metric (
    manual_metric_id      text NOT NULL,
    workspace_id          text NOT NULL,
    -- The observation hangs off one publication record (10.1: "每条观测关联
    -- 发布记录"). It carries no version_id: the version is resolved on read,
    -- two hops through the delivery task and the review request.
    publication_record_id text NOT NULL,
    platform              text NOT NULL CHECK (platform IN ('xiaohongshu', 'wechat_mp', 'douyin', 'shipinhao')),
    account_id            text NOT NULL DEFAULT '',
    -- Exactly the eleven 10.1 names: 曝光、阅读、播放、完播、点赞、评论、收藏、
    -- 分享、关注、私信、转化. There is deliberately no 'other': the SOP named
    -- these and nothing else, and an escape hatch would make the set a
    -- suggestion. 'read' and 'play' stay separate values - 10.1: "不同平台的
    -- 阅读和播放分别保留，不直接合并排名".
    metric                text NOT NULL CHECK (metric IN (
                              'impression', 'read', 'play', 'completion', 'like',
                              'comment', 'favorite', 'share', 'follow',
                              'direct_message', 'conversion')),
    value                 bigint,
    unit                  text NOT NULL DEFAULT '',
    stat_window           text NOT NULL DEFAULT '',
    sampled_at            timestamptz NOT NULL,
    recorded_by           text NOT NULL,
    -- 10.1's "证据附件". The attachment itself is W-03; this phase stores what
    -- a person wrote about it and has no upload path at all.
    evidence_note         text NOT NULL DEFAULT '',
    source_type           text NOT NULL CHECK (source_type IN ('manual', 'csv_import')),
    created_at            timestamptz NOT NULL DEFAULT now()
);
