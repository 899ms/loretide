-- One revision of one lead (specs/034, R-061 "线索记录可配置阶段、来源及关联
-- 标识，支持人工去重/合并并留痕").
--
-- customer_ref is a pseudonymous label the operator chose ("客户-0412"). There
-- is deliberately no column for a real name, a phone number, a messaging
-- handle, a mailbox, an identity document or a postal location (FR-016); a
-- guard test scans this file for them.
--
-- stage is free text (ruling Q5=A). merged_into non-empty means this lead was
-- merged into that one; unmerging is another revision with merged_into = ''.
--
-- Append-only, revision-based, same rules as content_roi_cost_revision.
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_lead_revision (
    workspace_id     text NOT NULL,
    lead_id          text NOT NULL,
    revision         integer NOT NULL CHECK (revision >= 1),
    voided           boolean NOT NULL DEFAULT false,
    customer_ref     text NOT NULL DEFAULT '',
    stage            text NOT NULL DEFAULT '',
    qualified        boolean NOT NULL DEFAULT false,
    first_seen_at    timestamptz NOT NULL,
    merged_into      text NOT NULL DEFAULT '',
    note             text NOT NULL DEFAULT '',
    dedupe_key       text NOT NULL DEFAULT '',
    not_duplicate_of text[] NOT NULL DEFAULT '{}',
    source_type      text NOT NULL CHECK (source_type IN ('manual', 'import')),
    import_batch_id  text NOT NULL DEFAULT '',
    recorded_by      text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);
