CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_source_revision_id_unique_idx
    ON content_source_revision (revision_id);
