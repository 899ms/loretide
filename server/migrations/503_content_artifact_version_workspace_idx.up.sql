-- Workspace deletion removes every version in one statement and needs the
-- leading column. 501 leads with version_id and 502 with artifact_id, so
-- without this the delete scans the whole table while holding its lock - and
-- this is the table that grows fastest, one row per save.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_artifact_version_workspace_idx
    ON content_artifact_version (workspace_id, artifact_id, revision DESC);
