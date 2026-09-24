-- One revision of one cost: money spent on content and marketing (specs/034,
-- R-061 "成本可配置为策划、人工、拍摄、剪辑、投放等类别").
--
-- Revision-based and append-only. A correction is the next revision of the
-- same cost_id; voiding is a revision with voided = true. Nothing ever UPDATEs
-- or DELETEs a row here except the workspace delete chain. A report version
-- cites (cost_id, revision), so rewriting a row would silently change a report
-- somebody already read.
--
-- Money is an integer count of the currency's minor unit plus the ISO code.
-- There is no floating point column anywhere in this feature (FR-001).
-- amount_minor is NULL exactly when pricing = 'labor_time' and no hourly rate
-- was given: the amount is "not computable", which is not zero.
--
-- There is deliberately no "counts as cost of goods sold" column (FR-012): cost
-- of goods lives only in a deal's gross basis, so one expense cannot be
-- subtracted twice.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2). The
-- (workspace_id, cost_id, revision) uniqueness is its own concurrent index.
CREATE TABLE IF NOT EXISTS content_roi_cost_revision (
    workspace_id     text NOT NULL,
    cost_id          text NOT NULL,
    revision         integer NOT NULL CHECK (revision >= 1),
    voided           boolean NOT NULL DEFAULT false,
    category         text NOT NULL,
    pricing          text NOT NULL CHECK (pricing IN ('amount', 'labor_time')),
    amount_minor     bigint,
    currency         text NOT NULL,
    labor_minutes    integer,
    labor_rate_minor bigint,
    incurred_at      timestamptz NOT NULL,
    ad_spend         boolean NOT NULL DEFAULT false,
    account_id       text NOT NULL DEFAULT '',
    work_id          text NOT NULL DEFAULT '',
    campaign_label   text NOT NULL DEFAULT '',
    evidence_note    text NOT NULL DEFAULT '',
    note             text NOT NULL DEFAULT '',
    dedupe_key       text NOT NULL DEFAULT '',
    not_duplicate_of text[] NOT NULL DEFAULT '{}',
    source_type      text NOT NULL CHECK (source_type IN ('manual', 'import')),
    import_batch_id  text NOT NULL DEFAULT '',
    recorded_by      text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    -- A direct amount always has one and never carries labor fields.
    CHECK (pricing <> 'amount' OR (amount_minor IS NOT NULL AND labor_minutes IS NULL AND labor_rate_minor IS NULL)),
    CHECK (pricing <> 'labor_time' OR labor_minutes IS NOT NULL)
);
