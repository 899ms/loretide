-- One revision of one search optimization suggestion (specs/036 PR 2, R-060
-- "输出可编辑建议、依据和修改差异"; D14-V10 "搜索优化建议可比较、采用和放弃").
--
-- A suggestion is its rows: the current state is the highest revision of a
-- suggestion_id, and every revision is a whole copy. A row is written once and
-- never changed. work_id, artifact_id, base_version_id and theme_id are the
-- same on every revision of a suggestion (FR-030); theme_revision is written
-- by the server: the theme's current revision when this revision was written.
--
-- proposed_body is the whole edited body (FR-034). The difference against the
-- base version is computed on read (FR-035): there is no diff column, and no
-- state, base_is_current or theme_changed column either - those are derived
-- on read (FR-038). There is no score, rank, density or keyword count column
-- (FR-036).
--
-- aspects repeats SuggestionAspects and author_kind AuthorKinds as a
-- backstop; the Go sets are the authority and search_suggestion_test.go holds
-- these lists to them. voided is the revision tables' common column
-- (contract §1); nothing in this version sets it.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2): the theme, material and work version ids are checked by the
-- application inside the write transaction.
CREATE TABLE IF NOT EXISTS content_search_suggestion_revision (
    workspace_id        text NOT NULL,
    suggestion_id       text NOT NULL,
    revision            integer NOT NULL CHECK (revision >= 1),
    voided              boolean NOT NULL DEFAULT false,
    work_id             text NOT NULL,
    artifact_id         text NOT NULL,
    base_version_id     text NOT NULL,
    theme_id            text NOT NULL,
    theme_revision      integer NOT NULL CHECK (theme_revision >= 1),
    target_question     text NOT NULL,
    aspects             text[] NOT NULL CHECK (
        cardinality(aspects) >= 1 AND aspects <@ ARRAY['title', 'body', 'topics', 'description']::text[]),
    rationale           text NOT NULL,
    evidence_source_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    proposed_body       text NOT NULL,
    author_kind         text NOT NULL CHECK (author_kind IN ('human')),
    recorded_by         text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);
