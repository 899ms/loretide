package feedbacklearning

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Work marks for the operating diagnosis (specs/035 PR 1, ruling Q2=A;
// contract §1.2; FR-020, FR-021).
//
// Whether a work belongs to a content pillar, and whether it agrees with an
// item of an account's expression profile, are judgements a person makes.
// With the model runner disabled nothing here makes them; the diagnosis only
// counts what people marked. A mark is never changed: a later mark of the
// same (work, kind, item) is a new row, and the latest by (created_at,
// mark_id) is the current one. Untagging a pillar is a mark too.
//
// A consistency mark records the profile revision it was checked against,
// read by the server at that moment; a request cannot choose it.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §1.2

// WorkMarkInput is the body of POST /work-marks. profile_revision_id,
// recorded_by, mark_id and created_at are the server's.
type WorkMarkInput struct {
	WorkID    string      `json:"work_id"`
	Kind      MarkKind    `json:"kind"`
	Item      string      `json:"item"`
	Verdict   MarkVerdict `json:"verdict"`
	AccountID string      `json:"account_id"`
	Note      string      `json:"note"`
}

// WorkMark is one stored mark.
type WorkMark struct {
	MarkID            string      `json:"mark_id"`
	WorkspaceID       string      `json:"workspace_id"`
	WorkID            string      `json:"work_id"`
	Kind              MarkKind    `json:"kind"`
	Item              string      `json:"item"`
	Verdict           MarkVerdict `json:"verdict"`
	AccountID         string      `json:"account_id"`
	ProfileRevisionID string      `json:"profile_revision_id"`
	Note              string      `json:"note"`
	RecordedBy        string      `json:"recorded_by"`
	CreatedAt         time.Time   `json:"created_at"`
}

// WorkMarkFilter narrows GET /work-marks; both empty lists every current
// mark of the workspace.
type WorkMarkFilter struct {
	WorkID    string
	AccountID string
}

const workMarkColumns = `mark_id, workspace_id, work_id, kind, item, verdict, account_id,
	profile_revision_id, note, recorded_by, created_at`

func scanWorkMark(row scanner) (WorkMark, error) {
	var mark WorkMark
	var kind, verdict string
	err := row.Scan(&mark.MarkID, &mark.WorkspaceID, &mark.WorkID, &kind, &mark.Item, &verdict,
		&mark.AccountID, &mark.ProfileRevisionID, &mark.Note, &mark.RecordedBy, &mark.CreatedAt)
	mark.Kind, mark.Verdict = MarkKind(kind), MarkVerdict(verdict)
	mark.CreatedAt = mark.CreatedAt.UTC()
	return mark, err
}

// latestWorkMarks keeps the current mark of each (work, kind, item): the
// latest by created_at, then by mark_id. The answer is sorted by work, kind
// and item.
func latestWorkMarks(marks []WorkMark) []WorkMark {
	type key struct{ work, kind, item string }
	latest := map[key]WorkMark{}
	for _, mark := range marks {
		k := key{mark.WorkID, string(mark.Kind), mark.Item}
		current, ok := latest[k]
		if !ok || cmp.Or(mark.CreatedAt.Compare(current.CreatedAt), cmp.Compare(mark.MarkID, current.MarkID)) > 0 {
			latest[k] = mark
		}
	}
	out := make([]WorkMark, 0, len(latest))
	for _, mark := range latest {
		out = append(out, mark)
	}
	slices.SortFunc(out, func(left, right WorkMark) int {
		return cmp.Or(cmp.Compare(left.WorkID, right.WorkID), cmp.Compare(left.Kind, right.Kind),
			cmp.Compare(left.Item, right.Item))
	})
	return out
}

// validateWorkMark is decision steps 4 to 6 on a mark: the controlled sets,
// then required fields and the kind-verdict pairing, then lengths. It
// answers the item as stored (a pillar name normalized).
func validateWorkMark(in WorkMarkInput) (string, error) {
	if !oneOf(string(in.Kind), MarkKinds) {
		return "", FieldError{Field: "kind", Reason: "not one of pillar, consistency"}
	}
	if !oneOf(string(in.Verdict), MarkVerdicts) {
		return "", FieldError{Field: "verdict", Reason: "not a verdict"}
	}
	if strings.TrimSpace(in.WorkID) == "" {
		return "", FieldError{Field: "work_id", Reason: "required"}
	}
	if !slices.Contains(verdictsFor(in.Kind), in.Verdict) {
		return "", FieldError{Field: "verdict", Reason: "not a verdict for this kind"}
	}
	item := in.Item
	switch in.Kind {
	case MarkConsistency:
		if strings.TrimSpace(in.AccountID) == "" {
			return "", FieldError{Field: "account_id", Reason: "required for a consistency mark"}
		}
		if !slices.Contains(profileFieldKeys, item) {
			return "", FieldError{Field: "item", Reason: "not an expression profile item"}
		}
	case MarkPillar:
		if item = normalizePillar(item); item == "" {
			return "", FieldError{Field: "item", Reason: "required"}
		}
		if err := checkRuneLimit("item", item, MaxPillarRunes); err != nil {
			return "", err
		}
	}
	if err := checkRuneLimit("note", in.Note, MaxMarkNoteRunes); err != nil {
		return "", err
	}
	return item, nil
}

// RecordWorkMark appends one mark. The work and the account named exist
// here before anything else is checked; a consistency mark takes the
// account's current profile revision.
func (s *DiagnosisStore) RecordWorkMark(ctx context.Context, workspaceID, actor string, in WorkMarkInput) (WorkMark, error) {
	const step = "record-work-mark"
	if !s.ready() {
		return WorkMark{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return WorkMark{}, ErrInvalid
	}
	if s.Works == nil || s.Accounts == nil {
		return WorkMark{}, ErrStorage
	}
	if in.WorkID != "" {
		if err := adapterError(s.Works.WorkExists(ctx, workspaceID, actor, in.WorkID)); err != nil {
			return WorkMark{}, err
		}
	}
	if in.AccountID != "" {
		if err := adapterError(s.Accounts.AccountExists(ctx, workspaceID, in.AccountID)); err != nil {
			return WorkMark{}, err
		}
	}
	item, err := validateWorkMark(in)
	if err != nil {
		return WorkMark{}, err
	}
	mark := WorkMark{
		MarkID: s.newID(), WorkspaceID: workspaceID, WorkID: in.WorkID, Kind: in.Kind, Item: item,
		Verdict: in.Verdict, AccountID: in.AccountID, Note: in.Note, RecordedBy: actor,
	}
	if in.Kind == MarkConsistency {
		profile, readErr := s.Accounts.CurrentProfile(ctx, workspaceID, in.AccountID)
		if readErr != nil {
			return WorkMark{}, adapterError(readErr)
		}
		if profile.RevisionID == "" {
			return WorkMark{}, FieldError{Field: "account_id", Reason: "the account has no expression profile to check against"}
		}
		mark.ProfileRevisionID = profile.RevisionID
	}
	err = s.roi().inTx(ctx, workspaceID, actor, step, func(ctx context.Context, tx pgx.Tx) (string, error) {
		if err := tx.QueryRow(ctx, `INSERT INTO content_opdiag_work_mark
			(workspace_id, mark_id, work_id, kind, item, verdict, account_id, profile_revision_id, note, recorded_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING created_at`,
			workspaceID, mark.MarkID, mark.WorkID, string(mark.Kind), mark.Item, string(mark.Verdict),
			mark.AccountID, mark.ProfileRevisionID, mark.Note, actor).Scan(&mark.CreatedAt); err != nil {
			return mark.MarkID, ErrStorage
		}
		mark.CreatedAt = mark.CreatedAt.UTC()
		return mark.MarkID, nil
	})
	if err != nil {
		return WorkMark{}, err
	}
	return mark, nil
}

// ListWorkMarks answers the current mark of each (work, kind, item),
// narrowed to one work or to one account's consistency marks.
func (s *DiagnosisStore) ListWorkMarks(ctx context.Context, workspaceID, actor string, filter WorkMarkFilter) ([]WorkMark, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+workMarkColumns+` FROM content_opdiag_work_mark
		WHERE workspace_id=$1 AND ($2='' OR work_id=$2)
		ORDER BY created_at, mark_id`, workspaceID, filter.WorkID)
	if err != nil {
		return nil, ErrStorage
	}
	marks, err := collect(rows, scanWorkMark)
	if err != nil {
		return nil, err
	}
	current := latestWorkMarks(marks)
	if filter.AccountID == "" {
		return current, nil
	}
	narrowed := []WorkMark{}
	for _, mark := range current {
		if mark.AccountID == filter.AccountID {
			narrowed = append(narrowed, mark)
		}
	}
	return narrowed, nil
}
