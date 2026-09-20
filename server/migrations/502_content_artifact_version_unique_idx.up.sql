-- Two jobs in one index.
--
-- It is the mutual exclusion for concurrent saves: two writers read the same
-- current revision N and both try to insert N+1, and this is what makes
-- exactly one of them fail with 23505 so the other can retry rather than both
-- landing under the same number.
--
-- It is also how a document's history is ordered and how the current revision
-- is found, which is why the document row carries no "latest version" pointer.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_artifact_version_unique_idx
    ON content_artifact_version (artifact_id, revision);
