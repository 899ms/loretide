-- Dropping the table loses every editing copy - the text a person has typed
-- but not yet saved as a version. Versions in content_artifact_version survive
-- this statement but are orphaned by it.
DROP TABLE IF EXISTS content_artifact;
