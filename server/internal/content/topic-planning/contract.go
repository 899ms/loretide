// Package topicplanning owns manually curated topic cards and append-only
// frozen brief revisions.
package topicplanning

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid topic planning input")
	ErrNotFound = errors.New("topic card or brief revision not found")
	ErrStorage  = errors.New("topic planning storage unavailable")
)

// FieldError names the field that was wrong, so a 400 can say which one.
type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason,omitempty"`
}

func (e FieldError) Error() string { return e.Reason + ": " + e.Field }
func (e FieldError) Unwrap() error { return ErrInvalid }

type Status string

const (
	StatusDraft    Status = "draft"
	StatusStarted  Status = "started"
	StatusSaved    Status = "saved"
	StatusDeferred Status = "deferred"
	StatusDropped  Status = "dropped"
)

type Action string

const (
	ActionStart Action = "start"
	ActionSave  Action = "save"
	ActionDefer Action = "defer"
	ActionDrop  Action = "drop"
)

func (a Action) Status() (Status, bool) {
	switch a {
	case ActionStart:
		return StatusStarted, true
	case ActionSave:
		return StatusSaved, true
	case ActionDefer:
		return StatusDeferred, true
	case ActionDrop:
		return StatusDropped, true
	default:
		return "", false
	}
}

// AccountFilterNone selects the cards that are linked to no account.
//
// A list filter needs three answers, not two: every card, the cards of one
// account, and the cards nobody has attached to an account yet. The third one
// has no id to name it, so it gets a word an id can never be - ids are hex.
const AccountFilterNone = "none"

type TopicCard struct {
	TopicCardID               string    `json:"topic_card_id"`
	WorkspaceID               string    `json:"workspace_id"`
	AccountID                 *string   `json:"account_id"`
	AudienceProblemJudgment   string    `json:"audience_problem_judgment"`
	IPFit                     string    `json:"ip_fit"`
	Timing                    string    `json:"timing"`
	ExistingContentRelation   string    `json:"existing_content_relation"`
	EvidenceGapsAndInvestment string    `json:"evidence_gaps_and_investment"`
	Channels                  []string  `json:"channels"`
	// SOP 5.2 item 2: "为什么适合这个 IP，引用哪些素材和过去的经营结论".
	FitSourceIDs              []string  `json:"fit_source_ids"`
	// SOP 5.2 item 5: "证据是否充分，存在什么缺口，需要多少研究或制作投入".
	EvidenceSourceIDs         []string  `json:"evidence_source_ids"`
	RecommendedAction         string    `json:"recommended_action"`
	Status                    Status    `json:"status"`
	DecisionReason            string    `json:"decision_reason"`
	DecisionNote              string    `json:"decision_note"`
	StartedBriefRevisionID    *string   `json:"started_brief_revision_id"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// PatchString distinguishes a missing field from an explicitly empty string.
// Empty text is a legitimate "not known" answer on a topic card, while a
// missing field must leave the stored value untouched.
type PatchString struct {
	Set   bool
	Value string
}

func (p *PatchString) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return ErrInvalid
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return ErrInvalid
	}
	p.Set = true
	p.Value = value
	return nil
}

// TopicBodyPatch is the only mutable portion of an existing topic card.
// Account ownership, source references, decision state, channels and the
// recommended action all have their own semantics and must not be changed by
// this endpoint.
type TopicBodyPatch struct {
	AudienceProblemJudgment   PatchString `json:"audience_problem_judgment"`
	IPFit                     PatchString `json:"ip_fit"`
	Timing                    PatchString `json:"timing"`
	ExistingContentRelation   PatchString `json:"existing_content_relation"`
	EvidenceGapsAndInvestment PatchString `json:"evidence_gaps_and_investment"`
}

func (p *TopicBodyPatch) UnmarshalJSON(data []byte) error {
	type wire TopicBodyPatch
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalid
	}
	*p = TopicBodyPatch(decoded)
	return nil
}

func (p TopicBodyPatch) Validate() error {
	if !p.AudienceProblemJudgment.Set && !p.IPFit.Set && !p.Timing.Set &&
		!p.ExistingContentRelation.Set && !p.EvidenceGapsAndInvestment.Set {
		return ErrInvalid
	}
	return nil
}

type BriefRevision struct {
	BriefRevisionID      string    `json:"brief_revision_id"`
	TopicCardID          string    `json:"topic_card_id"`
	WorkspaceID          string    `json:"workspace_id"`
	Revision             int64     `json:"revision"`
	Audience             string    `json:"audience"`
	CoreProblem          string    `json:"core_problem"`
	ClaimAndBoundaries   string    `json:"claim_and_boundaries"`
	Channels             []string  `json:"channels"`
	Format               string    `json:"format"`
	Structure            string    `json:"structure"`
	CitationRequirements string    `json:"citation_requirements"`
	SourceScope          string    `json:"source_scope"`
	Deliverable          string    `json:"deliverable"`
	TimeLimit            string    `json:"time_limit"`
	CostLimit            string    `json:"cost_limit"`
	CreatedAt            time.Time `json:"created_at"`
}

type ActionRequest struct {
	Action Action        `json:"action"`
	Reason string        `json:"reason"`
	Note   string        `json:"note"`
	Brief  BriefRevision `json:"brief"`
}

type ActionResult struct {
	TopicCard TopicCard      `json:"topic_card"`
	Brief     *BriefRevision `json:"brief_revision,omitempty"`
}
