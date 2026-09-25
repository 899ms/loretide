-- Dropping the column loses which cards an adopted diagnosis suggestion
-- created; a retry after this would create a second card. 会丢失选题卡的建卡
-- 幂等键.
ALTER TABLE content_topic_card DROP COLUMN IF EXISTS origin_key;
