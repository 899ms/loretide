-- The topic card detail page shows how many times this card has been started.
-- Without this the card-scoped listing is a scan of every snapshot in the
-- workspace; 492 leads with brief_revision_id and cannot serve it.
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_start_snapshot_card_idx
    ON content_start_snapshot (workspace_id, topic_card_id, created_at DESC);
