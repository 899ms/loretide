-- Dropping this table loses every ranking observation and every revision of
-- it: the queries, conditions and evidence people recorded. 会丢失全部排名观察.
DROP TABLE IF EXISTS content_search_rank_observation_revision;
