-- version_id is the key restored_from / adopted_from and future runs
-- reference, so it must be unique. Built CONCURRENTLY and alone in this file.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_artifact_version_id_unique_idx
    ON content_artifact_version (version_id);
