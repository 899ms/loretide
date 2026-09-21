-- SOP 3.3: "系统保留它们的『历史导入』标识". A work built by pasting in a
-- piece that was already published carries this flag; one written here does
-- not.
--
-- Decided at creation and never changed afterwards, the same rule migration
-- 516 gives content_source.historical_import. A guard test asserts no
-- UPDATE content_work names this column in its SET clause.
--
-- boolean NOT NULL DEFAULT false: every existing work was written here, so
-- false is the correct backfill, not a placeholder. ADD COLUMN with a
-- non-volatile DEFAULT does not rewrite the table on PostgreSQL 11+.
ALTER TABLE content_work
    ADD COLUMN IF NOT EXISTS historical_import boolean NOT NULL DEFAULT false;
