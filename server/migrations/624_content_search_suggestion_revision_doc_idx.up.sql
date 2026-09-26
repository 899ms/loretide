CREATE INDEX CONCURRENTLY IF NOT EXISTS content_search_suggestion_revision_doc_idx
    ON content_search_suggestion_revision (workspace_id, artifact_id, created_at);
