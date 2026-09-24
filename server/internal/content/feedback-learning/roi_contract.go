package feedbacklearning

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Costs, leads, touches, deals and refunds/adjustments (specs/034 PR 1,
// BO-06 / R-061).
//
// What these records are, and what this file makes impossible:
//
//   - Every record is a revision. A correction or a void is the next revision
//     of the same id; nothing is rewritten, so a report that cited
//     (id, revision) keeps meaning what it meant.
//   - Money is integer minor units plus an ISO code, and a decimal string at
//     the API. No float (roi_money.go).
//   - A touch is evidence and only evidence. The operator's attribution
//     judgement is another table (PR 2), and nothing here fills in a work, an
//     evidence type or a judgement for anybody.
//   - A lead has a pseudonymous label and no column for who the customer
//     really is.
//
// Contract: specs/034-roi-review/contracts/roi-review.md

var (
	// ErrConflict is the family of 409s: a stale base revision, a possible
	// duplicate the caller has not confirmed.
	ErrConflict = errors.New("roi record conflict")
)

// RevisionConflict is 409 naming base_revision: the revision the caller edited
// is no longer the latest.
type RevisionConflict struct{ Field string }

func (e RevisionConflict) Error() string { return "stale revision: " + e.Field }
func (e RevisionConflict) Unwrap() error { return ErrConflict }

// PossibleDuplicate is 409 possible_duplicate. Matches are the ids of the
// current records whose dedupe key equals this one's; the caller confirms
// "not a duplicate" by sending them back in not_duplicate_of.
type PossibleDuplicate struct{ Matches []string }

func (e PossibleDuplicate) Error() string { return "possible duplicate" }
func (e PossibleDuplicate) Unwrap() error { return ErrConflict }

// Pricing is how a cost states its amount. R-061: "人工时间须有用户设定单价才能
// 折算金额".
type Pricing string

const (
	PricingAmount    Pricing = "amount"
	PricingLaborTime Pricing = "labor_time"
)

var Pricings = []Pricing{PricingAmount, PricingLaborTime}

// EvidenceType is exactly R-061's six, and there is no "other" (FR-018).
type EvidenceType string

const (
	EvidencePlatformLinkedContent EvidenceType = "platform_linked_content"
	EvidenceContentComment        EvidenceType = "content_comment"
	EvidenceCustomerStatement     EvidenceType = "customer_statement"
	EvidenceDedicatedChannel      EvidenceType = "dedicated_channel"
	EvidenceAccountOnly           EvidenceType = "account_only"
	EvidenceUnknown               EvidenceType = "unknown"
)

var EvidenceTypes = []EvidenceType{
	EvidencePlatformLinkedContent, EvidenceContentComment, EvidenceCustomerStatement,
	EvidenceDedicatedChannel, EvidenceAccountOnly, EvidenceUnknown,
}

// TouchRole is R-061's "首次了解、预约前触点等角色", ruling Q5=A.
type TouchRole string

const (
	RoleFirstTouch TouchRole = "first_touch"
	RolePreBooking TouchRole = "pre_booking"
	RoleOther      TouchRole = "other"
)

var TouchRoles = []TouchRole{RoleFirstTouch, RolePreBooking, RoleOther}

// GrossBasis is where a deal's gross profit comes from.
type GrossBasis string

const (
	GrossNone   GrossBasis = "none"
	GrossStated GrossBasis = "stated_gross_profit"
	GrossCOGS   GrossBasis = "cogs"
)

var GrossBases = []GrossBasis{GrossNone, GrossStated, GrossCOGS}

// AdjustmentKind is R-061's "退款/调整".
type AdjustmentKind string

const (
	AdjustmentRefund AdjustmentKind = "refund"
	AdjustmentOther  AdjustmentKind = "adjustment"
)

var AdjustmentKinds = []AdjustmentKind{AdjustmentRefund, AdjustmentOther}

// RecordSource is written by the server from the endpoint used, never read
// from a request body - the 027 rule.
type RecordSource string

const (
	RecordManual RecordSource = "manual"
	RecordImport RecordSource = "import"
)

var RecordSources = []RecordSource{RecordManual, RecordImport}

// The four platforms Platforms leaves out. A touch may name any of
// ip-profile's eight; a manual metric still only the four 025 delivers to.
const (
	PlatformBilibili Platform = "bilibili"
	PlatformZhihu    Platform = "zhihu"
	PlatformWeibo    Platform = "weibo"
	PlatformKuaishou Platform = "kuaishou"
)

// TouchPlatforms is ip-profile's eight, in its order. Restated rather than
// imported (ip-profile is not a declared dependency); a test reads
// ip-profile's source and compares, so the two cannot drift. An empty platform
// is also accepted on a touch and is not a member.
var TouchPlatforms = []Platform{
	PlatformXiaohongshu, PlatformDouyin, PlatformWechatMP, PlatformBilibili,
	PlatformZhihu, PlatformWeibo, PlatformKuaishou, PlatformShipinhao,
}

// Length bounds, counted in runes.
const (
	MaxCustomerRefRunes = 100
	MaxStageRunes       = 100
	MaxOrderRefRunes    = 200
	MaxConfirmations    = 50
	// MaxLaborMinutes keeps labor_minutes inside its integer column.
	MaxLaborMinutes = 1_000_000_000
)

// CostRevision is one revision of one cost.
type CostRevision struct {
	WorkspaceID string  `json:"workspace_id"`
	CostID      string  `json:"cost_id"`
	Revision    int     `json:"revision"`
	Voided      bool    `json:"voided"`
	Category    string  `json:"category"`
	Pricing     Pricing `json:"pricing"`
	// AmountMinor is nil exactly when a labor cost has no rate: "not
	// computable", which AmountStatus says in words. It is never 0 for that.
	AmountMinor    *Minor       `json:"amount_minor"`
	Amount         *string      `json:"amount"`
	AmountStatus   string       `json:"amount_status"`
	Currency       string       `json:"currency"`
	LaborMinutes   *int64       `json:"labor_minutes"`
	LaborRateMinor *Minor       `json:"labor_rate_minor"`
	LaborRate      *string      `json:"labor_rate"`
	IncurredAt     time.Time    `json:"incurred_at"`
	AdSpend        bool         `json:"ad_spend"`
	AccountID      string       `json:"account_id"`
	WorkID         string       `json:"work_id"`
	CampaignLabel  string       `json:"campaign_label"`
	EvidenceNote   string       `json:"evidence_note"`
	Note           string       `json:"note"`
	DedupeKey      string       `json:"dedupe_key"`
	NotDuplicateOf []string     `json:"not_duplicate_of"`
	SourceType     RecordSource `json:"source_type"`
	ImportBatchID  string       `json:"import_batch_id"`
	RecordedBy     string       `json:"recorded_by"`
	CreatedAt      time.Time    `json:"created_at"`
	// Allocations are this revision's shares, ordered by target; empty for a
	// cost that is not shared (specs/034 PR 2).
	Allocations []CostAllocation `json:"allocations"`

	amountMinor    *int64
	laborRateMinor *int64
}

// Amount status values of a cost.
const (
	AmountOK               = "ok"
	AmountLaborRateMissing = "labor_rate_missing"
)

// LeadRevision is one revision of one lead. CustomerRef is a label the
// operator chose; there is no field for who the customer really is.
type LeadRevision struct {
	WorkspaceID    string       `json:"workspace_id"`
	LeadID         string       `json:"lead_id"`
	Revision       int          `json:"revision"`
	Voided         bool         `json:"voided"`
	CustomerRef    string       `json:"customer_ref"`
	Stage          string       `json:"stage"`
	Qualified      bool         `json:"qualified"`
	FirstSeenAt    time.Time    `json:"first_seen_at"`
	MergedInto     string       `json:"merged_into"`
	Note           string       `json:"note"`
	DedupeKey      string       `json:"dedupe_key"`
	NotDuplicateOf []string     `json:"not_duplicate_of"`
	SourceType     RecordSource `json:"source_type"`
	ImportBatchID  string       `json:"import_batch_id"`
	RecordedBy     string       `json:"recorded_by"`
	CreatedAt      time.Time    `json:"created_at"`
}

// TouchRevision is one revision of one touch: evidence, not a judgement.
type TouchRevision struct {
	WorkspaceID         string       `json:"workspace_id"`
	TouchID             string       `json:"touch_id"`
	Revision            int          `json:"revision"`
	Voided              bool         `json:"voided"`
	LeadID              string       `json:"lead_id"`
	EvidenceType        EvidenceType `json:"evidence_type"`
	Platform            string       `json:"platform"`
	AccountID           string       `json:"account_id"`
	WorkID              string       `json:"work_id"`
	PublicationRecordID string       `json:"publication_record_id"`
	Role                TouchRole    `json:"role"`
	Paid                bool         `json:"paid"`
	OccurredAt          time.Time    `json:"occurred_at"`
	EvidenceNote        string       `json:"evidence_note"`
	Note                string       `json:"note"`
	RecordedBy          string       `json:"recorded_by"`
	CreatedAt           time.Time    `json:"created_at"`
}

// DealRevision is one revision of one deal.
type DealRevision struct {
	WorkspaceID      string       `json:"workspace_id"`
	DealID           string       `json:"deal_id"`
	Revision         int          `json:"revision"`
	Voided           bool         `json:"voided"`
	LeadID           string       `json:"lead_id"`
	OrderRef         string       `json:"order_ref"`
	AmountMinor      Minor        `json:"amount_minor"`
	Amount           string       `json:"amount"`
	Currency         string       `json:"currency"`
	ClosedAt         time.Time    `json:"closed_at"`
	GrossBasis       GrossBasis   `json:"gross_basis"`
	GrossProfitMinor *Minor       `json:"gross_profit_minor"`
	GrossProfit      *string      `json:"gross_profit"`
	COGSMinor        *Minor       `json:"cogs_minor"`
	COGS             *string      `json:"cogs"`
	Note             string       `json:"note"`
	DedupeKey        string       `json:"dedupe_key"`
	NotDuplicateOf   []string     `json:"not_duplicate_of"`
	SourceType       RecordSource `json:"source_type"`
	ImportBatchID    string       `json:"import_batch_id"`
	RecordedBy       string       `json:"recorded_by"`
	CreatedAt        time.Time    `json:"created_at"`

	grossProfitMinor *int64
	cogsMinor        *int64
}

// AdjustmentRevision is one revision of one refund or adjustment. The revenue
// delta is a REDUCTION: a refund of 1000.00 is +100000.
type AdjustmentRevision struct {
	WorkspaceID       string         `json:"workspace_id"`
	AdjustmentID      string         `json:"adjustment_id"`
	Revision          int            `json:"revision"`
	Voided            bool           `json:"voided"`
	DealID            string         `json:"deal_id"`
	Kind              AdjustmentKind `json:"kind"`
	RevenueDeltaMinor Minor          `json:"revenue_delta_minor"`
	RevenueDelta      string         `json:"revenue_delta"`
	// GrossDeltaMinor is SIGNED and added to gross profit (FR-025): a
	// reduction of 300.00 is -30000, unlike the revenue delta above.
	// GrossDeltaMinor nil is "the operator did not say" (ruling Q3=A), and
	// makes the deal's gross profit not computable. It is never read as 0.
	GrossDeltaMinor *Minor    `json:"gross_delta_minor"`
	GrossDelta      *string   `json:"gross_delta"`
	Currency        string    `json:"currency"`
	OccurredAt      time.Time `json:"occurred_at"`
	Note            string    `json:"note"`
	RecordedBy      string    `json:"recorded_by"`
	CreatedAt       time.Time `json:"created_at"`

	grossDeltaMinor *int64
}

// CostInput is the body of a new cost or cost revision. Amounts are strings;
// a JSON number fails to decode and the boundary names the field.
type CostInput struct {
	Category       string   `json:"category"`
	Pricing        string   `json:"pricing"`
	Amount         *string  `json:"amount"`
	Currency       string   `json:"currency"`
	LaborMinutes   *int64   `json:"labor_minutes"`
	LaborRate      *string  `json:"labor_rate"`
	IncurredAt     string   `json:"incurred_at"`
	AdSpend        bool     `json:"ad_spend"`
	AccountID      string   `json:"account_id"`
	WorkID         string   `json:"work_id"`
	CampaignLabel  string   `json:"campaign_label"`
	EvidenceNote   string   `json:"evidence_note"`
	Note           string   `json:"note"`
	NotDuplicateOf []string `json:"not_duplicate_of"`
	// Allocations splits a shared cost (PR 2). Absent or null: a revision
	// keeps the previous revision's split, recomputed on its amount; [] says
	// the cost is no longer shared.
	Allocations *[]AllocationInput `json:"allocations"`
}

type LeadInput struct {
	CustomerRef    string   `json:"customer_ref"`
	Stage          string   `json:"stage"`
	Qualified      bool     `json:"qualified"`
	FirstSeenAt    string   `json:"first_seen_at"`
	Note           string   `json:"note"`
	NotDuplicateOf []string `json:"not_duplicate_of"`
}

type TouchInput struct {
	EvidenceType        string `json:"evidence_type"`
	Platform            string `json:"platform"`
	AccountID           string `json:"account_id"`
	WorkID              string `json:"work_id"`
	PublicationRecordID string `json:"publication_record_id"`
	Role                string `json:"role"`
	Paid                bool   `json:"paid"`
	OccurredAt          string `json:"occurred_at"`
	EvidenceNote        string `json:"evidence_note"`
	Note                string `json:"note"`
}

type DealInput struct {
	LeadID         string   `json:"lead_id"`
	OrderRef       string   `json:"order_ref"`
	Amount         *string  `json:"amount"`
	Currency       string   `json:"currency"`
	ClosedAt       string   `json:"closed_at"`
	GrossBasis     string   `json:"gross_basis"`
	GrossProfit    *string  `json:"gross_profit"`
	COGS           *string  `json:"cogs"`
	Note           string   `json:"note"`
	NotDuplicateOf []string `json:"not_duplicate_of"`
}

// AdjustmentInput's Amount is the revenue delta, a reduction; a refund
// larger than the deal's net is refused naming "amount" (FR-024).
type AdjustmentInput struct {
	Kind       string  `json:"kind"`
	Amount     *string `json:"amount"`
	GrossDelta *string `json:"gross_delta"`
	Currency   string  `json:"currency"`
	OccurredAt string  `json:"occurred_at"`
	Note       string  `json:"note"`
}

// Revision carries what every revision request adds to its record's body.
type Revision struct {
	// BaseRevision is the revision the caller edited. Required: without it a
	// second editor silently overwrites the first (FR-013).
	BaseRevision *int
	Voided       bool
}

// The validators below check in the contract's order - controlled sets, then
// required fields, combinations and amount formats, then lengths - and return
// the first failure, naming its field. They touch no database; existence of
// referenced rows is the store's step and comes before them.

func checkSet[T ~string](field, value string, allowed []T) error {
	if !oneOf(value, allowed) {
		return FieldError{Field: field, Reason: "not an allowed value"}
	}
	return nil
}

func checkCurrency(value string) error {
	if _, ok := CurrencyDigits(value); !ok {
		return FieldError{Field: "currency", Reason: "not a supported currency"}
	}
	return nil
}

func parseTime(field, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, FieldError{Field: field, Reason: "not an RFC 3339 timestamp"}
	}
	return parsed.UTC(), nil
}

func checkRunes(field, value string, limit int) error {
	if utf8.RuneCountInString(value) > limit {
		return FieldError{Field: field, Reason: "too long"}
	}
	return nil
}

func checkConfirmations(ids []string) error {
	if len(ids) > MaxConfirmations {
		return FieldError{Field: "not_duplicate_of", Reason: "too many"}
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return FieldError{Field: "not_duplicate_of", Reason: "invalid or missing"}
		}
	}
	return nil
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateCost checks a cost body and returns the revision it describes, with
// the amount parsed (or, for labor without a rate, marked not computable).
func ValidateCost(in CostInput) (CostRevision, error) {
	if err := firstError(
		checkSet("pricing", in.Pricing, Pricings),
		checkCurrency(in.Currency),
	); err != nil {
		return CostRevision{}, err
	}
	record := CostRevision{
		Category: in.Category, Pricing: Pricing(in.Pricing), Currency: in.Currency,
		AdSpend: in.AdSpend, AccountID: in.AccountID, WorkID: in.WorkID,
		CampaignLabel: in.CampaignLabel, EvidenceNote: in.EvidenceNote, Note: in.Note,
		NotDuplicateOf: in.NotDuplicateOf, AmountStatus: AmountOK,
	}
	if strings.TrimSpace(in.Category) == "" {
		return CostRevision{}, invalidField("category")
	}
	incurred, err := parseTime("incurred_at", in.IncurredAt)
	if err != nil {
		return CostRevision{}, err
	}
	record.IncurredAt = incurred
	switch record.Pricing {
	case PricingAmount:
		if in.LaborMinutes != nil {
			return CostRevision{}, FieldError{Field: "labor_minutes", Reason: "only for labor_time"}
		}
		if in.LaborRate != nil {
			return CostRevision{}, FieldError{Field: "labor_rate", Reason: "only for labor_time"}
		}
		if in.Amount == nil {
			return CostRevision{}, invalidField("amount")
		}
		amount, parseErr := ParseAmount("amount", *in.Amount, in.Currency)
		if parseErr != nil {
			return CostRevision{}, parseErr
		}
		record.amountMinor = &amount
	case PricingLaborTime:
		// The amount of a labor cost is the server's to compute. Accepting one
		// would let a client's arithmetic become the stored number.
		if in.Amount != nil {
			return CostRevision{}, FieldError{Field: "amount", Reason: "computed from labor time"}
		}
		if in.LaborMinutes == nil || *in.LaborMinutes <= 0 || *in.LaborMinutes > MaxLaborMinutes {
			return CostRevision{}, invalidField("labor_minutes")
		}
		minutes := *in.LaborMinutes
		record.LaborMinutes = &minutes
		if in.LaborRate == nil {
			// FR-011: still saved, and says why it has no amount.
			record.AmountStatus = AmountLaborRateMissing
			break
		}
		rate, parseErr := ParseAmount("labor_rate", *in.LaborRate, in.Currency)
		if parseErr != nil {
			return CostRevision{}, parseErr
		}
		amount, laborErr := LaborAmount(minutes, rate)
		if laborErr != nil {
			return CostRevision{}, laborErr
		}
		record.laborRateMinor = &rate
		record.amountMinor = &amount
	}
	if err = checkConfirmations(in.NotDuplicateOf); err != nil {
		return CostRevision{}, err
	}
	if err = firstError(
		checkRunes("category", in.Category, MaxShortRunes),
		checkRunes("campaign_label", in.CampaignLabel, MaxShortRunes),
		checkRunes("evidence_note", in.EvidenceNote, MaxNoteRunes),
		checkRunes("note", in.Note, MaxNoteRunes),
	); err != nil {
		return CostRevision{}, err
	}
	record.fill()
	return record, nil
}

// fill derives the JSON-facing amount fields from the parsed ones.
func (c *CostRevision) fill() {
	c.AmountMinor = minorPtr(c.amountMinor)
	c.Amount = formatAmountPtr(c.amountMinor, c.Currency)
	c.LaborRateMinor = minorPtr(c.laborRateMinor)
	c.LaborRate = formatAmountPtr(c.laborRateMinor, c.Currency)
	if c.Pricing == PricingLaborTime && c.amountMinor == nil {
		c.AmountStatus = AmountLaborRateMissing
	} else {
		c.AmountStatus = AmountOK
	}
	if c.NotDuplicateOf == nil {
		c.NotDuplicateOf = []string{}
	}
	if c.Allocations == nil {
		c.Allocations = []CostAllocation{}
	}
}

// ValidateLead checks a lead body.
func ValidateLead(in LeadInput) (LeadRevision, error) {
	firstSeen, err := parseTime("first_seen_at", in.FirstSeenAt)
	if err != nil {
		return LeadRevision{}, err
	}
	if err = checkConfirmations(in.NotDuplicateOf); err != nil {
		return LeadRevision{}, err
	}
	if err = firstError(
		checkRunes("customer_ref", in.CustomerRef, MaxCustomerRefRunes),
		checkRunes("stage", in.Stage, MaxStageRunes),
		checkRunes("note", in.Note, MaxNoteRunes),
	); err != nil {
		return LeadRevision{}, err
	}
	confirmations := in.NotDuplicateOf
	if confirmations == nil {
		confirmations = []string{}
	}
	return LeadRevision{
		CustomerRef: in.CustomerRef, Stage: in.Stage, Qualified: in.Qualified,
		FirstSeenAt: firstSeen, Note: in.Note, NotDuplicateOf: confirmations,
	}, nil
}

// CheckEvidenceFields is FR-019's table: which of work/publication, account
// and platform each evidence type requires, allows or forbids. The first
// violation is named.
//
//	type                     work/publication   account      platform
//	platform_linked_content  at least one       optional     required
//	content_comment          at least one       optional     required
//	customer_statement       optional           optional     optional
//	dedicated_channel        optional           optional     optional
//	account_only             must be empty      required     required
//	unknown                  must be empty      must be empty must be empty
func CheckEvidenceFields(evidence EvidenceType, platform, accountID, workID, publicationRecordID string) error {
	mustBeEmpty := func(field, value string) error {
		if value != "" {
			return FieldError{Field: field, Reason: "must be empty for this evidence type"}
		}
		return nil
	}
	required := func(field, value string) error {
		if value == "" {
			return FieldError{Field: field, Reason: "required for this evidence type"}
		}
		return nil
	}
	switch evidence {
	case EvidencePlatformLinkedContent, EvidenceContentComment:
		// This kind of evidence points at a piece of content by its nature;
		// without one it is not this kind of evidence.
		if workID == "" && publicationRecordID == "" {
			return FieldError{Field: "work_id", Reason: "required for this evidence type"}
		}
		return required("platform", platform)
	case EvidenceAccountOnly:
		// Knowing the work would make it something other than "account only".
		return firstError(
			mustBeEmpty("work_id", workID),
			mustBeEmpty("publication_record_id", publicationRecordID),
			required("account_id", accountID),
			required("platform", platform),
		)
	case EvidenceUnknown:
		return firstError(
			mustBeEmpty("platform", platform),
			mustBeEmpty("account_id", accountID),
			mustBeEmpty("work_id", workID),
			mustBeEmpty("publication_record_id", publicationRecordID),
		)
	}
	return nil
}

// ValidateTouch checks a touch body.
func ValidateTouch(in TouchInput) (TouchRevision, error) {
	if in.Platform != "" {
		if err := checkSet("platform", in.Platform, TouchPlatforms); err != nil {
			return TouchRevision{}, err
		}
	}
	if err := firstError(
		checkSet("evidence_type", in.EvidenceType, EvidenceTypes),
		checkSet("role", in.Role, TouchRoles),
	); err != nil {
		return TouchRevision{}, err
	}
	if err := CheckEvidenceFields(EvidenceType(in.EvidenceType), in.Platform, in.AccountID,
		in.WorkID, in.PublicationRecordID); err != nil {
		return TouchRevision{}, err
	}
	occurred, err := parseTime("occurred_at", in.OccurredAt)
	if err != nil {
		return TouchRevision{}, err
	}
	if err = firstError(
		checkRunes("evidence_note", in.EvidenceNote, MaxNoteRunes),
		checkRunes("note", in.Note, MaxNoteRunes),
	); err != nil {
		return TouchRevision{}, err
	}
	return TouchRevision{
		EvidenceType: EvidenceType(in.EvidenceType), Platform: in.Platform,
		AccountID: in.AccountID, WorkID: in.WorkID, PublicationRecordID: in.PublicationRecordID,
		Role: TouchRole(in.Role), Paid: in.Paid, OccurredAt: occurred,
		EvidenceNote: in.EvidenceNote, Note: in.Note,
	}, nil
}

// ValidateDeal checks a deal body. Gross profit under 'cogs' is not stored:
// it is amount minus cost of goods, derived by whoever reads it (FR-025).
func ValidateDeal(in DealInput) (DealRevision, error) {
	if err := firstError(
		checkSet("gross_basis", in.GrossBasis, GrossBases),
		checkCurrency(in.Currency),
	); err != nil {
		return DealRevision{}, err
	}
	if in.Amount == nil {
		return DealRevision{}, invalidField("amount")
	}
	amount, err := ParseAmount("amount", *in.Amount, in.Currency)
	if err != nil {
		return DealRevision{}, err
	}
	closed, err := parseTime("closed_at", in.ClosedAt)
	if err != nil {
		return DealRevision{}, err
	}
	record := DealRevision{
		LeadID: in.LeadID, OrderRef: in.OrderRef, Currency: in.Currency,
		ClosedAt: closed, GrossBasis: GrossBasis(in.GrossBasis), Note: in.Note,
		NotDuplicateOf: in.NotDuplicateOf,
	}
	record.AmountMinor = Minor(amount)
	unexpected := func(field string) error {
		return FieldError{Field: field, Reason: "not used by this gross basis"}
	}
	switch record.GrossBasis {
	case GrossNone:
		if in.GrossProfit != nil {
			return DealRevision{}, unexpected("gross_profit")
		}
		if in.COGS != nil {
			return DealRevision{}, unexpected("cogs")
		}
	case GrossStated:
		if in.COGS != nil {
			return DealRevision{}, unexpected("cogs")
		}
		if in.GrossProfit == nil {
			return DealRevision{}, invalidField("gross_profit")
		}
		profit, parseErr := ParseAmount("gross_profit", *in.GrossProfit, in.Currency)
		if parseErr != nil {
			return DealRevision{}, parseErr
		}
		record.grossProfitMinor = &profit
	case GrossCOGS:
		if in.GrossProfit != nil {
			return DealRevision{}, unexpected("gross_profit")
		}
		if in.COGS == nil {
			return DealRevision{}, invalidField("cogs")
		}
		cogs, parseErr := ParseAmount("cogs", *in.COGS, in.Currency)
		if parseErr != nil {
			return DealRevision{}, parseErr
		}
		record.cogsMinor = &cogs
	}
	if err = checkConfirmations(in.NotDuplicateOf); err != nil {
		return DealRevision{}, err
	}
	if err = firstError(
		checkRunes("order_ref", in.OrderRef, MaxOrderRefRunes),
		checkRunes("note", in.Note, MaxNoteRunes),
	); err != nil {
		return DealRevision{}, err
	}
	record.fill()
	return record, nil
}

func (d *DealRevision) fill() {
	d.Amount = FormatAmount(int64(d.AmountMinor), d.Currency)
	d.GrossProfitMinor = minorPtr(d.grossProfitMinor)
	d.GrossProfit = formatAmountPtr(d.grossProfitMinor, d.Currency)
	d.COGSMinor = minorPtr(d.cogsMinor)
	d.COGS = formatAmountPtr(d.cogsMinor, d.Currency)
	if d.NotDuplicateOf == nil {
		d.NotDuplicateOf = []string{}
	}
}

// ValidateAdjustment checks a refund or adjustment body. A refund is a
// reduction and so is positive; zero or negative is refused naming amount.
// Whether it exceeds the deal's net needs the deal and is the store's check.
func ValidateAdjustment(in AdjustmentInput) (AdjustmentRevision, error) {
	if err := firstError(
		checkSet("kind", in.Kind, AdjustmentKinds),
		checkCurrency(in.Currency),
	); err != nil {
		return AdjustmentRevision{}, err
	}
	if in.Amount == nil {
		return AdjustmentRevision{}, invalidField("amount")
	}
	delta, err := ParseAmount("amount", *in.Amount, in.Currency)
	if err != nil {
		return AdjustmentRevision{}, err
	}
	if AdjustmentKind(in.Kind) == AdjustmentRefund && delta <= 0 {
		return AdjustmentRevision{}, FieldError{Field: "amount", Reason: "a refund is a positive reduction"}
	}
	record := AdjustmentRevision{
		Kind: AdjustmentKind(in.Kind), Currency: in.Currency, Note: in.Note,
		RevenueDeltaMinor: Minor(delta),
	}
	if in.GrossDelta != nil {
		gross, parseErr := ParseAmount("gross_delta", *in.GrossDelta, in.Currency)
		if parseErr != nil {
			return AdjustmentRevision{}, parseErr
		}
		record.grossDeltaMinor = &gross
	}
	occurred, err := parseTime("occurred_at", in.OccurredAt)
	if err != nil {
		return AdjustmentRevision{}, err
	}
	record.OccurredAt = occurred
	if err = checkRunes("note", in.Note, MaxNoteRunes); err != nil {
		return AdjustmentRevision{}, err
	}
	record.fill()
	return record, nil
}

func (a *AdjustmentRevision) fill() {
	a.RevenueDelta = FormatAmount(int64(a.RevenueDeltaMinor), a.Currency)
	a.GrossDeltaMinor = minorPtr(a.grossDeltaMinor)
	a.GrossDelta = formatAmountPtr(a.grossDeltaMinor, a.Currency)
}
