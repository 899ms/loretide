-- The same flag on the publication record, and NOT resolvable by joining back
-- to content_work on purpose (specs/031 FR-015a).
--
-- When SOP 10.2's aggregates land, this column is the only thing that can tell
-- a number copied off a platform three years ago from one recorded last week.
-- Without it the first "how did we do this month" query silently includes the
-- imported history, and the join that would have prevented it is exactly the
-- join nobody remembers to write.
--
-- The table is append-only, so there is nothing to guard against here: no
-- UPDATE or DELETE path exists against it at all.
ALTER TABLE content_publication_record
    ADD COLUMN IF NOT EXISTS historical_import boolean NOT NULL DEFAULT false;
