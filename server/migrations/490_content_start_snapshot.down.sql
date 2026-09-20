-- Dropping the table loses every snapshot that has been started. Stated here
-- rather than left implicit, the same way 485 states it for brief revisions:
-- what a run was started with cannot be reconstructed from the current
-- configuration, because the point of the snapshot is that the configuration
-- has since moved on.
DROP TABLE IF EXISTS content_start_snapshot;
