-- One generated version of one brand/account operating diagnosis report
-- (specs/035 PR 1, R-057 "报告记录分析范围、时间窗口、数据完整性、原始依据";
-- D14-V05 "诊断可追溯到输入版本/观察窗口").
--
-- A row is written once, when a person generates a diagnosis, and never
-- changed: a later generation is the next version_no, and the earlier one
-- stays exactly as it was (FR-040, FR-044). params are the diagnosis
-- parameters as used (contract §4); inputs are a copy of the fields the
-- calculator needs from every input it read, each with a fingerprint
-- (contract §6), so the stored result can be recomputed without reading any
-- other table (FR-045); result is the calculator's output (contract §5.2)
-- under calc_version.
--
-- "Inputs have been updated since" is derived on read and has no column here
-- (FR-043). There is no AI judgement column and no AI judgement table
-- (FR-071): a later judgement layer keys on (report_id, version_no).
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_opdiag_report_version (
    workspace_id text NOT NULL,
    report_id    text NOT NULL,
    version_no   integer NOT NULL CHECK (version_no >= 1),
    scope_kind   text NOT NULL CHECK (scope_kind IN ('account', 'brand')),
    account_ids  text[] NOT NULL,
    title        text NOT NULL DEFAULT '',
    params       jsonb NOT NULL,
    inputs       jsonb NOT NULL,
    calc_version text NOT NULL,
    result       jsonb NOT NULL,
    created_by   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
