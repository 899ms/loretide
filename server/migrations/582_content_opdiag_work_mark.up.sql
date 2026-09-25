-- One person's mark on one work, for the operating diagnosis (specs/035 PR 1,
-- ruling Q2=A; R-057 "内容支柱""定位与表达一致性").
--
-- Which content pillar a work belongs to, and whether it agrees with one item
-- of an account's expression profile, are judgements. With the model runner
-- disabled nothing here makes them: a person marks, and the diagnosis only
-- counts the marks. The current mark of a (work_id, kind, item) is its latest
-- row by (created_at, mark_id); an earlier mark is never changed or removed.
--
-- kind pillar takes verdict tagged / untagged; kind consistency takes
-- consistent / inconsistent / unsure, names the account whose profile it was
-- checked against, and the profile revision the server read at that moment
-- (never one the request chose).
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_opdiag_work_mark (
    workspace_id        text NOT NULL,
    mark_id             text NOT NULL,
    work_id             text NOT NULL,
    kind                text NOT NULL CHECK (kind IN ('pillar', 'consistency')),
    item                text NOT NULL,
    verdict             text NOT NULL CHECK (verdict IN ('tagged', 'untagged', 'consistent', 'inconsistent', 'unsure')),
    account_id          text NOT NULL DEFAULT '',
    profile_revision_id text NOT NULL DEFAULT '',
    note                text NOT NULL DEFAULT '',
    recorded_by         text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (kind = 'pillar' AND verdict IN ('tagged', 'untagged'))
        OR (kind = 'consistency' AND verdict IN ('consistent', 'inconsistent', 'unsure'))
    )
);
