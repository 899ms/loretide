-- A human adoption of a search suggestion writes a new document version.
-- Keep source=edited; this expands only the controlled action set.
ALTER TABLE content_artifact_version
    DROP CONSTRAINT IF EXISTS content_artifact_version_action_check,
    ADD CONSTRAINT content_artifact_version_action_check
        CHECK (action IN ('saved', 'restored', 'adopted', 'imported', 'suggestion_applied'));
