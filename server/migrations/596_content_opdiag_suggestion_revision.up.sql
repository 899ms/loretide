-- A person's suggestion on one operating diagnosis report version
-- (specs/035 PR 3, FR-052 to FR-054; R-057 "建议可采用为账号配置修改、选题或
-- 待办").
--
-- Revisioned like the judgements: a change or a void is the next revision of
-- the same suggestion_id. target_kind is exactly three values - a topic
-- card, a todo, an account profile change proposal - and there is no
-- business memory target (FR-068). target holds that kind's parameters
-- (contract §7.3). A decision is taken on one revision (the decision table).
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_opdiag_suggestion_revision (
    workspace_id  text NOT NULL,
    suggestion_id text NOT NULL,
    revision      integer NOT NULL CHECK (revision >= 1),
    report_id     text NOT NULL,
    version_no    integer NOT NULL CHECK (version_no >= 1),
    body          text NOT NULL,
    target_kind   text NOT NULL CHECK (target_kind IN ('topic_card', 'todo', 'profile_proposal')),
    target        jsonb NOT NULL,
    judgement_ids text[] NOT NULL DEFAULT '{}',
    evidence_refs text[] NOT NULL DEFAULT '{}',
    author_kind   text NOT NULL CHECK (author_kind IN ('human')),
    voided        boolean NOT NULL DEFAULT false,
    recorded_by   text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
