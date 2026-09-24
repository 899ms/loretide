package feedbacklearning

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The operator's attribution judgement on a deal (specs/034 PR 2: FR-020,
// FR-021, FR-035; D14-V16).
//
// A judgement is not evidence. The touches under a lead record what the
// operator saw; this record says which of those touches the operator accepts
// as where one deal came from. Keeping the two apart is the point:
//
//   - writing a judgement writes one row here and nothing else - no touch
//     revision appears, so the evidence never looks as if it changed;
//   - a touch has no column naming the deal it was credited to;
//   - nothing in this module fills a judgement in for anybody. There is one
//     write path, it takes a person's request, and a guard test holds that it
//     is the only one.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §1.7

// Judgement is R-061's "可确认来源、用户判断、多触点及来源不明".
type Judgement string

const (
	JudgementConfirmed Judgement = "confirmed"
	JudgementOperator  Judgement = "operator_judgement"
	JudgementMulti     Judgement = "multi_touch"
	JudgementUnknown   Judgement = "unknown"
)

var Judgements = []Judgement{JudgementConfirmed, JudgementOperator, JudgementMulti, JudgementUnknown}

// AttributionRevision is one revision of the judgement on one deal. Weights
// is empty (no manual weights) or one positive integer per accepted touch.
type AttributionRevision struct {
	WorkspaceID string    `json:"workspace_id"`
	DealID      string    `json:"deal_id"`
	Revision    int       `json:"revision"`
	Voided      bool      `json:"voided"`
	Judgement   Judgement `json:"judgement"`
	TouchIDs    []string  `json:"touch_ids"`
	Weights     []int64   `json:"weights"`
	Note        string    `json:"note"`
	RecordedBy  string    `json:"recorded_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// AttributionInput is the body of a judgement revision.
type AttributionInput struct {
	Judgement string   `json:"judgement"`
	TouchIDs  []string `json:"touch_ids"`
	Weights   []int64  `json:"weights"`
	Note      string   `json:"note"`
}

// ValidateAttribution checks a judgement body in the contract's order:
// the controlled set, then the combinations, then lengths. Whether the
// touches exist, and belong to the deal's lead, is the store's earlier step.
func ValidateAttribution(in AttributionInput) (AttributionRevision, error) {
	if err := checkSet("judgement", in.Judgement, Judgements); err != nil {
		return AttributionRevision{}, err
	}
	touchIDs := slices.Clone(in.TouchIDs)
	if touchIDs == nil {
		touchIDs = []string{}
	}
	weights := slices.Clone(in.Weights)
	if weights == nil {
		weights = []int64{}
	}
	// "来源不明" accepts no touch at all; one with touches is not unknown.
	if Judgement(in.Judgement) == JudgementUnknown && len(touchIDs) > 0 {
		return AttributionRevision{}, FieldError{Field: "touch_ids", Reason: "an unknown source accepts no touch"}
	}
	if len(touchIDs) > MaxConfirmations {
		return AttributionRevision{}, FieldError{Field: "touch_ids", Reason: "too many"}
	}
	seen := map[string]bool{}
	for _, id := range touchIDs {
		if strings.TrimSpace(id) == "" || seen[id] {
			return AttributionRevision{}, FieldError{Field: "touch_ids", Reason: "empty or repeated"}
		}
		seen[id] = true
	}
	if len(weights) > 0 {
		if len(weights) != len(touchIDs) {
			return AttributionRevision{}, FieldError{Field: "weights", Reason: "one weight per touch, or none"}
		}
		for _, weight := range weights {
			if weight <= 0 || weight > MaxWeight {
				return AttributionRevision{}, FieldError{Field: "weights", Reason: "a weight is a positive integer"}
			}
		}
	}
	if err := checkRunes("note", in.Note, MaxNoteRunes); err != nil {
		return AttributionRevision{}, err
	}
	return AttributionRevision{
		Judgement: Judgement(in.Judgement), TouchIDs: touchIDs, Weights: weights, Note: in.Note,
	}, nil
}

const attributionColumns = `workspace_id, deal_id, revision, voided, judgement, touch_ids,
	weights, note, recorded_by, created_at`

func scanAttribution(row scanner) (AttributionRevision, error) {
	var attribution AttributionRevision
	var judgement string
	var weights []int32
	err := row.Scan(&attribution.WorkspaceID, &attribution.DealID, &attribution.Revision,
		&attribution.Voided, &judgement, &attribution.TouchIDs, &weights, &attribution.Note,
		&attribution.RecordedBy, &attribution.CreatedAt)
	attribution.Judgement = Judgement(judgement)
	attribution.Weights = make([]int64, 0, len(weights))
	for _, weight := range weights {
		attribution.Weights = append(attribution.Weights, int64(weight))
	}
	if attribution.TouchIDs == nil {
		attribution.TouchIDs = []string{}
	}
	attribution.CreatedAt = attribution.CreatedAt.UTC()
	return attribution, err
}

// RecordAttribution writes the next revision of the judgement on a deal; the
// first one is written with base_revision 0. It is the one path in this
// module that writes a judgement, and it writes nothing else: the touches it
// names are read, never revised (FR-020).
func (s *ROIStore) RecordAttribution(ctx context.Context, workspaceID, actor, dealID string,
	in AttributionInput, revision Revision) (AttributionRevision, error) {
	if dealID == "" {
		return AttributionRevision{}, ErrNotFound
	}
	var written AttributionRevision
	previousRevision := 0
	step := func() string { return writeStep("attribution", previousRevision == 0, revision.Voided) }
	err := s.inTxStep(ctx, workspaceID, actor, step, func(ctx context.Context, tx pgx.Tx) (string, error) {
		if err := lockDeal(ctx, tx, workspaceID, dealID); err != nil {
			return dealID, err
		}
		deal, err := latestDeal(ctx, tx, workspaceID, dealID)
		if err != nil {
			return dealID, err
		}
		previous, err := scanAttribution(tx.QueryRow(ctx, `SELECT `+attributionColumns+`
			FROM content_roi_attribution_revision WHERE workspace_id=$1 AND deal_id=$2
			ORDER BY revision DESC LIMIT 1`, workspaceID, dealID))
		switch {
		case err == nil:
			previousRevision = previous.Revision
		case !errors.Is(err, pgx.ErrNoRows):
			return dealID, ErrStorage
		}
		// Decision step 2: every accepted touch exists and hangs off this
		// deal's lead, or off a lead merged into it. A touch of some other
		// lead is answered exactly as one that does not exist.
		voided, err := s.checkAttributedTouches(ctx, tx, workspaceID, deal.LeadID, in.TouchIDs)
		if err != nil {
			return dealID, err
		}
		record, err := ValidateAttribution(in)
		if err != nil {
			return dealID, err
		}
		if voided {
			return dealID, FieldError{Field: "touch_ids", Reason: "a voided touch cannot be accepted"}
		}
		if err = checkBase(revision, previousRevision); err != nil {
			return dealID, err
		}
		record.Revision = previousRevision + 1
		record.WorkspaceID, record.DealID = workspaceID, dealID
		record.Voided, record.RecordedBy = revision.Voided, actor
		weights := make([]int32, 0, len(record.Weights))
		for _, weight := range record.Weights {
			weights = append(weights, int32(weight))
		}
		s.beforeInsert(ctx, "attribution", dealID)
		if err = tx.QueryRow(ctx, `INSERT INTO content_roi_attribution_revision
			(workspace_id, deal_id, revision, voided, judgement, touch_ids, weights, note, recorded_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			RETURNING created_at`,
			workspaceID, dealID, record.Revision, record.Voided, string(record.Judgement),
			record.TouchIDs, weights, record.Note, actor).Scan(&record.CreatedAt); err != nil {
			return dealID, insertError(err)
		}
		record.CreatedAt = record.CreatedAt.UTC()
		written = record
		return dealID, nil
	})
	if err != nil {
		return AttributionRevision{}, err
	}
	return written, nil
}

// checkAttributedTouches answers ErrNotFound when any touch id is missing or
// belongs to a lead other than leadID and the leads merged into it, and
// reports whether any of them is voided.
func (s *ROIStore) checkAttributedTouches(ctx context.Context, tx pgx.Tx, workspaceID, leadID string, touchIDs []string) (bool, error) {
	if len(touchIDs) == 0 {
		return false, nil
	}
	if leadID == "" {
		return false, ErrNotFound
	}
	members, err := mergedMembers(ctx, tx, workspaceID, leadID)
	if err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (touch_id) `+touchColumns+`
		FROM content_roi_touch_revision WHERE workspace_id=$1 AND touch_id = ANY($2::text[])
		ORDER BY touch_id, revision DESC`, workspaceID, touchIDs)
	if err != nil {
		return false, ErrStorage
	}
	touches, err := collect(rows, scanTouch)
	if err != nil {
		return false, err
	}
	found := map[string]TouchRevision{}
	for _, touch := range touches {
		found[touch.TouchID] = touch
	}
	voided := false
	for _, id := range touchIDs {
		touch, ok := found[id]
		if !ok || !slices.Contains(members, touch.LeadID) {
			return false, ErrNotFound
		}
		voided = voided || touch.Voided
	}
	return voided, nil
}
