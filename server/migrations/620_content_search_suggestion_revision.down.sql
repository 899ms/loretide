-- Dropping this table loses every search optimization suggestion and every
-- revision of it: the target questions, rationales and proposed bodies people
-- wrote. 会丢失全部搜索优化建议.
DROP TABLE IF EXISTS content_search_suggestion_revision;
