-- One "hand this piece over" to-do (SOP 9.1).
--
-- scheduled_at is a time a PERSON reads. Nothing in this repository reads it in
-- order to act: there is no timer, no queue and no publish call. SOP 9.1's
-- "到期后产生站内待办" is a comparison made when the list is read, not an event
-- produced in the background - a test searches for any other reader of this
-- column.
--
-- handoff_method is SOP 9.1's three actions, and none of the three means the
-- piece was published: "导出成功、复制完成或交接给他人都不自动等于发布成功".
-- Whether it was published is content_publication_record's business, and a test
-- asserts that reaching handed_off produces no publication row.
--
-- review_request_id may be '': a task can be drafted while the review is still
-- pending. From ready onwards the module requires the referenced request to be
-- approved - a rule in code, because there is no foreign key to hang it on
-- (R1) and because the answer has to be "that precondition is not met", not
-- error 23503.
--
-- There is no hold_reason column. A reason belongs to the transition that
-- happened, not to how the task looks now: the same task can be held twice for
-- different reasons, and a column would only keep the last one.
--
-- No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE (R1/R2).
CREATE TABLE IF NOT EXISTS content_delivery_task (
    delivery_task_id  text NOT NULL,
    workspace_id      text NOT NULL,
    work_id           text NOT NULL,
    artifact_id       text NOT NULL,
    review_request_id text NOT NULL DEFAULT '',
    channel           text NOT NULL CHECK (channel IN ('xiaohongshu', 'wechat_mp', 'douyin', 'shipinhao')),
    status            text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ready', 'scheduled', 'handed_off', 'cancelled', 'held')),
    scheduled_at      timestamptz,
    handoff_method    text NOT NULL DEFAULT '' CHECK (handoff_method IN ('', 'export', 'copy', 'handed_to_operator')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
