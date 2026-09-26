-- One revision of one ranking observation (specs/036 PR 4, R-060 item 5
-- "排名观察须记录时间、查询词、观察条件和证据"; D2 "单次观察绝不呈现为稳定排名
-- 或全平台排名"; D14-V10 "搜索观察保留窗口与条件").
--
-- An observation is one person looking once: at observed_at, searching query
-- on platform under conditions (logged in or not, region, device, sort
-- order), and either finding the piece at position or not finding it in the
-- first scanned_depth results. "Not found" always says how far the person
-- looked, or it could not be told apart from "did not look" (FR-074).
--
-- The current state is the highest revision of an observation_id; a
-- correction or a void is one more revision (FR-073). A row is written once
-- and never changed.
--
-- theme_id, publication_record_id and account_id may be ''; when not, the
-- application checks them through adapters inside the write transaction.
--
-- platform repeats feedbacklearning.Platforms (Q8=A: the four delivery
-- channels) and result_kind RankResultKinds as a backstop; the Go sets are
-- the authority and search_contract_test.go holds these lists to them.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_search_rank_observation_revision (
    workspace_id          text NOT NULL,
    observation_id        text NOT NULL,
    revision              integer NOT NULL CHECK (revision >= 1),
    voided                boolean NOT NULL DEFAULT false,
    platform              text NOT NULL CHECK (platform IN ('xiaohongshu', 'wechat_mp', 'douyin', 'shipinhao')),
    account_id            text NOT NULL DEFAULT '',
    query                 text NOT NULL CHECK (query <> ''),
    theme_id              text NOT NULL DEFAULT '',
    publication_record_id text NOT NULL DEFAULT '',
    observed_at           timestamptz NOT NULL,
    conditions            text NOT NULL CHECK (conditions <> ''),
    result_kind           text NOT NULL CHECK (result_kind IN ('position', 'not_found')),
    position              integer,
    scanned_depth         integer,
    evidence_note         text NOT NULL CHECK (evidence_note <> ''),
    recorded_by           text NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    CHECK ((result_kind = 'position' AND position >= 1 AND scanned_depth IS NULL)
        OR (result_kind = 'not_found' AND scanned_depth >= 1 AND position IS NULL))
);
