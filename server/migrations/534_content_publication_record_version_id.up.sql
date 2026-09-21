-- SOP 3.3's "发布后快照": which version of the body this publication is of.
--
-- Until now the answer took two hops - publication record to delivery task to
-- review request - because a piece that went through the normal flow always
-- has both. A historical import has neither: there was no review request and
-- no handover task, and inventing them to hang a version off would be exactly
-- the "虚构版本链" §3.3 forbids. So the record points at the version directly.
--
-- '' means "not known", the same real state delivery_task_id's '' already is.
-- Every existing record is left at '' and keeps resolving through the two
-- hops; the resolver reads this column first and only falls back (specs/031
-- FR-011a).
--
-- The table is append-only, so this points at one version forever. A later
-- save on the same document does not move it - which is the point: the
-- publication is of what was published, not of whatever the document says now.
ALTER TABLE content_publication_record
    ADD COLUMN IF NOT EXISTS version_id text NOT NULL DEFAULT '';
