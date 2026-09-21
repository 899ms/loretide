-- SOP 5.2 item 2: "为什么适合这个 IP，引用哪些素材和过去的经营结论".
-- A topic card links source inbox items that justify why this idea fits
-- the brand.
--
-- jsonb NOT NULL DEFAULT '[]'::jsonb: the same table's channels column
-- (migration 483) is already a jsonb array, decoded with the existing
-- decodeStrings helper. Using text[] across tables would split one table
-- into two different collection representations.
--
-- No foreign keys, no cascades, no index. Reverse lookups are out of
-- scope for 030; a concurrent GIN index can be added later without data
-- migration if reverse queries are ever needed.
ALTER TABLE content_topic_card
    ADD COLUMN IF NOT EXISTS fit_source_ids jsonb NOT NULL DEFAULT '[]'::jsonb;
