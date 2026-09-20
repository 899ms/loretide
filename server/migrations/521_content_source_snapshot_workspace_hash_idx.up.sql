-- "Has this brand collected this exact content before" - the duplicate hint -
-- and workspace deletion. Leads with workspace_id, so one index serves both.
--
-- NOT unique: collecting the same text twice is two collection events, each
-- with its own annotation, time and person. R-011: "内容相同不删除独立的收藏
-- 上下文与批注". A unique index here would enforce the merge the SOP forbids.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_source_snapshot_workspace_hash_idx
    ON content_source_snapshot (workspace_id, content_hash);
