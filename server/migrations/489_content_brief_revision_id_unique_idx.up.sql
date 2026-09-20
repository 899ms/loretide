-- Stable brief revision identifiers are external references. Keep their
-- uniqueness index concurrent and independent from the table migration.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_brief_revision_id_unique_idx
    ON content_brief_revision (brief_revision_id);
