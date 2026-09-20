-- Dropping the table loses every review request and with it every frozen
-- delivery snapshot - the only record of what was approved. Stated here rather
-- than left implicit, the same way 490 and 494 state it.
DROP TABLE IF EXISTS content_review_request;
