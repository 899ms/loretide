-- One share of one cost revision (specs/034 PR 2, R-061 "共享费用采用用户确认的
-- 分摊方式，保留原始总额，避免跨作品重复计入").
--
-- A share belongs to a revision: changing how a cost is split is a new
-- revision of the cost carrying its new shares, and the old revision's shares
-- stay exactly as they were. The cost's own amount_minor is never touched; the
-- shares of one (cost_id, cost_revision) add up to it exactly, which the write
-- transaction in Go guarantees (largest-remainder rule, FR-032) and a test
-- holds. The database does not express that sum.
--
-- target_id is a work id, an account id, a campaign label or a YYYY-MM period,
-- by target_kind. weight is set for the 'weights' method only.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_cost_allocation (
    workspace_id    text NOT NULL,
    cost_id         text NOT NULL,
    cost_revision   integer NOT NULL CHECK (cost_revision >= 1),
    target_kind     text NOT NULL CHECK (target_kind IN ('work', 'account', 'campaign_label', 'period')),
    target_id       text NOT NULL CHECK (target_id <> ''),
    method          text NOT NULL CHECK (method IN ('weights', 'amounts')),
    weight          integer CHECK (weight IS NULL OR weight > 0),
    allocated_minor bigint NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CHECK ((method = 'weights') = (weight IS NOT NULL))
);
