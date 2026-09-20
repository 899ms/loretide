-- Dropping the table loses every real number anyone copied down, and there is
-- nowhere else they exist - the system never fetched them from a platform and
-- cannot fetch them again. Stated here rather than left implicit, the same way
-- 490, 494 and 513 state it.
DROP TABLE IF EXISTS content_manual_metric;
