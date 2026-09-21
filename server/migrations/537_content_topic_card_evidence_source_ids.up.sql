-- SOP 5.2 item 5: "证据是否充分，存在什么缺口，需要多少研究或制作投入".
-- A topic card links source inbox items that serve as evidence or identify
-- source gaps for the topic.
--
-- jsonb NOT NULL DEFAULT '[]'::jsonb: kept as its own column rather than
-- collapsed into fit_source_ids because fit (item 2) and evidence (item 5)
-- answer two different questions. Stored as jsonb matching fit_source_ids
-- and channels on the same table.
--
-- No foreign keys, no cascades, no index.
ALTER TABLE content_topic_card
    ADD COLUMN IF NOT EXISTS evidence_source_ids jsonb NOT NULL DEFAULT '[]'::jsonb;
