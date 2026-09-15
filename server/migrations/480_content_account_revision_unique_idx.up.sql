-- Two jobs in one index.
--
-- It is the mutual exclusion for concurrent writes: both writers read the same
-- current revision N and try to insert N+1, and this is what makes exactly one
-- of them fail with 23505 so the other can retry rather than both landing.
--
-- It is also how the current revision is found - ORDER BY revision DESC LIMIT 1
-- for one account - which is why the account table carries no pointer column.
--
-- CONCURRENTLY, alone in this file: PostgreSQL refuses a concurrent build
-- inside a transaction or a multi-statement string.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_account_revision_account_revision_idx
    ON content_account_revision (account_id, revision);
