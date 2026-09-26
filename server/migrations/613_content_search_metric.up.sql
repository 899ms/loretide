-- One search metric a person copied off a platform's back end (specs/036
-- PR 4, R-060 item 5 "手工登记平台提供的搜索曝光、搜索来源访问等指标"; D14-V10
-- "搜索观察保留窗口").
--
-- The same shape as 027's content_manual_metric (Q2=A), in its own table:
-- 027's Metrics is SOP 10.1's eleven names exactly and 035's performance
-- dimension iterates it, so the two search metrics do not join that set.
--
-- metric is exactly the two a platform offers (FR-071). There is no
-- search_volume, search_rank, competition or 'other': a platform that does
-- not show a number is not recorded, and a number from a third-party tool is
-- not a platform's.
--
-- value is NULLABLE: NULL is "the platform does not show me this", 0 is a
-- confirmed zero. stat_window is required here (FR-070), unlike 027.
--
-- source_type is written by the server; this version has one entry point.
--
-- platform repeats feedbacklearning.Platforms, metric SearchMetrics and
-- source_type SearchMetricSources as a backstop; the Go sets are the
-- authority and search_contract_test.go holds these lists to them.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2): the publication record and account are checked by the
-- application inside the write transaction.
CREATE TABLE IF NOT EXISTS content_search_metric (
    workspace_id          text NOT NULL,
    search_metric_id      text NOT NULL,
    publication_record_id text NOT NULL,
    platform              text NOT NULL CHECK (platform IN ('xiaohongshu', 'wechat_mp', 'douyin', 'shipinhao')),
    account_id            text NOT NULL DEFAULT '',
    metric                text NOT NULL CHECK (metric IN ('search_impression', 'search_visit')),
    value                 bigint CHECK (value >= 0),
    unit                  text NOT NULL DEFAULT '',
    stat_window           text NOT NULL CHECK (stat_window <> ''),
    sampled_at            timestamptz NOT NULL,
    evidence_note         text NOT NULL DEFAULT '',
    source_type           text NOT NULL CHECK (source_type IN ('manual')),
    recorded_by           text NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now()
);
