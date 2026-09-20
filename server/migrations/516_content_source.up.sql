-- One act of collecting something (SOP §4 step 1).
--
-- Five columns are set once and never change: kind, url, captured_at,
-- recorded_by and historical_import. They describe the collection event, and an
-- event does not get re-collected. A guard test scans the module for any UPDATE
-- whose SET list names one of them.
--
-- historical_import is §3.3's "历史导入" marker: the system keeps it for content
-- brought in from before. It is decided when the row is created because it is a
-- fact about where the material came from, not a label someone applies later.
--
-- annotation and personal_judgement are deliberately two columns, not one.
-- §4 step 4: "原文、自动摘要和个人判断分开保存". The first is what the person
-- noted while collecting; the second is why they kept it. Merging them would
-- make "why is this here" unanswerable without reading prose. The third of the
-- three - the automatic summary - has no column: this card runs no parser.
--
-- The body does NOT live here. R-010 says "原文件与正文快照分别保存", so the
-- pasted text goes to content_source_snapshot with its hash.
--
-- No parse-status column. SOP §7.1 lists pending → processing → ready / partial
-- / failed, but nothing in this card would ever advance it; the column arrives
-- with the parser (spec Out of Scope 1).
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_source (
    source_id          text NOT NULL,
    workspace_id       text NOT NULL,
    kind               text NOT NULL CHECK (kind IN ('pasted_text', 'url')),
    url                text NOT NULL DEFAULT '',
    captured_at        timestamptz NOT NULL DEFAULT now(),
    recorded_by        text NOT NULL,
    historical_import  boolean NOT NULL DEFAULT false,
    title              text NOT NULL DEFAULT '',
    tags               text[] NOT NULL DEFAULT '{}',
    annotation         text NOT NULL DEFAULT '',
    personal_judgement text NOT NULL DEFAULT '',
    status             text NOT NULL DEFAULT 'inbox' CHECK (status IN ('inbox', 'organized', 'archived')),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
