-- Dropping this table forgets which Idempotency-Keys were used: a client that
-- retries an import after the rollback would import it again. The imports
-- themselves stay.
DROP TABLE IF EXISTS content_roi_import_claim;
