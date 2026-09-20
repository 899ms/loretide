-- artifact_id is the key versions reference, so it must be unique. Built
-- CONCURRENTLY and alone in this file.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_artifact_id_unique_idx
    ON content_artifact (artifact_id);
