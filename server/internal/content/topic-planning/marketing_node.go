package topicplanning

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	// Node dates are judged in the node's own IANA zone. cmd/server/main.go
	// embeds the zone database for the server binary, but that import lives in
	// package main: this package's own tests, run on a machine without a system
	// zone database (Windows), could not load America/Los_Angeles without it.
	_ "time/tzdata"
)

// Contract: specs/033-marketing-nodes/contracts/marketing-nodes.md

// ErrConflict is a revision that was not the node's current one when the
// write arrived: somebody else changed the node first. Nothing was written.
var ErrConflict = errors.New("marketing node revision conflict")

type NodeStatus string

const (
	NodeStatusUnconfirmed NodeStatus = "unconfirmed"
	NodeStatusActive      NodeStatus = "active"
	NodeStatusCancelled   NodeStatus = "cancelled"
)

func (s NodeStatus) valid() bool {
	return s == NodeStatusUnconfirmed || s == NodeStatusActive || s == NodeStatusCancelled
}

type NodeOrigin string

const (
	NodeOriginManual NodeOrigin = "manual"
	NodeOriginImport NodeOrigin = "import"
)

type NodeKind string

const (
	NodeKindHoliday       NodeKind = "holiday"
	NodeKindIndustry      NodeKind = "industry"
	NodeKindBrandCampaign NodeKind = "brand_campaign"
	NodeKindMarketing     NodeKind = "marketing"
)

type ChangeKind string

const (
	ChangeCreate     ChangeKind = "create"
	ChangeEdit       ChangeKind = "edit"
	ChangeReschedule ChangeKind = "reschedule"
	ChangeConfirm    ChangeKind = "confirm"
	ChangeCancel     ChangeKind = "cancel"
)

type DateCertainty string

const (
	DateConfirmed DateCertainty = "confirmed"
	DateTentative DateCertainty = "tentative"
)

// Limits from the contract (§1.1, §2.1). The note and role limits are not in
// the contract; they only keep a single field from being unbounded.
const (
	MaxNodeNameLength      = 200
	MaxNodeGoalLength      = 2000
	MaxNodeDateBasisLength = 1000
	MaxNodeNoteLength      = 2000
	MaxNodeRoleLength      = 200
	MaxNodeAccounts        = 20
	MaxNodeSpanDays        = 366
	MaxNodeLeadDays        = 365
	MaxNodeImportRows      = 100
	maxNodeRequestBytes    = 1 << 20
)

// NodeAccount is one account a node applies to, with the role a person wrote
// for it on this node.
type NodeAccount struct {
	AccountID string `json:"account_id"`
	Role      string `json:"role"`
}

// NodeContent is everything a person authors on a node. Every revision stores
// a complete copy of it.
//
// LeadDays is nil when no lead time was set, and a pointer to 0 when no
// preparation is needed. The two are different answers (FR-013).
type NodeContent struct {
	Name              string        `json:"name"`
	Kind              NodeKind      `json:"kind"`
	StartsOn          string        `json:"starts_on"`
	EndsOn            string        `json:"ends_on"`
	Timezone          string        `json:"timezone"`
	LeadDays          *int          `json:"lead_days"`
	Accounts          []NodeAccount `json:"accounts"`
	Goal              string        `json:"goal"`
	MaterialSourceIDs []string      `json:"material_source_ids"`
	DateCertainty     DateCertainty `json:"date_certainty"`
	DateBasis         string        `json:"date_basis"`
}

// NodeRevision is one append-only revision row.
type NodeRevision struct {
	RevisionID  string     `json:"revision_id"`
	NodeID      string     `json:"node_id"`
	WorkspaceID string     `json:"workspace_id"`
	Revision    int64      `json:"revision"`
	ChangeKind  ChangeKind `json:"change_kind"`
	StatusAfter NodeStatus `json:"status_after"`
	NodeContent
	Note      string    `json:"note"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}

// NodeTiming is the read-time computation of where a node stands (§3.2). It
// carries the date and zone it was computed from, so a page can say which
// "today" the phase was judged by (FR-017).
type NodeTiming struct {
	Today               string    `json:"today"`
	Timezone            string    `json:"timezone"`
	Phase               NodePhase `json:"phase"`
	PreparationStartsOn *string   `json:"preparation_starts_on"`
}

// MarketingNode is the node pointer with its current revision and timing.
type MarketingNode struct {
	NodeID          string       `json:"node_id"`
	WorkspaceID     string       `json:"workspace_id"`
	Status          NodeStatus   `json:"status"`
	Origin          NodeOrigin   `json:"origin"`
	CurrentRevision int64        `json:"current_revision"`
	Current         NodeRevision `json:"current"`
	Timing          NodeTiming   `json:"timing"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// ImportOutcome is what happened to one import row.
type ImportOutcome string

const (
	ImportCreated   ImportOutcome = "created"
	ImportDuplicate ImportOutcome = "duplicate"
	ImportInvalid   ImportOutcome = "invalid"
)

// ImportRowResult reports one row. Row counts from 1 in the order the rows
// were sent.
type ImportRowResult struct {
	Row     int           `json:"row"`
	Outcome ImportOutcome `json:"outcome"`
	NodeID  string        `json:"node_id,omitempty"`
	Field   string        `json:"field,omitempty"`
}

type ImportResult struct {
	Results []ImportRowResult `json:"results"`
}

// wireContent is the request shape of NodeContent. lead_days arrives raw so a
// missing key, null and a number can be told apart and 1.5 can be refused
// instead of truncated.
type wireContent struct {
	Name              string          `json:"name"`
	Kind              string          `json:"kind"`
	StartsOn          string          `json:"starts_on"`
	EndsOn            string          `json:"ends_on"`
	Timezone          string          `json:"timezone"`
	LeadDays          json.RawMessage `json:"lead_days"`
	Accounts          []NodeAccount   `json:"accounts"`
	Goal              string          `json:"goal"`
	MaterialSourceIDs []string        `json:"material_source_ids"`
	DateCertainty     string          `json:"date_certainty"`
	DateBasis         string          `json:"date_basis"`
}

// CreateNodeRequest is a manual create.
type CreateNodeRequest struct {
	Content NodeContent
	Note    string
}

// ReviseNodeRequest is an edit or reschedule. It is a whole-revision submit:
// every revision stores the full content, so an omitted lead_days here means
// "not set in this revision", unlike the topic body PATCH where an omitted
// field means "unchanged" (contract §2.1).
type ReviseNodeRequest struct {
	BaseRevision int64
	Content      NodeContent
	Note         string
}

// TransitionRequest is a confirm or a cancel.
type TransitionRequest struct {
	BaseRevision int64  `json:"base_revision"`
	Note         string `json:"note"`
}

// decodeStrict decodes exactly one JSON value with no unknown keys, the same
// rule the topic body PATCH applies (contract.go TopicBodyPatch).
func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

// decodeLeadDays tells the three states apart: missing or null is "not set";
// an integer literal is a value; anything else (1.5, "3", 1e2) is invalid.
func decodeLeadDays(raw json.RawMessage) (*int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	value, err := strconv.Atoi(string(trimmed))
	if err != nil {
		return nil, FieldError{Field: "lead_days", Reason: "must be a whole number of days"}
	}
	return &value, nil
}

func (w wireContent) content() (NodeContent, error) {
	lead, err := decodeLeadDays(w.LeadDays)
	if err != nil {
		return NodeContent{}, err
	}
	return NodeContent{
		Name: w.Name, Kind: NodeKind(w.Kind), StartsOn: w.StartsOn, EndsOn: w.EndsOn,
		Timezone: w.Timezone, LeadDays: lead, Accounts: w.Accounts, Goal: w.Goal,
		MaterialSourceIDs: w.MaterialSourceIDs, DateCertainty: DateCertainty(w.DateCertainty),
		DateBasis: w.DateBasis,
	}, nil
}

// DecodeCreateNode reads a manual create body.
func DecodeCreateNode(data []byte) (CreateNodeRequest, error) {
	var body struct {
		wireContent
		Note string `json:"note"`
	}
	if err := decodeStrict(data, &body); err != nil {
		return CreateNodeRequest{}, err
	}
	content, err := body.content()
	if err != nil {
		return CreateNodeRequest{}, err
	}
	return CreateNodeRequest{Content: content, Note: body.Note}, nil
}

// DecodeReviseNode reads an edit/reschedule body. base_revision is required.
func DecodeReviseNode(data []byte) (ReviseNodeRequest, error) {
	var body struct {
		wireContent
		BaseRevision *int64 `json:"base_revision"`
		Note         string `json:"note"`
	}
	if err := decodeStrict(data, &body); err != nil {
		return ReviseNodeRequest{}, err
	}
	if body.BaseRevision == nil || *body.BaseRevision < 1 {
		return ReviseNodeRequest{}, FieldError{Field: "base_revision", Reason: "required"}
	}
	content, err := body.content()
	if err != nil {
		return ReviseNodeRequest{}, err
	}
	return ReviseNodeRequest{BaseRevision: *body.BaseRevision, Content: content, Note: body.Note}, nil
}

// DecodeTransition reads a confirm or cancel body. base_revision is required.
func DecodeTransition(data []byte) (TransitionRequest, error) {
	var body struct {
		BaseRevision *int64 `json:"base_revision"`
		Note         string `json:"note"`
	}
	if err := decodeStrict(data, &body); err != nil {
		return TransitionRequest{}, err
	}
	if body.BaseRevision == nil || *body.BaseRevision < 1 {
		return TransitionRequest{}, FieldError{Field: "base_revision", Reason: "required"}
	}
	if utf8.RuneCountInString(body.Note) > MaxNodeNoteLength {
		return TransitionRequest{}, FieldError{Field: "note", Reason: "too long"}
	}
	return TransitionRequest{BaseRevision: *body.BaseRevision, Note: body.Note}, nil
}

// ImportRow is one decoded import row: either content, or the field that made
// it invalid. A bad row never fails the whole import (FR-008).
type ImportRow struct {
	Content NodeContent
	Err     error
}

// DecodeImport reads {"rows": [...]}. The envelope must be well formed and
// hold 1-100 rows; each row is then decoded on its own, so one malformed row
// is reported as invalid without taking the rest down.
func DecodeImport(data []byte) ([]ImportRow, error) {
	var body struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := decodeStrict(data, &body); err != nil {
		return nil, err
	}
	if len(body.Rows) == 0 || len(body.Rows) > MaxNodeImportRows {
		return nil, FieldError{Field: "rows", Reason: "between 1 and 100 rows"}
	}
	rows := make([]ImportRow, len(body.Rows))
	for i, raw := range body.Rows {
		var wire wireContent
		if err := decodeStrict(raw, &wire); err != nil {
			rows[i].Err = FieldError{Field: "row", Reason: "malformed row"}
			continue
		}
		content, err := wire.content()
		if err != nil {
			rows[i].Err = err
			continue
		}
		normalized, err := NormalizeNodeContent(content)
		rows[i] = ImportRow{Content: normalized, Err: err}
	}
	return rows, nil
}

func tooLong(value string, limit int) bool { return utf8.RuneCountInString(value) > limit }

// NormalizeNodeContent checks the shape of a node's content and returns it in
// stored form. It is pure: whether the accounts and materials belong to the
// brand is checked later, inside the write transaction (FR-044).
func NormalizeNodeContent(c NodeContent) (NodeContent, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || tooLong(c.Name, MaxNodeNameLength) {
		return NodeContent{}, FieldError{Field: "name", Reason: "1-200 characters"}
	}
	switch c.Kind {
	case NodeKindHoliday, NodeKindIndustry, NodeKindBrandCampaign, NodeKindMarketing:
	default:
		return NodeContent{}, FieldError{Field: "kind", Reason: "unknown kind"}
	}
	starts, ok := ParseDate(c.StartsOn)
	if !ok {
		return NodeContent{}, FieldError{Field: "starts_on", Reason: "YYYY-MM-DD"}
	}
	ends, ok := ParseDate(c.EndsOn)
	if !ok {
		return NodeContent{}, FieldError{Field: "ends_on", Reason: "YYYY-MM-DD"}
	}
	if ends.Compare(starts) < 0 {
		return NodeContent{}, FieldError{Field: "ends_on", Reason: "before starts_on"}
	}
	// Both days are included, so a one-day node spans 1.
	if starts.DaysUntil(ends)+1 > MaxNodeSpanDays {
		return NodeContent{}, FieldError{Field: "ends_on", Reason: "longer than 366 days"}
	}
	if _, err := LoadNodeLocation(c.Timezone); err != nil {
		return NodeContent{}, err
	}
	if c.LeadDays != nil && (*c.LeadDays < 0 || *c.LeadDays > MaxNodeLeadDays) {
		return NodeContent{}, FieldError{Field: "lead_days", Reason: "0-365"}
	}

	accounts := make([]NodeAccount, 0, len(c.Accounts))
	seen := make(map[string]struct{}, len(c.Accounts))
	for _, account := range c.Accounts {
		id := strings.TrimSpace(account.AccountID)
		if id == "" || tooLong(account.Role, MaxNodeRoleLength) {
			return NodeContent{}, FieldError{Field: "accounts", Reason: "account_id required, role at most 200 characters"}
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		accounts = append(accounts, NodeAccount{AccountID: id, Role: account.Role})
	}
	if len(accounts) > MaxNodeAccounts {
		return NodeContent{}, FieldError{Field: "accounts", Reason: "at most 20 accounts"}
	}
	c.Accounts = accounts

	if tooLong(c.Goal, MaxNodeGoalLength) {
		return NodeContent{}, FieldError{Field: "goal", Reason: "too long"}
	}
	sources, err := NormalizeSourceIDs("material_source_ids", c.MaterialSourceIDs)
	if err != nil {
		return NodeContent{}, err
	}
	c.MaterialSourceIDs = sources
	switch c.DateCertainty {
	case DateConfirmed, DateTentative:
	default:
		return NodeContent{}, FieldError{Field: "date_certainty", Reason: "confirmed or tentative"}
	}
	if tooLong(c.DateBasis, MaxNodeDateBasisLength) {
		return NodeContent{}, FieldError{Field: "date_basis", Reason: "too long"}
	}
	return c, nil
}

// LoadNodeLocation accepts a loadable IANA zone and refuses "" and "Local"
// explicitly: time.LoadLocation reads "" as UTC and "Local" as the server's
// zone, neither of which is a property of the node (handler/workspace.go
// validateTimezoneSetting, same rule).
func LoadNodeLocation(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return nil, FieldError{Field: "timezone", Reason: "IANA time zone required"}
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, FieldError{Field: "timezone", Reason: "unknown time zone"}
	}
	return loc, nil
}

func sameLead(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ClassifyChange decides the change kind of an edit from the content itself
// (FR-006): any change to the start, end, zone or lead time - including
// "not set" <-> a value - is a reschedule; any other change is an edit. A
// submit identical to the current revision is refused rather than recorded as
// an empty revision.
func ClassifyChange(previous, next NodeContent) (ChangeKind, error) {
	schedule := previous.StartsOn != next.StartsOn || previous.EndsOn != next.EndsOn ||
		previous.Timezone != next.Timezone || !sameLead(previous.LeadDays, next.LeadDays)
	if schedule {
		return ChangeReschedule, nil
	}
	same := previous.Name == next.Name && previous.Kind == next.Kind &&
		slices.Equal(previous.Accounts, next.Accounts) && previous.Goal == next.Goal &&
		slices.Equal(previous.MaterialSourceIDs, next.MaterialSourceIDs) &&
		previous.DateCertainty == next.DateCertainty && previous.DateBasis == next.DateBasis
	if same {
		return "", FieldError{Field: "revision", Reason: "nothing changed"}
	}
	return ChangeEdit, nil
}

// ComputeTiming is the read-time timing of a node's content at instant now.
func ComputeTiming(now time.Time, content NodeContent) (NodeTiming, error) {
	loc, err := LoadNodeLocation(content.Timezone)
	if err != nil {
		return NodeTiming{}, ErrStorage
	}
	starts, okStart := ParseDate(content.StartsOn)
	ends, okEnd := ParseDate(content.EndsOn)
	if !okStart || !okEnd {
		return NodeTiming{}, ErrStorage
	}
	today := Today(now, loc)
	timing := NodeTiming{
		Today: today.String(), Timezone: content.Timezone,
		Phase: Phase(today, starts, ends, content.LeadDays),
	}
	if prep, ok := PreparationStartsOn(starts, content.LeadDays); ok {
		value := prep.String()
		timing.PreparationStartsOn = &value
	}
	return timing, nil
}

// ParseNodeStatusFilter reads the optional ?status= list filter.
func ParseNodeStatusFilter(value string) (NodeStatus, error) {
	if value == "" {
		return "", nil
	}
	status := NodeStatus(value)
	if !status.valid() {
		return "", FieldError{Field: "status", Reason: "unknown status"}
	}
	return status, nil
}
