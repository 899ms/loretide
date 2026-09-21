-- Narrowing back to three values.
--
-- This FAILS if any row already carries action = 'imported', and that is the
-- correct behaviour, not a bug to work around. The precondition for rolling
-- this back is that nothing has been imported yet. Letting it pass quietly -
-- by deleting those rows, or by leaving the constraint off - would either
-- destroy a person's imported history or leave the database without the
-- backstop the Go enum is checked against.
--
-- If a rollback is genuinely needed after imports exist, the imported versions
-- have to be dealt with deliberately first. There is no automatic answer to
-- "what should happen to them", so this migration does not invent one.
ALTER TABLE content_artifact_version
    DROP CONSTRAINT IF EXISTS content_artifact_version_action_check,
    ADD CONSTRAINT content_artifact_version_action_check
        CHECK (action IN ('saved', 'restored', 'adopted'));
