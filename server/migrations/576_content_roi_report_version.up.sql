-- One generated version of one ROI review report (specs/034 PR 4, R-061
-- "原始观测、计算结果、AI 推断分开保存；后续调整产生新的报告输入版本").
--
-- A row is written once, when a person generates a report, and never changed:
-- a later generation is the next version_no, and the earlier one stays exactly
-- as it was (FR-053, FR-054). params are the report parameters as used
-- (contract §5.1); inputs are the (kind, id, revision) of every record read,
-- together with a full copy of each revision's content, so the stored result
-- can be recomputed without the record tables (FR-057); result is the
-- calculator's output (contract §5.2) under calc_version.
--
-- "Inputs have been updated since" is derived on read and has no column here
-- (FR-055). There is no AI explanation column and no AI explanation table
-- (FR-061): a later explanation layer keys on (report_id, version_no).
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_roi_report_version (
    workspace_id text NOT NULL,
    report_id    text NOT NULL,
    version_no   integer NOT NULL CHECK (version_no >= 1),
    title        text NOT NULL DEFAULT '',
    params       jsonb NOT NULL,
    inputs       jsonb NOT NULL,
    calc_version text NOT NULL,
    result       jsonb NOT NULL,
    created_by   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
