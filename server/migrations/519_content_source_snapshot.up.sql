-- The material itself, kept apart from the row that describes collecting it.
--
-- R-010: "原文件与正文快照分别保存；每个快照保存抓取/解析时间、内容哈希…".
-- The hash is what makes SOP §4's "重复素材先提示合并关联" answerable at all:
-- without it, "have I collected this before" has no cheap answer.
--
-- Append-only. Nothing updates or deletes a snapshot - a guard test scans the
-- module for either against this table, and also asserts an INSERT exists, so
-- the guard cannot pass by the module having no SQL at all.
--
-- A pasted_text source has exactly one row here. A url source has NONE: this
-- card makes no outbound request (FR-002), so there is nothing to snapshot. The
-- consequence is that URL duplicates get no hint, which the spec records as a
-- known limit rather than papering over.
--
-- No locator columns (page / paragraph / timecode). R-010 lists them for parsed
-- material; nothing here parses, so storing empty ones would be three columns
-- that only ever hold ''.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_source_snapshot (
    snapshot_id  text NOT NULL,
    workspace_id text NOT NULL,
    source_id    text NOT NULL,
    content      text NOT NULL,
    content_hash text NOT NULL,
    captured_at  timestamptz NOT NULL DEFAULT now()
);
