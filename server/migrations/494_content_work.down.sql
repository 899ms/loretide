-- Dropping the table loses every work, and with it the only handle on the
-- documents and versions written under it. Stated here rather than left
-- implicit, the same way 485 and 490 state it.
DROP TABLE IF EXISTS content_work;
