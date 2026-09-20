-- Dropping the table loses every saved version, including the ones SOP 7.1
-- says are always kept. Nothing reconstructs them: the editing copy holds only
-- the latest text, and the point of a version is that the text has moved on.
DROP TABLE IF EXISTS content_artifact_version;
