-- specs/035 PR 3, ruling Q3 supplement (contract §7.4): the key a topic card
-- was created under by topic-planning's CreateOnce - for an adopted
-- operating diagnosis suggestion, opdiag-suggestion:<suggestion_id> - so a
-- retry after a failure between creating the card and recording the outcome
-- finds that card instead of creating a second one.
--
-- Nullable, and NULL for every card Create writes: existing cards and
-- existing behaviour are unchanged. The API response does not carry it.
-- Uniqueness per workspace is content_topic_card_origin_key_idx, its own
-- concurrent migration; NULLs never collide.
--
-- No foreign keys, no cascades, no index here (as 536 / 537).
ALTER TABLE content_topic_card
    ADD COLUMN IF NOT EXISTS origin_key text;
