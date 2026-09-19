-- Append-only frozen brief history. revision_id is the stable reference used
-- by future runs; revision is only the per-card human-readable counter.
CREATE TABLE IF NOT EXISTS content_brief_revision (
    brief_revision_id     text NOT NULL,
    topic_card_id         text NOT NULL,
    workspace_id          text NOT NULL,
    revision              bigint NOT NULL,
    audience              text NOT NULL,
    core_problem          text NOT NULL,
    claim_and_boundaries  text NOT NULL,
    channels              jsonb NOT NULL,
    format                text NOT NULL,
    structure             text NOT NULL,
    citation_requirements text NOT NULL,
    source_scope          text NOT NULL,
    deliverable           text NOT NULL,
    time_limit            text NOT NULL,
    cost_limit            text NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now()
);
