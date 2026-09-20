-- "List everything that happened to this request / this task", oldest first,
-- and workspace deletion. 508 leads with transition_id, so this is the only
-- workspace_id-leading index on the table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_review_transition_subject_idx
    ON content_review_transition (workspace_id, subject_kind, subject_id, created_at);
