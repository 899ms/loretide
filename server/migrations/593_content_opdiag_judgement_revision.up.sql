-- A person's judgement, alternative explanation or limitation on one
-- operating diagnosis report version (specs/035 PR 3, FR-050 to FR-054;
-- R-057 "AI 判断、替代解释、局限", written by a person in this version).
--
-- Revisioned: a change, and a void, is the next revision of the same
-- judgement_id, and an earlier revision is never changed. The current state
-- is the highest revision. basis evidence cites reference keys of that
-- version's result (evidence_refs, at least one); basis qualitative cites
-- none, and the page says it has no data behind it. author_kind is human
-- and nothing else: an AI author needs a migration that widens the CHECK.
--
-- Append-only. No PRIMARY KEY, no UNIQUE (R5), no FOREIGN KEY, no CASCADE
-- (R1/R2).
CREATE TABLE IF NOT EXISTS content_opdiag_judgement_revision (
    workspace_id       text NOT NULL,
    judgement_id       text NOT NULL,
    revision           integer NOT NULL CHECK (revision >= 1),
    report_id          text NOT NULL,
    version_no         integer NOT NULL CHECK (version_no >= 1),
    kind               text NOT NULL CHECK (kind IN ('judgement', 'alternative_explanation', 'limitation')),
    basis              text NOT NULL CHECK (basis IN ('evidence', 'qualitative')),
    evidence_refs      text[] NOT NULL DEFAULT '{}',
    about_judgement_id text NOT NULL DEFAULT '',
    body               text NOT NULL,
    author_kind        text NOT NULL CHECK (author_kind IN ('human')),
    voided             boolean NOT NULL DEFAULT false,
    recorded_by        text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (basis = 'evidence' AND cardinality(evidence_refs) > 0)
        OR (basis = 'qualitative' AND cardinality(evidence_refs) = 0)
    ),
    CHECK (about_judgement_id = '' OR kind = 'alternative_explanation')
);
