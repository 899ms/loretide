-- work_id is the key documents and future runs reference, so it must be
-- unique. Built CONCURRENTLY and alone in this file: PostgreSQL refuses a
-- concurrent index build inside a transaction or a multi-statement string.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_work_id_unique_idx
    ON content_work (work_id);
