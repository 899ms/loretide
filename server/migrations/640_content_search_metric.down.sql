-- Dropping this table loses every search metric anyone copied down, and
-- there is nowhere else they exist: the system never fetched them from a
-- platform and cannot fetch them again. 会丢失全部搜索指标.
DROP TABLE IF EXISTS content_search_metric;
