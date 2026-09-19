-- One manually curated topic card owned by a content workspace.
-- No foreign keys or cascades: workspace isolation and deletion are explicit
-- application responsibilities for content modules.
CREATE TABLE IF NOT EXISTS content_topic_card (
    topic_card_id                 text NOT NULL,
    workspace_id                  text NOT NULL,
    account_id                    text,
    audience_problem_judgment     text NOT NULL,
    ip_fit                        text NOT NULL,
    timing                        text NOT NULL,
    existing_content_relation     text NOT NULL,
    evidence_gaps_and_investment  text NOT NULL,
    channels                      jsonb NOT NULL,
    recommended_action            text NOT NULL,
    status                        text NOT NULL DEFAULT 'draft'
                                  CHECK (status IN ('draft', 'started', 'saved', 'deferred', 'dropped')),
    decision_reason               text NOT NULL DEFAULT '',
    decision_note                 text NOT NULL DEFAULT '',
    started_brief_revision_id     text,
    created_at                    timestamptz NOT NULL DEFAULT now(),
    updated_at                    timestamptz NOT NULL DEFAULT now()
);
