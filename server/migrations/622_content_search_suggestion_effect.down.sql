-- Dropping this table loses the record of what every adoption did: which
-- version it wrote, or why it failed. 会丢失全部采用结果记录.
DROP TABLE IF EXISTS content_search_suggestion_effect;
