-- One revision of one deal (specs/034, R-061 "成交保留关联线索、订单标识、确认
-- 金额、退款及可用毛利依据").
--
-- gross_basis says where gross profit comes from: 'none' (unknown - business
-- ROI will be not computable), 'stated_gross_profit' (the operator typed it) or
-- 'cogs' (confirmed amount minus cost of goods, computed by the server). The
-- inputs are stored, never a number the client worked out.
--
-- Append-only, revision-based. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY,
-- no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_deal_revision (
    workspace_id       text NOT NULL,
    deal_id            text NOT NULL,
    revision           integer NOT NULL CHECK (revision >= 1),
    voided             boolean NOT NULL DEFAULT false,
    lead_id            text NOT NULL DEFAULT '',
    order_ref          text NOT NULL DEFAULT '',
    amount_minor       bigint NOT NULL,
    currency           text NOT NULL,
    closed_at          timestamptz NOT NULL,
    gross_basis        text NOT NULL CHECK (gross_basis IN ('none', 'stated_gross_profit', 'cogs')),
    gross_profit_minor bigint,
    cogs_minor         bigint,
    note               text NOT NULL DEFAULT '',
    dedupe_key         text NOT NULL DEFAULT '',
    not_duplicate_of   text[] NOT NULL DEFAULT '{}',
    source_type        text NOT NULL CHECK (source_type IN ('manual', 'import')),
    import_batch_id    text NOT NULL DEFAULT '',
    recorded_by        text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CHECK (gross_basis <> 'stated_gross_profit' OR gross_profit_minor IS NOT NULL),
    CHECK (gross_basis <> 'cogs' OR cogs_minor IS NOT NULL)
);
