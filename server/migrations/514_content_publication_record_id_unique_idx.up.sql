CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_publication_record_id_unique_idx
    ON content_publication_record (publication_record_id);
