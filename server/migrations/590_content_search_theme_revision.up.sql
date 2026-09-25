-- One revision of one search theme (specs/036 PR 1, R-060 "整理用户搜索问题、
-- 关键词/主题组与搜索意图，关联资料、候选选题和内容简报"; D14-V09 "搜索主题、
-- 意图、来源可追溯").
--
-- A theme is its rows: the current state is the highest revision of a
-- theme_id, and every revision is a whole copy of what a person wrote.
-- Archiving is one more revision with voided = true. A row is written once
-- and never changed (FR-010).
--
-- There is no search_volume, competition or rank column (FR-014, Q5): with no
-- search data source they are "unknown" on every read, derived, never stored.
--
-- platform repeats ipprofile.Platforms, intent SearchIntents and origin
-- ThemeOrigins as a backstop; the Go sets are the authority and
-- search_contract_test.go holds these lists to them.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2): account, material, topic card and brief ids are checked by the
-- application inside the write transaction.
CREATE TABLE IF NOT EXISTS content_search_theme_revision (
    workspace_id       text NOT NULL,
    theme_id           text NOT NULL,
    revision           integer NOT NULL CHECK (revision >= 1),
    voided             boolean NOT NULL DEFAULT false,
    name               text NOT NULL,
    platform           text NOT NULL CHECK (platform IN (
        'xiaohongshu', 'douyin', 'wechat_mp', 'bilibili', 'zhihu', 'weibo', 'kuaishou', 'shipinhao')),
    account_id         text NOT NULL DEFAULT '',
    business_goal      text NOT NULL DEFAULT '',
    questions          jsonb NOT NULL DEFAULT '[]'::jsonb,
    keywords           jsonb NOT NULL DEFAULT '[]'::jsonb,
    intent             text NOT NULL CHECK (intent IN (
        'learn', 'solve', 'compare', 'buy', 'find', 'unclassified')),
    origin             text NOT NULL CHECK (origin IN (
        'manual_keyword', 'customer_question', 'authorized_material')),
    origin_note        text NOT NULL DEFAULT '',
    source_ids         jsonb NOT NULL DEFAULT '[]'::jsonb,
    topic_card_ids     jsonb NOT NULL DEFAULT '[]'::jsonb,
    brief_revision_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    note               text NOT NULL DEFAULT '',
    recorded_by        text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);
