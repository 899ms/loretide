-- Dropping this table loses every suggestion people wrote on diagnosis
-- report versions, with their history; the decisions taken on them would
-- point at nothing. 会丢失人写的建议.
DROP TABLE IF EXISTS content_opdiag_suggestion_revision;
