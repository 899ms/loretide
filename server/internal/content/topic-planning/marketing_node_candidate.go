package topicplanning

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
)

// Marketing node candidates, adoption and schedule impact (specs/033 PR 2).
// Contract: specs/033-marketing-nodes/contracts/marketing-nodes.md §4, §5.
//
// A candidate is one row per (brand, node, account); the account is '' for a
// brand-level candidate. Everything a person writes on it - the angle, the
// status, the adoption and the impact decision - is stored. Everything else a
// candidate shows (timing, collisions, material gaps, duplicate risks, the
// relation to the account) is computed on every read from columns people
// filled in, because "today" moves and a stored copy would go stale.
//
// Nothing here writes text of its own. The one string this file composes, the
// timing prefilled on an adopted card, is the node's own fields joined by a
// fixed template (§4.3); no executor is involved (constitution IX, FR-029).
//
// Lock order on every write: the node row (FOR SHARE), then the candidate row
// (FOR UPDATE). Revise, confirm and cancel take the node row FOR UPDATE, so an
// adoption and a reschedule serialize and adopted_revision is always a
// revision the adopter actually saw.

type CandidateStatus string

const (
	CandidateOpen      CandidateStatus = "open"
	CandidateAdopted   CandidateStatus = "adopted"
	CandidateDismissed CandidateStatus = "dismissed"
)

type ImpactDecision string

const (
	ImpactKept    ImpactDecision = "kept"
	ImpactHandled ImpactDecision = "handled"
)

// Limits on what a person writes on a candidate (contract §1.1 for the angle;
// the reason and the impact note share the note limit).
const (
	MaxCandidateAngleLength  = 2000
	MaxCandidateReasonLength = 2000
	MaxImpactNoteLength      = 2000
	minNameMatchRunes        = 2
)

// CandidateTiming is the node's timing plus the two numbers a person needs to
// judge whether there is still time: days until the start and the lead time.
// LeadShort is nil - not false - when no lead time was set (§5).
type CandidateTiming struct {
	NodeTiming
	DaysUntilStart int   `json:"days_until_start"`
	LeadDays       *int  `json:"lead_days"`
	LeadShort      *bool `json:"lead_short"`
}

// AccountRelation is the account's own configuration, read as stored. A value
// nobody filled in stays empty with status pending; nothing is written in its
// place (FR-023).
type AccountRelation struct {
	Audience       ipprofile.TextField `json:"audience"`
	ContentPillars ipprofile.TextField `json:"content_pillars"`
	ContentGoals   ipprofile.TextField `json:"content_goals"`
}

// CandidateRelation is why this candidate is related to its account: the
// node's goal, the role a person gave the account on the node, and the
// account's configuration. Account is nil for a brand-level candidate and for
// an account that was never configured.
type CandidateRelation struct {
	Goal    string           `json:"goal"`
	Role    string           `json:"role"`
	Account *AccountRelation `json:"account"`
}

type NodeCollision struct {
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
}

type MaterialGapKind string

const (
	GapNone     MaterialGapKind = "none"
	GapArchived MaterialGapKind = "archived"
	GapMissing  MaterialGapKind = "missing"
)

type MaterialGap struct {
	Kind     MaterialGapKind `json:"kind"`
	SourceID string          `json:"source_id,omitempty"`
}

type DuplicateReason string

const (
	DuplicateNameMatch       DuplicateReason = "name_match"
	DuplicateAdoptedFromNode DuplicateReason = "adopted_from_node"
)

type DuplicateRisk struct {
	TopicCardID string          `json:"topic_card_id"`
	Reason      DuplicateReason `json:"reason"`
}

// NodeCandidate is a stored candidate with its read-time computation.
type NodeCandidate struct {
	CandidateID            string          `json:"candidate_id"`
	WorkspaceID            string          `json:"workspace_id"`
	NodeID                 string          `json:"node_id"`
	AccountID              string          `json:"account_id"`
	Angle                  string          `json:"angle"`
	Status                 CandidateStatus `json:"status"`
	DismissReason          string          `json:"dismiss_reason"`
	TopicCardID            string          `json:"topic_card_id"`
	AdoptedRevision        *int64          `json:"adopted_revision"`
	ImpactDecision         ImpactDecision  `json:"impact_decision"`
	ImpactDecisionNote     string          `json:"impact_decision_note"`
	ImpactDecisionRevision *int64          `json:"impact_decision_revision"`
	ImpactDecidedBy        string          `json:"impact_decided_by"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`

	InScope        bool              `json:"in_scope"`
	Timing         CandidateTiming   `json:"timing"`
	Relation       CandidateRelation `json:"relation"`
	Collisions     []NodeCollision   `json:"collisions"`
	MaterialGaps   []MaterialGap     `json:"material_gaps"`
	DuplicateRisks []DuplicateRisk   `json:"duplicate_risks"`
	Origin         NodeOrigin        `json:"origin"`
	DateCertainty  DateCertainty     `json:"date_certainty"`
	DateBasis      string            `json:"date_basis"`
}

// ---------------------------------------------------------------------------
// Pure rules (§5). Tested without a database in marketing_node_candidate_test.go.

// NodeWindow is what the collision rule needs to know about one node.
type NodeWindow struct {
	NodeID     string
	Name       string
	Status     NodeStatus
	StartsOn   string
	EndsOn     string
	LeadDays   *int
	AccountIDs []string
}

func windowOf(node MarketingNode) NodeWindow {
	accounts := make([]string, 0, len(node.Current.Accounts))
	for _, account := range node.Current.Accounts {
		accounts = append(accounts, account.AccountID)
	}
	return NodeWindow{
		NodeID: node.NodeID, Name: node.Current.Name, Status: node.Status,
		StartsOn: node.Current.StartsOn, EndsOn: node.Current.EndsOn,
		LeadDays: node.Current.LeadDays, AccountIDs: accounts,
	}
}

// span is [preparation start, end], or [start, end] when no lead time was set.
func (w NodeWindow) span() (Date, Date, bool) {
	starts, okStart := ParseDate(w.StartsOn)
	ends, okEnd := ParseDate(w.EndsOn)
	if !okStart || !okEnd {
		return Date{}, Date{}, false
	}
	if prep, ok := PreparationStartsOn(starts, w.LeadDays); ok {
		starts = prep
	}
	return starts, ends, true
}

// FindCollisions lists the other active nodes whose [preparation start, end]
// overlaps self's, touching endpoints included, and that compete for the same
// account: either node applies to every account (no accounts listed), or both
// list this candidate's account (FR-025, Q1=A). Dates of nodes in different
// zones are compared as written - the known approximation of Q2=A.
func FindCollisions(self NodeWindow, accountID string, others []NodeWindow) []NodeCollision {
	collisions := []NodeCollision{}
	from, to, ok := self.span()
	if !ok {
		return collisions
	}
	for _, other := range others {
		if other.NodeID == self.NodeID || other.Status != NodeStatusActive {
			continue
		}
		otherFrom, otherTo, ok := other.span()
		if !ok || from.Compare(otherTo) > 0 || otherFrom.Compare(to) > 0 {
			continue
		}
		shared := len(self.AccountIDs) == 0 || len(other.AccountIDs) == 0 ||
			(accountID != "" && slices.Contains(self.AccountIDs, accountID) && slices.Contains(other.AccountIDs, accountID))
		if !shared {
			continue
		}
		collisions = append(collisions, NodeCollision{
			NodeID: other.NodeID, Name: other.Name, StartsOn: other.StartsOn, EndsOn: other.EndsOn,
		})
	}
	return collisions
}

// ComputeCandidateTiming adds days-until-start and lead shortness to a node's
// timing (FR-024). Short means: not started yet, and fewer days left than the
// lead time asks for. With no lead time there is no answer, so LeadShort is
// nil rather than false.
func ComputeCandidateTiming(now time.Time, content NodeContent) (CandidateTiming, error) {
	timing, err := ComputeTiming(now, content)
	if err != nil {
		return CandidateTiming{}, err
	}
	today, _ := ParseDate(timing.Today)
	starts, _ := ParseDate(content.StartsOn)
	days := today.DaysUntil(starts)
	result := CandidateTiming{NodeTiming: timing, DaysUntilStart: days, LeadDays: content.LeadDays}
	if content.LeadDays != nil {
		short := days > 0 && days < *content.LeadDays
		result.LeadShort = &short
	}
	return result, nil
}

// NameMatchPattern is the ILIKE pattern for "contains the node name": the name
// trimmed, with the LIKE metacharacters escaped so a node called "100%" does
// not match everything. A name shorter than two characters is not matched at
// all - "6" would hit every card (FR-027).
func NameMatchPattern(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < minNameMatchRunes {
		return "", false
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(name)
	return "%" + escaped + "%", true
}

// duplicateRiskQuery reads the cards of this brand that are not dropped and
// mention the node name in any of the five body answers. An account candidate
// looks at that account's cards; a brand-level candidate at every card.
func duplicateRiskQuery(workspaceID, accountID, pattern string) (string, []any) {
	query := `SELECT topic_card_id FROM content_topic_card
		WHERE workspace_id = $1 AND status <> 'dropped'
		  AND (audience_problem_judgment ILIKE $2 ESCAPE '\' OR ip_fit ILIKE $2 ESCAPE '\'
		    OR timing ILIKE $2 ESCAPE '\' OR existing_content_relation ILIKE $2 ESCAPE '\'
		    OR evidence_gaps_and_investment ILIKE $2 ESCAPE '\')`
	args := []any{workspaceID, pattern}
	if accountID != "" {
		query += ` AND account_id = $3`
		args = append(args, accountID)
	}
	return query + ` ORDER BY created_at, topic_card_id`, args
}

// AdoptionTiming is the "why now" prefilled on a card created by adoption: the
// node's own name, dates, zone and preparation start, joined by a fixed
// Chinese template (§4.3, plan D4). It is a composed fact, not generated text.
func AdoptionTiming(content NodeContent) string {
	preparation := "准备期未设置"
	if starts, ok := ParseDate(content.StartsOn); ok {
		if prep, ok := PreparationStartsOn(starts, content.LeadDays); ok {
			preparation = "准备期自 " + prep.String()
		}
	}
	return content.Name + "｜" + content.StartsOn + "–" + content.EndsOn +
		"（" + content.Timezone + "）｜" + preparation
}

// ---------------------------------------------------------------------------
// Request decoding.

// CandidatePatch changes what a person wrote on a candidate. Each key may be
// missing, which leaves the column as it is (§2.1).
type CandidatePatch struct {
	Angle         PatchString `json:"angle"`
	Status        PatchString `json:"status"`
	DismissReason PatchString `json:"dismiss_reason"`
}

func DecodeCandidatePatch(data []byte) (CandidatePatch, error) {
	var patch CandidatePatch
	if err := decodeStrict(data, &patch); err != nil {
		return CandidatePatch{}, err
	}
	if !patch.Angle.Set && !patch.Status.Set && !patch.DismissReason.Set {
		return CandidatePatch{}, ErrInvalid
	}
	if patch.Angle.Set && tooLong(patch.Angle.Value, MaxCandidateAngleLength) {
		return CandidatePatch{}, FieldError{Field: "angle", Reason: "too long"}
	}
	if patch.Status.Set {
		switch CandidateStatus(patch.Status.Value) {
		case CandidateOpen, CandidateDismissed:
		default:
			return CandidatePatch{}, FieldError{Field: "status", Reason: "open or dismissed"}
		}
	}
	if patch.DismissReason.Set && tooLong(patch.DismissReason.Value, MaxCandidateReasonLength) {
		return CandidatePatch{}, FieldError{Field: "dismiss_reason", Reason: "too long"}
	}
	return patch, nil
}

type AdoptMode string

const (
	AdoptCreate AdoptMode = "create"
	AdoptLink   AdoptMode = "link"
)

type AdoptRequest struct {
	Mode        AdoptMode `json:"mode"`
	TopicCardID string    `json:"topic_card_id"`
}

func DecodeAdopt(data []byte) (AdoptRequest, error) {
	var req AdoptRequest
	if err := decodeStrict(data, &req); err != nil {
		return AdoptRequest{}, err
	}
	req.TopicCardID = strings.TrimSpace(req.TopicCardID)
	switch req.Mode {
	case AdoptCreate:
		if req.TopicCardID != "" {
			return AdoptRequest{}, FieldError{Field: "topic_card_id", Reason: "not allowed when creating a card"}
		}
	case AdoptLink:
		if req.TopicCardID == "" {
			return AdoptRequest{}, FieldError{Field: "topic_card_id", Reason: "required when linking a card"}
		}
	default:
		return AdoptRequest{}, FieldError{Field: "mode", Reason: "create or link"}
	}
	return req, nil
}

// AdoptResult is the candidate after adoption and the card it points at.
type AdoptResult struct {
	Candidate NodeCandidate `json:"candidate"`
	TopicCard TopicCard     `json:"topic_card"`
}

type ImpactDecisionRequest struct {
	Decision ImpactDecision `json:"decision"`
	Note     string         `json:"note"`
}

func DecodeImpactDecision(data []byte) (ImpactDecisionRequest, error) {
	var req ImpactDecisionRequest
	if err := decodeStrict(data, &req); err != nil {
		return ImpactDecisionRequest{}, err
	}
	if req.Decision != ImpactKept && req.Decision != ImpactHandled {
		return ImpactDecisionRequest{}, FieldError{Field: "decision", Reason: "kept or handled"}
	}
	if tooLong(req.Note, MaxImpactNoteLength) {
		return ImpactDecisionRequest{}, FieldError{Field: "note", Reason: "too long"}
	}
	return req, nil
}

// ImpactDates are the four fields whose change makes a revision a reschedule.
type ImpactDates struct {
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
	Timezone string `json:"timezone"`
	LeadDays *int   `json:"lead_days"`
}

// ImpactItem is one adopted card whose node moved or was cancelled after the
// adoption, and after the last decision a person recorded on it (§4.4).
type ImpactItem struct {
	CandidateID     string      `json:"candidate_id"`
	TopicCardID     string      `json:"topic_card_id"`
	AccountID       string      `json:"account_id"`
	CardStatus      string      `json:"card_status"`
	AdoptedRevision int64       `json:"adopted_revision"`
	CurrentRevision int64       `json:"current_revision"`
	Before          ImpactDates `json:"before"`
	After           ImpactDates `json:"after"`
	Cancelled       bool        `json:"cancelled"`
}

// ---------------------------------------------------------------------------
// Store.

const candidateColumns = `c.candidate_id, c.workspace_id, c.node_id, c.account_id,
	c.angle, c.status, c.dismiss_reason, c.topic_card_id, c.adopted_revision,
	c.impact_decision, c.impact_decision_note, c.impact_decision_revision,
	c.impact_decided_by, c.created_at, c.updated_at`

const candidateSelect = `SELECT ` + candidateColumns + `
	FROM content_marketing_node_candidate c`

func scanCandidate(row scanner) (NodeCandidate, error) {
	var c NodeCandidate
	err := row.Scan(&c.CandidateID, &c.WorkspaceID, &c.NodeID, &c.AccountID,
		&c.Angle, &c.Status, &c.DismissReason, &c.TopicCardID, &c.AdoptedRevision,
		&c.ImpactDecision, &c.ImpactDecisionNote, &c.ImpactDecisionRevision,
		&c.ImpactDecidedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return NodeCandidate{}, ErrNotFound
	}
	if err != nil {
		return NodeCandidate{}, ErrStorage
	}
	return c, nil
}

// lockNodeShared reads the node inside a write transaction and holds it
// against a concurrent revise, confirm or cancel until the transaction ends.
// A node that is not this brand's is ErrNotFound (FR-042).
func (s *Store) lockNodeShared(ctx context.Context, tx pgx.Tx, workspaceID, nodeID string) (MarketingNode, error) {
	return s.scanNode(tx.QueryRow(ctx, nodeSelect+`
		WHERE n.workspace_id = $1 AND n.node_id = $2
		FOR SHARE OF n`, workspaceID, nodeID))
}

// lockCandidate reads one candidate of this node and this brand FOR UPDATE.
// A candidate of another node, or of another brand, is ErrNotFound.
func lockCandidate(ctx context.Context, tx pgx.Tx, workspaceID, nodeID, candidateID string) (NodeCandidate, error) {
	return scanCandidate(tx.QueryRow(ctx, candidateSelect+`
		WHERE c.workspace_id = $1 AND c.node_id = $2 AND c.candidate_id = $3
		FOR UPDATE`, workspaceID, nodeID, candidateID))
}

// SyncCandidates makes sure every account the node applies to has a
// candidate, or the brand has one when the node lists no account (FR-018). It
// only ever inserts, and ON CONFLICT on the candidate key index makes a
// repeated or concurrent sync a no-op for rows that exist: an angle, a status
// or an adoption a person recorded is never touched (FR-019, FR-020). Rows of
// accounts that left the node stay; the read marks them out of scope (FR-021).
func (s *Store) SyncCandidates(ctx context.Context, workspaceID, actor, nodeID string) ([]NodeCandidate, error) {
	const step = "sync-marketing-candidates"
	if s == nil {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, ErrInvalid)
		return nil, ErrInvalid
	}
	fail := func(err error) ([]NodeCandidate, error) {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
		return nil, err
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if ctx, err = s.audit(ctx, tx, workspaceID, actor, nodeID, step); err != nil {
		return fail(err)
	}
	node, err := s.lockNodeShared(ctx, tx, workspaceID, nodeID)
	if err != nil {
		return fail(err)
	}
	if node.Status != NodeStatusActive {
		return fail(FieldError{Field: "status", Reason: "only an active node has candidates"})
	}
	accounts := []string{""}
	if len(node.Current.Accounts) > 0 {
		accounts = accounts[:0]
		for _, account := range node.Current.Accounts {
			accounts = append(accounts, account.AccountID)
		}
	}
	for _, accountID := range accounts {
		if _, err = tx.Exec(ctx, `
			INSERT INTO content_marketing_node_candidate (candidate_id, workspace_id, node_id, account_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (workspace_id, node_id, account_id) DO NOTHING`,
			s.newID(), workspaceID, nodeID, accountID); err != nil {
			return fail(ErrStorage)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	return s.ListCandidates(ctx, workspaceID, actor, nodeID)
}

// ListCandidates returns the node's candidates with their read-time
// computation. A node that is not this brand's is ErrNotFound.
func (s *Store) ListCandidates(ctx context.Context, workspaceID, actor, nodeID string) ([]NodeCandidate, error) {
	const step = "list-marketing-candidates"
	candidates, err := s.readCandidates(ctx, workspaceID, actor, nodeID, "")
	if err != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
	}
	return candidates, err
}

func (s *Store) readCandidate(ctx context.Context, workspaceID, actor, nodeID, candidateID string) (NodeCandidate, error) {
	candidates, err := s.readCandidates(ctx, workspaceID, actor, nodeID, candidateID)
	if err != nil {
		return NodeCandidate{}, err
	}
	if len(candidates) != 1 {
		return NodeCandidate{}, ErrNotFound
	}
	return candidates[0], nil
}

// readCandidates loads the node's candidates and computes, for each, what §5
// asks for. only narrows the result to one candidate; the others are still
// read, because "adopted by another candidate of this node" needs them.
func (s *Store) readCandidates(ctx context.Context, workspaceID, actor, nodeID, only string) ([]NodeCandidate, error) {
	if s == nil || s.DB == nil {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" {
		return nil, ErrInvalid
	}
	node, err := s.scanNode(s.DB.QueryRow(ctx, nodeSelect+`
		WHERE n.workspace_id = $1 AND n.node_id = $2`, workspaceID, nodeID))
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, candidateSelect+`
		WHERE c.workspace_id = $1 AND c.node_id = $2
		ORDER BY c.created_at, c.candidate_id`, workspaceID, nodeID)
	if err != nil {
		return nil, ErrStorage
	}
	all := []NodeCandidate{}
	for rows.Next() {
		candidate, scanErr := scanCandidate(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		all = append(all, candidate)
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, ErrStorage
	}

	timing, err := ComputeCandidateTiming(s.now(), node.Current.NodeContent)
	if err != nil {
		return nil, err
	}
	others, err := s.activeNodeWindows(ctx, workspaceID, nodeID)
	if err != nil {
		return nil, err
	}
	gaps, err := s.materialGaps(ctx, workspaceID, node.Current.MaterialSourceIDs)
	if err != nil {
		return nil, err
	}
	self := windowOf(node)
	pattern, matchable := NameMatchPattern(node.Current.Name)

	result := []NodeCandidate{}
	for _, candidate := range all {
		if only != "" && candidate.CandidateID != only {
			continue
		}
		candidate.Timing = timing
		candidate.Origin = node.Origin
		candidate.DateCertainty = node.Current.DateCertainty
		candidate.DateBasis = node.Current.DateBasis
		candidate.MaterialGaps = gaps
		candidate.Collisions = FindCollisions(self, candidate.AccountID, others)
		candidate.InScope, candidate.Relation.Role = scopeOf(node.Current.Accounts, candidate.AccountID)
		candidate.Relation.Goal = node.Current.Goal
		if candidate.Relation.Account, err = s.accountRelation(ctx, workspaceID, candidate.AccountID); err != nil {
			return nil, err
		}
		if candidate.DuplicateRisks, err = s.duplicateRisks(ctx, workspaceID, candidate, all, pattern, matchable); err != nil {
			return nil, err
		}
		result = append(result, candidate)
	}
	return result, nil
}

// scopeOf says whether the candidate's account is still one the node applies
// to, and the role written for it there. The brand-level candidate is in
// scope while the node lists no account.
func scopeOf(accounts []NodeAccount, accountID string) (bool, string) {
	if accountID == "" {
		return len(accounts) == 0, ""
	}
	for _, account := range accounts {
		if account.AccountID == accountID {
			return true, account.Role
		}
	}
	return false, ""
}

func (s *Store) activeNodeWindows(ctx context.Context, workspaceID, nodeID string) ([]NodeWindow, error) {
	rows, err := s.DB.Query(ctx, nodeSelect+`
		WHERE n.workspace_id = $1 AND n.status = 'active' AND n.node_id <> $2
		ORDER BY r.starts_on, n.node_id`, workspaceID, nodeID)
	if err != nil {
		return nil, ErrStorage
	}
	defer rows.Close()
	windows := []NodeWindow{}
	for rows.Next() {
		other, scanErr := s.scanNode(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		windows = append(windows, windowOf(other))
	}
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	return windows, nil
}

// materialGaps reports each referenced material that is archived or no longer
// this brand's, and "none" when the node refers to no material (FR-026).
func (s *Store) materialGaps(ctx context.Context, workspaceID string, sourceIDs []string) ([]MaterialGap, error) {
	if len(sourceIDs) == 0 {
		return []MaterialGap{{Kind: GapNone}}, nil
	}
	if s.SourceStatuses == nil {
		return nil, ErrStorage
	}
	gaps := []MaterialGap{}
	for _, id := range sourceIDs {
		status, found, err := s.SourceStatuses.Status(ctx, workspaceID, id)
		if err != nil {
			return nil, ErrStorage
		}
		switch {
		case !found:
			gaps = append(gaps, MaterialGap{Kind: GapMissing, SourceID: id})
		case status == "archived":
			gaps = append(gaps, MaterialGap{Kind: GapArchived, SourceID: id})
		}
	}
	return gaps, nil
}

// accountRelation reads the account's current configuration, as stored. An
// empty status is the profile's own default and is reported as pending.
func (s *Store) accountRelation(ctx context.Context, workspaceID, accountID string) (*AccountRelation, error) {
	if accountID == "" {
		return nil, nil
	}
	if s.Accounts == nil {
		return nil, ErrStorage
	}
	revision, err := s.Accounts.CurrentPersonaRevision(ctx, workspaceID, accountID)
	if errors.Is(err, ipprofile.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrStorage
	}
	field := func(value ipprofile.TextField) ipprofile.TextField {
		if value.Status == "" {
			value.Status = ipprofile.FieldPending
		}
		return value
	}
	profile := revision.Profile
	return &AccountRelation{
		Audience:       field(profile.Audience),
		ContentPillars: field(profile.ContentPillars),
		ContentGoals:   field(profile.ContentGoals),
	}, nil
}

// duplicateRisks lists the cards other candidates of this node adopted, then
// the cards that mention the node name (FR-027). The candidate's own card is
// not a risk to itself, and a card is listed once.
func (s *Store) duplicateRisks(ctx context.Context, workspaceID string, candidate NodeCandidate, all []NodeCandidate, pattern string, matchable bool) ([]DuplicateRisk, error) {
	risks := []DuplicateRisk{}
	listed := map[string]struct{}{}
	if candidate.TopicCardID != "" {
		listed[candidate.TopicCardID] = struct{}{}
	}
	for _, other := range all {
		if other.CandidateID == candidate.CandidateID || other.TopicCardID == "" {
			continue
		}
		if _, seen := listed[other.TopicCardID]; seen {
			continue
		}
		listed[other.TopicCardID] = struct{}{}
		risks = append(risks, DuplicateRisk{TopicCardID: other.TopicCardID, Reason: DuplicateAdoptedFromNode})
	}
	if !matchable {
		return risks, nil
	}
	query, args := duplicateRiskQuery(workspaceID, candidate.AccountID, pattern)
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, ErrStorage
	}
	defer rows.Close()
	for rows.Next() {
		var cardID string
		if err = rows.Scan(&cardID); err != nil {
			return nil, ErrStorage
		}
		if _, seen := listed[cardID]; seen {
			continue
		}
		listed[cardID] = struct{}{}
		risks = append(risks, DuplicateRisk{TopicCardID: cardID, Reason: DuplicateNameMatch})
	}
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	return risks, nil
}

// EditCandidate changes the angle, the open/dismissed status or the dismiss
// reason. An adopted candidate keeps its status: its card exists, and
// marking it dismissed (or open) would make the record disagree with that
// (FR-036). Its angle can still be edited; the card is not affected.
func (s *Store) EditCandidate(ctx context.Context, workspaceID, actor, nodeID, candidateID string, patch CandidatePatch) (NodeCandidate, error) {
	const step = "edit-marketing-candidate"
	if s == nil {
		return NodeCandidate{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" || candidateID == "" {
		s.reportNodeFailure(ctx, workspaceID, actor, candidateID, step, ErrInvalid)
		return NodeCandidate{}, ErrInvalid
	}
	fail := func(err error) (NodeCandidate, error) {
		s.reportNodeFailure(ctx, workspaceID, actor, candidateID, step, err)
		return NodeCandidate{}, err
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if ctx, err = s.audit(ctx, tx, workspaceID, actor, candidateID, step); err != nil {
		return fail(err)
	}
	current, err := lockCandidate(ctx, tx, workspaceID, nodeID, candidateID)
	if err != nil {
		return fail(err)
	}
	if patch.Status.Set && current.Status == CandidateAdopted {
		return fail(FieldError{Field: "status", Reason: "an adopted candidate keeps its status"})
	}
	if _, err = tx.Exec(ctx, `
		UPDATE content_marketing_node_candidate
		SET angle = CASE WHEN $4 THEN $5 ELSE angle END,
			status = CASE WHEN $6 THEN $7 ELSE status END,
			dismiss_reason = CASE WHEN $8 THEN $9 ELSE dismiss_reason END,
			updated_at = now()
		WHERE workspace_id = $1 AND node_id = $2 AND candidate_id = $3`,
		workspaceID, nodeID, candidateID,
		patch.Angle.Set, patch.Angle.Value, patch.Status.Set, patch.Status.Value,
		patch.DismissReason.Set, patch.DismissReason.Value); err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	return s.readCandidate(ctx, workspaceID, actor, nodeID, candidateID)
}

// AdoptCandidate creates a draft topic card for the candidate, or links an
// existing card of this brand, and marks the candidate adopted with the
// node's current revision - all in one fenced transaction (FR-030 to FR-033).
//
// It starts nothing. The card is a draft; no brief, start snapshot, work,
// review, delivery or publication is written, and none of the card's four
// actions runs (FR-035). A candidate already adopted returns the card it
// already has: a double click, a retry or a concurrent request never makes a
// second card (FR-033), because the candidate row is locked before the check.
//
// The account and the materials are checked inside the fence (FR-032). The
// card insert is store.go's insertCardTx, the same statement Create uses.
func (s *Store) AdoptCandidate(ctx context.Context, workspaceID, actor, nodeID, candidateID string, req AdoptRequest) (AdoptResult, error) {
	const step = "adopt-marketing-candidate"
	if s == nil {
		return AdoptResult{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" || candidateID == "" {
		s.reportNodeFailure(ctx, workspaceID, actor, candidateID, step, ErrInvalid)
		return AdoptResult{}, ErrInvalid
	}
	fail := func(err error) (AdoptResult, error) {
		s.reportNodeFailure(ctx, workspaceID, actor, candidateID, step, err)
		return AdoptResult{}, err
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if ctx, err = s.audit(ctx, tx, workspaceID, actor, candidateID, step); err != nil {
		return fail(err)
	}
	node, err := s.lockNodeShared(ctx, tx, workspaceID, nodeID)
	if err != nil {
		return fail(err)
	}
	candidate, err := lockCandidate(ctx, tx, workspaceID, nodeID, candidateID)
	if err != nil {
		return fail(err)
	}

	var card TopicCard
	if candidate.Status == CandidateAdopted {
		card, err = scanTopicCard(tx.QueryRow(ctx, topicCardSelect+`
			WHERE workspace_id = $1 AND topic_card_id = $2`, workspaceID, candidate.TopicCardID))
		if err != nil {
			return fail(err)
		}
		if err = tx.Commit(ctx); err != nil {
			return fail(ErrStorage)
		}
		view, err := s.readCandidate(ctx, workspaceID, actor, nodeID, candidateID)
		if err != nil {
			return fail(err)
		}
		return AdoptResult{Candidate: view, TopicCard: card}, nil
	}
	if node.Status == NodeStatusCancelled {
		return fail(FieldError{Field: "status", Reason: "node is cancelled"})
	}

	switch req.Mode {
	case AdoptCreate:
		var accountID *string
		if candidate.AccountID != "" {
			id := candidate.AccountID
			accountID = &id
			if err = s.checkAccount(ctx, workspaceID, accountID); err != nil {
				return fail(err)
			}
		}
		card, err = prepareNewCard(TopicCard{
			TopicCardID: s.newID(), WorkspaceID: workspaceID, AccountID: accountID,
			IPFit: candidate.Angle, Timing: AdoptionTiming(node.Current.NodeContent),
			FitSourceIDs: node.Current.MaterialSourceIDs,
		})
		if err != nil {
			return fail(err)
		}
		if card, err = s.insertCardTx(ctx, tx, card); err != nil {
			return fail(err)
		}
	case AdoptLink:
		// Read only. Linking records the card on the candidate; the card
		// itself is not written (FR-034). FOR SHARE keeps its account from
		// changing between this check and the commit.
		card, err = scanTopicCard(tx.QueryRow(ctx, topicCardSelect+`
			WHERE workspace_id = $1 AND topic_card_id = $2 FOR SHARE`, workspaceID, req.TopicCardID))
		if err != nil {
			return fail(err)
		}
		if candidate.AccountID != "" && card.AccountID != nil && *card.AccountID != candidate.AccountID {
			return fail(FieldError{Field: "topic_card_id", Reason: "the card is written for another account"})
		}
	default:
		return fail(FieldError{Field: "mode", Reason: "create or link"})
	}

	if _, err = tx.Exec(ctx, `
		UPDATE content_marketing_node_candidate
		SET status = 'adopted', topic_card_id = $4, adopted_revision = $5, updated_at = now()
		WHERE workspace_id = $1 AND node_id = $2 AND candidate_id = $3`,
		workspaceID, nodeID, candidateID, card.TopicCardID, node.CurrentRevision); err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	view, err := s.readCandidate(ctx, workspaceID, actor, nodeID, candidateID)
	if err != nil {
		return fail(err)
	}
	return AdoptResult{Candidate: view, TopicCard: card}, nil
}

// ListImpact lists the adopted candidates whose node was rescheduled or
// cancelled after the adoption and after the last decision a person recorded
// on them (FR-038, FR-039). An edit that moves no date - the goal, the name -
// is not an impact, before or after a decision. It reads; it changes nothing.
func (s *Store) ListImpact(ctx context.Context, workspaceID, actor, nodeID string) ([]ImpactItem, error) {
	const step = "list-marketing-impact"
	fail := func(err error) ([]ImpactItem, error) {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
		return nil, err
	}
	if s == nil || s.DB == nil {
		return fail(ErrStorage)
	}
	if workspaceID == "" || actor == "" || nodeID == "" {
		return fail(ErrInvalid)
	}
	node, err := s.scanNode(s.DB.QueryRow(ctx, nodeSelect+`
		WHERE n.workspace_id = $1 AND n.node_id = $2`, workspaceID, nodeID))
	if err != nil {
		return fail(err)
	}
	rows, err := s.DB.Query(ctx, `
		SELECT c.candidate_id, c.topic_card_id, c.account_id, c.adopted_revision,
			COALESCE(tc.status, 'missing'),
			to_char(b.starts_on, 'YYYY-MM-DD'), to_char(b.ends_on, 'YYYY-MM-DD'),
			b.timezone, b.lead_days
		FROM content_marketing_node_candidate c
		JOIN content_marketing_node_revision b
		  ON b.workspace_id = c.workspace_id AND b.node_id = c.node_id AND b.revision = c.adopted_revision
		LEFT JOIN content_topic_card tc
		  ON tc.workspace_id = c.workspace_id AND tc.topic_card_id = c.topic_card_id
		WHERE c.workspace_id = $1 AND c.node_id = $2 AND c.status = 'adopted'
		  AND EXISTS (
			SELECT 1 FROM content_marketing_node_revision r
			WHERE r.workspace_id = c.workspace_id AND r.node_id = c.node_id
			  AND r.change_kind IN ('reschedule', 'cancel')
			  AND r.revision > GREATEST(c.adopted_revision, COALESCE(c.impact_decision_revision, 0)))
		ORDER BY c.created_at, c.candidate_id`, workspaceID, nodeID)
	if err != nil {
		return fail(ErrStorage)
	}
	defer rows.Close()
	after := ImpactDates{
		StartsOn: node.Current.StartsOn, EndsOn: node.Current.EndsOn,
		Timezone: node.Current.Timezone, LeadDays: node.Current.LeadDays,
	}
	items := []ImpactItem{}
	for rows.Next() {
		item := ImpactItem{
			CurrentRevision: node.CurrentRevision, After: after,
			Cancelled: node.Status == NodeStatusCancelled,
		}
		if err = rows.Scan(&item.CandidateID, &item.TopicCardID, &item.AccountID, &item.AdoptedRevision,
			&item.CardStatus, &item.Before.StartsOn, &item.Before.EndsOn,
			&item.Before.Timezone, &item.Before.LeadDays); err != nil {
			return fail(ErrStorage)
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		return fail(ErrStorage)
	}
	return items, nil
}

// DecideImpact records a person's decision on an impact item: kept as it is,
// or handled by hand. It writes the candidate row and nothing else - not the
// card, not any downstream table (FR-040). The decision carries the node
// revision it was made at, so a later reschedule brings the item back
// (FR-039).
func (s *Store) DecideImpact(ctx context.Context, workspaceID, actor, nodeID, candidateID string, req ImpactDecisionRequest) (NodeCandidate, error) {
	const step = "decide-marketing-impact"
	if s == nil {
		return NodeCandidate{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" || candidateID == "" {
		s.reportNodeFailure(ctx, workspaceID, actor, candidateID, step, ErrInvalid)
		return NodeCandidate{}, ErrInvalid
	}
	fail := func(err error) (NodeCandidate, error) {
		s.reportNodeFailure(ctx, workspaceID, actor, candidateID, step, err)
		return NodeCandidate{}, err
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if ctx, err = s.audit(ctx, tx, workspaceID, actor, candidateID, step); err != nil {
		return fail(err)
	}
	node, err := s.lockNodeShared(ctx, tx, workspaceID, nodeID)
	if err != nil {
		return fail(err)
	}
	candidate, err := lockCandidate(ctx, tx, workspaceID, nodeID, candidateID)
	if err != nil {
		return fail(err)
	}
	if candidate.Status != CandidateAdopted {
		return fail(FieldError{Field: "status", Reason: "only an adopted candidate has an impact to decide"})
	}
	if _, err = tx.Exec(ctx, `
		UPDATE content_marketing_node_candidate
		SET impact_decision = $4, impact_decision_note = $5, impact_decision_revision = $6,
			impact_decided_by = $7, updated_at = now()
		WHERE workspace_id = $1 AND node_id = $2 AND candidate_id = $3`,
		workspaceID, nodeID, candidateID, req.Decision, req.Note, node.CurrentRevision, actor); err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	return s.readCandidate(ctx, workspaceID, actor, nodeID, candidateID)
}
