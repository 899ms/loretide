-- One thing a person said, plus what the operator made of it (SOP 10.1,
-- PRD R-045). Append-only: never UPDATEd, never DELETEd except with its
-- workspace.
--
-- redacted_excerpt and interpretation are TWO columns, because R-045 says
-- "引用摘录与运营者判断分别保存". Folded into one paragraph, nobody can tell
-- afterwards which sentence is the evidence and which is the judgement - and
-- 10.2's review is precisely the act of checking the second against the first.
-- Either column may be empty on its own.
--
-- Redaction is done by the PERSON. 10.1 says "支持脱敏摘录" - it supports
-- someone redacting, it does not redact for them. Nothing here identifies or
-- replaces personal information, and a test asserts no such path exists.
--
-- source_type is exactly the three 10.1 names: 评论、私信、线索. No 'other',
-- for the same reason the metric set has none.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_feedback_excerpt (
    feedback_excerpt_id   text NOT NULL,
    workspace_id          text NOT NULL,
    publication_record_id text NOT NULL,
    source_type           text NOT NULL CHECK (source_type IN ('comment', 'private_message', 'lead')),
    redacted_excerpt      text NOT NULL DEFAULT '',
    interpretation        text NOT NULL DEFAULT '',
    -- An array rather than a delimited string: a tag containing the delimiter
    -- would otherwise be split in half by every reader, silently.
    tags                  text[] NOT NULL DEFAULT '{}',
    occurred_at           timestamptz NOT NULL,
    recorded_by           text NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now()
);
