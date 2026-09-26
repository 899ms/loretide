-- What adopting a search optimization suggestion did (specs/036, contract
-- §1.4). Built in PR 2 so the state a suggestion reads as can be derived from
-- it; rows are written from PR 3, when adoption opens. Insert-only: a decision
-- may have several effects (a failed attempt, then a retry), at most one of
-- them done - checked by the application, not by a partial unique index.
--
-- version_id is the version the adoption wrote ('' when it failed);
-- failure_code repeats EffectFailures as a backstop, '' when done.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_search_suggestion_effect (
    workspace_id text NOT NULL,
    effect_id    text NOT NULL,
    decision_id  text NOT NULL,
    outcome      text NOT NULL CHECK (outcome IN ('done', 'failed')),
    version_id   text NOT NULL DEFAULT '',
    failure_code text NOT NULL DEFAULT '' CHECK (failure_code IN (
        '', 'base_moved', 'draft_unsaved', 'no_change', 'target_not_found', 'storage')),
    created_at   timestamptz NOT NULL DEFAULT now()
);
