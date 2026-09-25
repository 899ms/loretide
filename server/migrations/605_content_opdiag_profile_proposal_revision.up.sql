-- An account profile change proposal an adopted suggestion produced
-- (specs/035 PR 3, FR-065 to FR-067; D3 "仍需显式确认的账号配置修改提议").
--
-- Adopting writes revision 1, proposed, with the account's current profile
-- revision_id as base_revision_id, and writes nothing in ip-profile. Only an
-- explicit confirmation writes a new ip-profile revision, and only while the
-- current revision is still the base; the proposal's next revision is then
-- confirmed with the new revision's id. Dismissing is a revision too.
-- patches change the eight text items only: [{field, value}], 1 to 8, no
-- field twice.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_opdiag_profile_proposal_revision (
    workspace_id        text NOT NULL,
    proposal_id         text NOT NULL,
    revision            integer NOT NULL CHECK (revision >= 1),
    account_id          text NOT NULL,
    decision_id         text NOT NULL,
    base_revision_id    text NOT NULL DEFAULT '',
    patches             jsonb NOT NULL,
    state               text NOT NULL CHECK (state IN ('proposed', 'confirmed', 'dismissed')),
    applied_revision_id text NOT NULL DEFAULT '',
    voided              boolean NOT NULL DEFAULT false,
    recorded_by         text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CHECK ((state = 'confirmed') = (applied_revision_id <> ''))
);
