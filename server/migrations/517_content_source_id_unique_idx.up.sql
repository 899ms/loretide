-- Identity. Separate from the table because R5 keeps UNIQUE out of CREATE TABLE
-- and R3/R4 keep each concurrent build in its own single-statement file.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_source_id_unique_idx
    ON content_source (source_id);
