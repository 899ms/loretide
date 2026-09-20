-- Dropping the table loses the whole disposition history: which person held a
-- task, when, and why. The rows it records exist nowhere else.
DROP TABLE IF EXISTS content_review_transition;
