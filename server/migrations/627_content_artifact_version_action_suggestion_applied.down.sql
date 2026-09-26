-- This intentionally fails if suggestion_applied rows exist. Removing such
-- rows would rewrite append-only version history; resolve them deliberately
-- before a rollback rather than silently dropping evidence.
ALTER TABLE content_artifact_version
    DROP CONSTRAINT IF EXISTS content_artifact_version_action_check,
    ADD CONSTRAINT content_artifact_version_action_check
        CHECK (action IN ('saved', 'restored', 'adopted', 'imported'));
