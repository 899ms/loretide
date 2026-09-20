-- Dropping the table loses every publication statement anyone made, including
-- the evidence they gave for it. SOP 9.3 keeps published records; this down
-- migration does not, which is why it says so.
DROP TABLE IF EXISTS content_publication_record;
