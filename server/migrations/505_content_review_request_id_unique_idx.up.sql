-- review_request_id is what delivery tasks and transitions reference. R5 keeps
-- the uniqueness out of the create-table migration, so it is a concurrent
-- unique index here.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_review_request_id_unique_idx
    ON content_review_request (review_request_id);
