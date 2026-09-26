-- A person's decision on one suggestion revision: adopt or reject (specs/035
-- PR 3, FR-060 to FR-063; D14-V05 "拒绝建议不改配置或记忆，采纳有动作记录").
--
-- One revision takes one decision, once: a later change of mind is a new
-- suggestion revision and its own decision, and this row stays. The unique
-- index content_opdiag_decision_suggestion_idx (its own migration) is the
-- last line behind the check the code makes first. A reject writes this row
-- and the audit entry and nothing else. mode is only for adopting into a
-- topic card: create a draft card, or link an existing one.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_opdiag_decision (
    workspace_id        text NOT NULL,
    decision_id         text NOT NULL,
    suggestion_id       text NOT NULL,
    suggestion_revision integer NOT NULL CHECK (suggestion_revision >= 1),
    decision            text NOT NULL CHECK (decision IN ('adopt', 'reject')),
    mode                text NOT NULL DEFAULT '' CHECK (mode IN ('', 'create', 'link')),
    link_target_id      text NOT NULL DEFAULT '',
    note                text NOT NULL DEFAULT '',
    decided_by          text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CHECK (decision = 'adopt' OR (mode = '' AND link_target_id = ''))
);
