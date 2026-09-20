-- One index, two readers: listing the works of one topic card reads all three
-- columns, and workspace deletion removes every work in one statement and
-- needs the leading column. 495 leads with work_id and can serve neither, so
-- without this the delete would scan the table whole while holding its lock.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_work_card_idx
    ON content_work (workspace_id, topic_card_id, created_at DESC);
