-- SOP 3.3's historical import needs a fourth action.
--
-- The source stays 'edited': a body pasted back in from a platform is still
-- something a person wrote, years ago. What is new is the ACTION - nobody
-- wrote this today, and recording it as 'saved' would make the version history
-- say someone composed a two-year-old article five minutes ago.
--
-- 'imported' is added to action and NOT to source. Migration 500's own comment
-- and work-editor's contract both state the source set is exactly SOP 7.1's
-- three; widening it would contradict a sentence that was written down on
-- purpose. The action set carries no such claim.
--
-- One ALTER TABLE with two actions, not two statements: the table is never
-- without a CHECK on action, not even for the instant between them.
--
-- The constraint name is PostgreSQL's own for the inline column CHECK in
-- migration 500 (<table>_<column>_check). A DROP that misses because the name
-- differs would leave the three-value constraint in place and every import
-- would fail at the database; the integration test that writes an 'imported'
-- version against a migrated database is what proves the swap landed.
ALTER TABLE content_artifact_version
    DROP CONSTRAINT IF EXISTS content_artifact_version_action_check,
    ADD CONSTRAINT content_artifact_version_action_check
        CHECK (action IN ('saved', 'restored', 'adopted', 'imported'));
