-- One revision of one refund or adjustment on a deal (specs/034, R-061
-- "退款/调整记录").
--
-- revenue_delta_minor is a REDUCTION: a refund of 1000.00 CNY is +100000.
-- An adjustment may be either sign. gross_delta_minor NULL means the operator
-- did not say how gross profit moved, and then that deal's gross profit is not
-- computable (ruling Q3=A) - nothing guesses a proportional share.
--
-- currency must equal the deal's; that is checked in Go.
--
-- Append-only, revision-based. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY,
-- no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_adjustment_revision (
    workspace_id        text NOT NULL,
    adjustment_id       text NOT NULL,
    revision            integer NOT NULL CHECK (revision >= 1),
    voided              boolean NOT NULL DEFAULT false,
    deal_id             text NOT NULL,
    kind                text NOT NULL CHECK (kind IN ('refund', 'adjustment')),
    revenue_delta_minor bigint NOT NULL,
    gross_delta_minor   bigint,
    currency            text NOT NULL,
    occurred_at         timestamptz NOT NULL,
    note                text NOT NULL DEFAULT '',
    recorded_by         text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);
