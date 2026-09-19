// Package topicplanning owns manually curated topic cards and append-only
// frozen brief revisions.
package topicplanning

import (
	"errors"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid topic planning input")
	ErrNotFound = errors.New("topic card or brief revision not found")
	ErrStorage  = errors.New("topic planning storage unavailable")
)

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
	RecommendedAction         string    `json:"recommended_action"`
	Status                    Status    `json:"status"`
	DecisionReason            string    `json:"decision_reason"`
	DecisionNote              string    `json:"decision_note"`
	StartedBriefRevisionID    *string   `json:"started_brief_revision_id"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
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
