package feedbacklearning

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Storage for costs, leads, touches, deals and refunds/adjustments
// (specs/034 PR 1).
//
// Every write path is the same four steps in one transaction, and the order
// is the point:
//
//  1. the workspace delete fence, first (begin), so a deletion that already
//     committed cannot be followed by an orphan row;
//  2. the contract's decision order - the path id and every referenced row
//     exist here, then controlled sets, required fields and combinations,
//     lengths, then base_revision, then possible duplicates;
//  3. one INSERT of the next revision - never an UPDATE, never a DELETE;
//  4. the audit event, in the same transaction, so a write nobody can account
//     for cannot commit.
//
// Contract: specs/034-roi-review/contracts/roi-review.md

// Accounts answers whether a content account exists in a workspace. The
// adapter asks ip-profile; this module does not import it.
type Accounts interface {
	AccountExists(ctx context.Context, workspaceID, accountID string) error
}

// Works answers whether a work exists in a workspace. The adapter asks
// work-editor; this module does not import it.
type Works interface {
	WorkExists(ctx context.Context, workspaceID, actor, workID string) error
}

// Timezones answers the brand's timezone (loretide.timezone), which decides
// which calendar day a dedupe key uses.
type Timezones interface {
	Location(ctx context.Context, workspaceID string) *time.Location
}

// ROIStore is the ROI half of this module. It shares Store's database, fence,
// audit sink and publication adapter, and adds the three adapters the ROI
// records need.
type ROIStore struct {
	*Store
	Accounts  Accounts
	Works     Works
	Timezones Timezones
	// BeforeRevisionInsert is a test seam and nil in production. It runs
	// after every check has passed and right before the INSERT, which is the
	// window in which two concurrent writers can both believe they hold the
	// latest revision; a test parks both writers there to prove the unique
	// index, and not only the read, turns the loser into a 409.
	BeforeRevisionInsert func(ctx context.Context, kind, id string)
}

// ROIListFilter narrows a list. From is inclusive, To exclusive, both on the
// record's own time (incurred, first seen, closed). IncludeInactive also
// returns voided records, and merged leads.
type ROIListFilter struct {
	From            *time.Time
	To              *time.Time
	AccountID       string
	WorkID          string
	LeadID          string
	IncludeInactive bool
}

// CostHistory is one cost with every revision, oldest first.
type CostHistory struct {
	CostID    string         `json:"cost_id"`
	Current   CostRevision   `json:"current"`
	Revisions []CostRevision `json:"revisions"`
}

// TouchHistory is one touch with every revision, oldest first.
type TouchHistory struct {
	TouchID   string          `json:"touch_id"`
	Current   TouchRevision   `json:"current"`
	Revisions []TouchRevision `json:"revisions"`
}

// LeadDetail is one lead, its revisions, and the touches under it - its own
// and those of every lead merged into it, each touch once.
type LeadDetail struct {
	LeadID    string         `json:"lead_id"`
	Current   LeadRevision   `json:"current"`
	Revisions []LeadRevision `json:"revisions"`
	Touches   []TouchHistory `json:"touches"`
}

// AdjustmentHistory is one refund or adjustment with every revision.
type AdjustmentHistory struct {
	AdjustmentID string               `json:"adjustment_id"`
	Current      AdjustmentRevision   `json:"current"`
	Revisions    []AdjustmentRevision `json:"revisions"`
}

// DealDetail is one deal, its revisions and its refunds/adjustments.
type DealDetail struct {
	DealID      string              `json:"deal_id"`
	Current     DealRevision        `json:"current"`
	Revisions   []DealRevision      `json:"revisions"`
	Adjustments []AdjustmentHistory `json:"adjustments"`
}

// MergeInput merges a lead into another; an empty target unmerges it.
type MergeInput struct {
	TargetLeadID string `json:"target_lead_id"`
	Note         string `json:"note"`
}

// maxMergeChain bounds a walk along merged_into. Real chains are one or two
// long; the bound is only there so a corrupted cycle cannot spin forever.
const maxMergeChain = 64

func (s *ROIStore) ready() bool {
	return s != nil && s.Store != nil && s.DB != nil
}

func (s *ROIStore) fail(ctx context.Context, workspaceID, actor, objectID, step string, err error) {
	if !s.ready() {
		return
	}
	// A conflict is the caller's input meeting somebody else's, not a
	// database failure; it is logged as the input conflict it is.
	if errors.Is(err, ErrConflict) {
		err = ErrInvalid
	}
	s.reportFailure(ctx, workspaceID, actor, objectID, step, err)
}

// inTx runs one write path. body returns the object id for the audit event.
func (s *ROIStore) inTx(ctx context.Context, workspaceID, actor, step string,
	body func(ctx context.Context, tx pgx.Tx) (string, error)) error {
	if !s.ready() {
		return ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.fail(ctx, workspaceID, actor, "", step, ErrInvalid)
		return ErrInvalid
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.fail(ctx, workspaceID, actor, "", step, err)
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	objectID, err := body(ctx, tx)
	if err != nil {
		s.fail(ctx, workspaceID, actor, objectID, step, err)
		return err
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, objectID, step); err != nil {
		s.fail(ctx, workspaceID, actor, objectID, step, err)
		return ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.fail(ctx, workspaceID, actor, objectID, step, err)
		return ErrStorage
	}
	return nil
}

func (s *ROIStore) location(ctx context.Context, workspaceID string) *time.Location {
	if s.Timezones != nil {
		if location := s.Timezones.Location(ctx, workspaceID); location != nil {
			return location
		}
	}
	// The brand default (specs/004) when nothing answers.
	if location, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return location
	}
	return time.UTC
}

func adapterError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return ErrNotFound
	default:
		return ErrStorage
	}
}

func (s *ROIStore) checkAccount(ctx context.Context, workspaceID, accountID string) error {
	if accountID == "" {
		return nil
	}
	if s.Accounts == nil {
		return ErrStorage
	}
	return adapterError(s.Accounts.AccountExists(ctx, workspaceID, accountID))
}

func (s *ROIStore) checkWork(ctx context.Context, workspaceID, actor, workID string) error {
	if workID == "" {
		return nil
	}
	if s.Works == nil {
		return ErrStorage
	}
	return adapterError(s.Works.WorkExists(ctx, workspaceID, actor, workID))
}

func (s *ROIStore) checkPublication(ctx context.Context, workspaceID, publicationRecordID string) error {
	if publicationRecordID == "" {
		return nil
	}
	if s.Publications == nil {
		return ErrStorage
	}
	_, _, _, err := s.Publications.Resolve(ctx, workspaceID, publicationRecordID)
	return adapterError(err)
}

func (s *ROIStore) beforeInsert(ctx context.Context, kind, id string) {
	if s.BeforeRevisionInsert != nil {
		s.BeforeRevisionInsert(ctx, kind, id)
	}
}

// checkBase is decision step 6.
func checkBase(revision Revision, latest int) error {
	if revision.BaseRevision == nil {
		return FieldError{Field: "base_revision", Reason: "invalid or missing"}
	}
	if *revision.BaseRevision != latest {
		return RevisionConflict{Field: "base_revision"}
	}
	return nil
}

// insertError maps a failed INSERT. A unique violation is the second writer
// of the same revision number losing the race past the read check: the same
// 409 the read check would have given it.
func insertError(err error) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		return RevisionConflict{Field: "base_revision"}
	}
	return ErrStorage
}

func queryIDs(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, ErrStorage
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, ErrStorage
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	return ids, nil
}

// confirmDuplicates is decision step 7. Every match must be named in
// not_duplicate_of; if any is not, the write stops and says which. When the
// caller did confirm, that confirmation is its own audit event (FR-026).
func (s *ROIStore) confirmDuplicates(ctx context.Context, tx pgx.Tx, workspaceID, actor, objectID string,
	matches, confirmations []string) error {
	unconfirmed := []string{}
	for _, match := range matches {
		if !slices.Contains(confirmations, match) {
			unconfirmed = append(unconfirmed, match)
		}
	}
	if len(unconfirmed) > 0 {
		return PossibleDuplicate{Matches: unconfirmed}
	}
	if len(matches) == 0 {
		return nil
	}
	if _, err := s.audit(ctx, tx, workspaceID, actor, objectID, "confirm-not-duplicate"); err != nil {
		return ErrStorage
	}
	return nil
}

// lockDeal serializes writes that check a deal's net amount, so two refunds
// submitted together cannot each see the other's absence and together take
// the net below zero. Transaction-scoped; released at commit or rollback.
func lockDeal(ctx context.Context, tx pgx.Tx, workspaceID, dealID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"content-roi-deal:"+workspaceID+":"+dealID); err != nil {
		return ErrStorage
	}
	return nil
}

func writeStep(kind string, create, voided bool) string {
	switch {
	case create:
		return "record-" + kind
	case voided:
		return "void-" + kind
	default:
		return "revise-" + kind
	}
}

// ---------------------------------------------------------------- costs

const costColumns = `workspace_id, cost_id, revision, voided, category, pricing,
	amount_minor, currency, labor_minutes, labor_rate_minor, incurred_at, ad_spend,
	account_id, work_id, campaign_label, evidence_note, note, dedupe_key,
	not_duplicate_of, source_type, import_batch_id, recorded_by, created_at`

func scanCost(row scanner) (CostRevision, error) {
	var cost CostRevision
	var pricing, source string
	err := row.Scan(&cost.WorkspaceID, &cost.CostID, &cost.Revision, &cost.Voided,
		&cost.Category, &pricing, &cost.amountMinor, &cost.Currency, &cost.LaborMinutes,
		&cost.laborRateMinor, &cost.IncurredAt, &cost.AdSpend, &cost.AccountID, &cost.WorkID,
		&cost.CampaignLabel, &cost.EvidenceNote, &cost.Note, &cost.DedupeKey,
		&cost.NotDuplicateOf, &source, &cost.ImportBatchID, &cost.RecordedBy, &cost.CreatedAt)
	cost.Pricing = Pricing(pricing)
	cost.SourceType = RecordSource(source)
	cost.IncurredAt = cost.IncurredAt.UTC()
	cost.CreatedAt = cost.CreatedAt.UTC()
	cost.fill()
	return cost, err
}

func (s *ROIStore) latestCost(ctx context.Context, tx pgx.Tx, workspaceID, costID string) (CostRevision, error) {
	cost, err := scanCost(tx.QueryRow(ctx, `SELECT `+costColumns+`
		FROM content_roi_cost_revision WHERE workspace_id=$1 AND cost_id=$2
		ORDER BY revision DESC LIMIT 1`, workspaceID, costID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CostRevision{}, ErrNotFound
	}
	if err != nil {
		return CostRevision{}, ErrStorage
	}
	return cost, nil
}

// CreateCost writes revision 1 of a new cost. source is the entry point's.
func (s *ROIStore) CreateCost(ctx context.Context, workspaceID, actor string, in CostInput, source RecordSource) (CostRevision, error) {
	return s.writeCost(ctx, workspaceID, actor, "", in, Revision{}, source)
}

// ReviseCost writes the next revision of a cost, voiding it when asked.
func (s *ROIStore) ReviseCost(ctx context.Context, workspaceID, actor, costID string, in CostInput, revision Revision) (CostRevision, error) {
	if costID == "" {
		return CostRevision{}, ErrNotFound
	}
	return s.writeCost(ctx, workspaceID, actor, costID, in, revision, RecordManual)
}

func (s *ROIStore) writeCost(ctx context.Context, workspaceID, actor, costID string, in CostInput,
	revision Revision, source RecordSource) (CostRevision, error) {
	var written CostRevision
	create := costID == ""
	err := s.inTx(ctx, workspaceID, actor, writeStep("cost", create, revision.Voided),
		func(ctx context.Context, tx pgx.Tx) (string, error) {
			var previous *CostRevision
			if !create {
				latest, err := s.latestCost(ctx, tx, workspaceID, costID)
				if err != nil {
					return costID, err
				}
				previous = &latest
			}
			if err := firstError(
				s.checkAccount(ctx, workspaceID, in.AccountID),
				s.checkWork(ctx, workspaceID, actor, in.WorkID),
			); err != nil {
				return costID, err
			}
			record, err := ValidateCost(in)
			if err != nil {
				return costID, err
			}
			record.Revision = 1
			if previous != nil {
				if err = checkBase(revision, previous.Revision); err != nil {
					return costID, err
				}
				record.Revision = previous.Revision + 1
				if in.NotDuplicateOf == nil {
					record.NotDuplicateOf = previous.NotDuplicateOf
				}
			} else {
				costID = s.newID()
			}
			record.WorkspaceID, record.CostID, record.Voided = workspaceID, costID, revision.Voided
			record.SourceType, record.RecordedBy = source, actor
			record.DedupeKey = CostDedupeKey(record, s.location(ctx, workspaceID))
			if !record.Voided {
				matches, err := queryIDs(ctx, tx, `SELECT r.cost_id FROM content_roi_cost_revision r
					WHERE r.workspace_id=$1 AND r.dedupe_key=$2 AND r.cost_id<>$3 AND NOT r.voided
					  AND r.revision = (SELECT l.revision FROM content_roi_cost_revision l
					      WHERE l.workspace_id=r.workspace_id AND l.cost_id=r.cost_id
					      ORDER BY l.revision DESC LIMIT 1)
					ORDER BY r.cost_id`, workspaceID, record.DedupeKey, costID)
				if err != nil {
					return costID, err
				}
				if err = s.confirmDuplicates(ctx, tx, workspaceID, actor, costID, matches, record.NotDuplicateOf); err != nil {
					return costID, err
				}
			}
			s.beforeInsert(ctx, "cost", costID)
			if err = tx.QueryRow(ctx, `INSERT INTO content_roi_cost_revision
				(workspace_id, cost_id, revision, voided, category, pricing, amount_minor,
				 currency, labor_minutes, labor_rate_minor, incurred_at, ad_spend, account_id,
				 work_id, campaign_label, evidence_note, note, dedupe_key, not_duplicate_of,
				 source_type, import_batch_id, recorded_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,'',$21)
				RETURNING created_at`,
				workspaceID, costID, record.Revision, record.Voided, record.Category,
				string(record.Pricing), record.amountMinor, record.Currency, record.LaborMinutes,
				record.laborRateMinor, record.IncurredAt, record.AdSpend, record.AccountID,
				record.WorkID, record.CampaignLabel, record.EvidenceNote, record.Note,
				record.DedupeKey, record.NotDuplicateOf, string(source), actor).
				Scan(&record.CreatedAt); err != nil {
				return costID, insertError(err)
			}
			record.CreatedAt = record.CreatedAt.UTC()
			record.fill()
			written = record
			return costID, nil
		})
	if err != nil {
		return CostRevision{}, err
	}
	return written, nil
}

// ListCosts returns the latest revision of each cost, newest first. Voided
// costs only with IncludeInactive.
func (s *ROIStore) ListCosts(ctx context.Context, workspaceID, actor string, filter ROIListFilter) ([]CostRevision, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+costColumns+` FROM (
			SELECT DISTINCT ON (cost_id) `+costColumns+`
			FROM content_roi_cost_revision WHERE workspace_id=$1
			ORDER BY cost_id, revision DESC) latest
		WHERE ($2::boolean OR NOT voided)
		  AND ($3::timestamptz IS NULL OR incurred_at >= $3::timestamptz)
		  AND ($4::timestamptz IS NULL OR incurred_at < $4::timestamptz)
		  AND ($5::text = '' OR account_id = $5::text)
		  AND ($6::text = '' OR work_id = $6::text)
		ORDER BY incurred_at DESC, cost_id`,
		workspaceID, filter.IncludeInactive, filter.From, filter.To, filter.AccountID, filter.WorkID)
	if err != nil {
		s.fail(ctx, workspaceID, actor, "", "list-costs", err)
		return nil, ErrStorage
	}
	return collect(rows, scanCost)
}

// GetCost returns every revision of one cost, oldest first.
func (s *ROIStore) GetCost(ctx context.Context, workspaceID, actor, costID string) (CostHistory, error) {
	if !s.ready() {
		return CostHistory{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return CostHistory{}, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+costColumns+` FROM content_roi_cost_revision
		WHERE workspace_id=$1 AND cost_id=$2 ORDER BY revision`, workspaceID, costID)
	if err != nil {
		return CostHistory{}, ErrStorage
	}
	revisions, err := collect(rows, scanCost)
	if err != nil {
		return CostHistory{}, err
	}
	if len(revisions) == 0 {
		return CostHistory{}, ErrNotFound
	}
	return CostHistory{CostID: costID, Current: revisions[len(revisions)-1], Revisions: revisions}, nil
}

// collect scans every row, or fails as a whole.
func collect[T any](rows pgx.Rows, scan func(scanner) (T, error)) ([]T, error) {
	defer rows.Close()
	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, ErrStorage
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	return items, nil
}

// ---------------------------------------------------------------- leads

const leadColumns = `workspace_id, lead_id, revision, voided, customer_ref, stage,
	qualified, first_seen_at, merged_into, note, dedupe_key, not_duplicate_of,
	source_type, import_batch_id, recorded_by, created_at`

func scanLead(row scanner) (LeadRevision, error) {
	var lead LeadRevision
	var source string
	err := row.Scan(&lead.WorkspaceID, &lead.LeadID, &lead.Revision, &lead.Voided,
		&lead.CustomerRef, &lead.Stage, &lead.Qualified, &lead.FirstSeenAt, &lead.MergedInto,
		&lead.Note, &lead.DedupeKey, &lead.NotDuplicateOf, &source, &lead.ImportBatchID,
		&lead.RecordedBy, &lead.CreatedAt)
	lead.SourceType = RecordSource(source)
	lead.FirstSeenAt = lead.FirstSeenAt.UTC()
	lead.CreatedAt = lead.CreatedAt.UTC()
	if lead.NotDuplicateOf == nil {
		lead.NotDuplicateOf = []string{}
	}
	return lead, err
}

func latestLead(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, workspaceID, leadID string) (LeadRevision, error) {
	lead, err := scanLead(q.QueryRow(ctx, `SELECT `+leadColumns+`
		FROM content_roi_lead_revision WHERE workspace_id=$1 AND lead_id=$2
		ORDER BY revision DESC LIMIT 1`, workspaceID, leadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadRevision{}, ErrNotFound
	}
	if err != nil {
		return LeadRevision{}, ErrStorage
	}
	return lead, nil
}

func (s *ROIStore) CreateLead(ctx context.Context, workspaceID, actor string, in LeadInput, source RecordSource) (LeadRevision, error) {
	return s.writeLead(ctx, workspaceID, actor, "", in, Revision{}, source)
}

func (s *ROIStore) ReviseLead(ctx context.Context, workspaceID, actor, leadID string, in LeadInput, revision Revision) (LeadRevision, error) {
	if leadID == "" {
		return LeadRevision{}, ErrNotFound
	}
	return s.writeLead(ctx, workspaceID, actor, leadID, in, revision, RecordManual)
}

func (s *ROIStore) writeLead(ctx context.Context, workspaceID, actor, leadID string, in LeadInput,
	revision Revision, source RecordSource) (LeadRevision, error) {
	var written LeadRevision
	create := leadID == ""
	err := s.inTx(ctx, workspaceID, actor, writeStep("lead", create, revision.Voided),
		func(ctx context.Context, tx pgx.Tx) (string, error) {
			var previous *LeadRevision
			if !create {
				latest, err := latestLead(ctx, tx, workspaceID, leadID)
				if err != nil {
					return leadID, err
				}
				previous = &latest
			}
			record, err := ValidateLead(in)
			if err != nil {
				return leadID, err
			}
			record.Revision = 1
			if previous != nil {
				if err = checkBase(revision, previous.Revision); err != nil {
					return leadID, err
				}
				record.Revision = previous.Revision + 1
				// A plain revision keeps the merge as it was; only /merge
				// changes it, and that is audited as a merge.
				record.MergedInto = previous.MergedInto
				if in.NotDuplicateOf == nil {
					record.NotDuplicateOf = previous.NotDuplicateOf
				}
			} else {
				leadID = s.newID()
			}
			record.WorkspaceID, record.LeadID, record.Voided = workspaceID, leadID, revision.Voided
			record.SourceType, record.RecordedBy = source, actor
			record.DedupeKey = LeadDedupeKey(record.CustomerRef, record.FirstSeenAt, s.location(ctx, workspaceID))
			if !record.Voided && record.MergedInto == "" && record.DedupeKey != "" {
				matches, err := queryIDs(ctx, tx, `SELECT r.lead_id FROM content_roi_lead_revision r
					WHERE r.workspace_id=$1 AND r.dedupe_key=$2 AND r.lead_id<>$3
					  AND NOT r.voided AND r.merged_into = ''
					  AND r.revision = (SELECT l.revision FROM content_roi_lead_revision l
					      WHERE l.workspace_id=r.workspace_id AND l.lead_id=r.lead_id
					      ORDER BY l.revision DESC LIMIT 1)
					ORDER BY r.lead_id`, workspaceID, record.DedupeKey, leadID)
				if err != nil {
					return leadID, err
				}
				if err = s.confirmDuplicates(ctx, tx, workspaceID, actor, leadID, matches, record.NotDuplicateOf); err != nil {
					return leadID, err
				}
			}
			s.beforeInsert(ctx, "lead", leadID)
			if err = insertLead(ctx, tx, &record); err != nil {
				return leadID, err
			}
			written = record
			return leadID, nil
		})
	if err != nil {
		return LeadRevision{}, err
	}
	return written, nil
}

func insertLead(ctx context.Context, tx pgx.Tx, lead *LeadRevision) error {
	if err := tx.QueryRow(ctx, `INSERT INTO content_roi_lead_revision
		(workspace_id, lead_id, revision, voided, customer_ref, stage, qualified,
		 first_seen_at, merged_into, note, dedupe_key, not_duplicate_of, source_type,
		 import_batch_id, recorded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING created_at`,
		lead.WorkspaceID, lead.LeadID, lead.Revision, lead.Voided, lead.CustomerRef,
		lead.Stage, lead.Qualified, lead.FirstSeenAt, lead.MergedInto, lead.Note,
		lead.DedupeKey, lead.NotDuplicateOf, string(lead.SourceType), lead.ImportBatchID,
		lead.RecordedBy).Scan(&lead.CreatedAt); err != nil {
		return insertError(err)
	}
	lead.CreatedAt = lead.CreatedAt.UTC()
	return nil
}

// MergeLead writes the next revision of leadID with merged_into set to the
// target, or cleared when the target is "" (FR-022). No touch is touched:
// they are read under the target instead.
func (s *ROIStore) MergeLead(ctx context.Context, workspaceID, actor, leadID string, in MergeInput, revision Revision) (LeadRevision, error) {
	if leadID == "" {
		return LeadRevision{}, ErrNotFound
	}
	step := "merge-lead"
	if in.TargetLeadID == "" {
		step = "unmerge-lead"
	}
	var written LeadRevision
	err := s.inTx(ctx, workspaceID, actor, step, func(ctx context.Context, tx pgx.Tx) (string, error) {
		previous, err := latestLead(ctx, tx, workspaceID, leadID)
		if err != nil {
			return leadID, err
		}
		if in.TargetLeadID != "" {
			target, targetErr := latestLead(ctx, tx, workspaceID, in.TargetLeadID)
			if targetErr != nil {
				return leadID, targetErr
			}
			if in.TargetLeadID == leadID || target.Voided {
				return leadID, FieldError{Field: "merged_into", Reason: "not a lead this one can merge into"}
			}
			// A→B→A: following the target's own merges must not arrive back
			// here, or the two leads would each be counted under the other
			// and under neither.
			current := target
			for range maxMergeChain {
				if current.MergedInto == "" {
					break
				}
				if current.MergedInto == leadID {
					return leadID, FieldError{Field: "merged_into", Reason: "the merge would form a cycle"}
				}
				current, err = latestLead(ctx, tx, workspaceID, current.MergedInto)
				if err != nil {
					return leadID, ErrStorage
				}
			}
		}
		if err = checkRunes("note", in.Note, MaxNoteRunes); err != nil {
			return leadID, err
		}
		if err = checkBase(revision, previous.Revision); err != nil {
			return leadID, err
		}
		record := previous
		record.Revision = previous.Revision + 1
		record.MergedInto = in.TargetLeadID
		record.Note = in.Note
		record.SourceType = RecordManual
		record.ImportBatchID = ""
		record.RecordedBy = actor
		s.beforeInsert(ctx, "lead", leadID)
		if err = insertLead(ctx, tx, &record); err != nil {
			return leadID, err
		}
		written = record
		return leadID, nil
	})
	if err != nil {
		return LeadRevision{}, err
	}
	return written, nil
}

// ListLeads returns the latest revision of each current lead: not voided and
// not merged into another, unless IncludeInactive.
func (s *ROIStore) ListLeads(ctx context.Context, workspaceID, actor string, filter ROIListFilter) ([]LeadRevision, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+leadColumns+` FROM (
			SELECT DISTINCT ON (lead_id) `+leadColumns+`
			FROM content_roi_lead_revision WHERE workspace_id=$1
			ORDER BY lead_id, revision DESC) latest
		WHERE ($2::boolean OR (NOT voided AND merged_into = ''))
		  AND ($3::timestamptz IS NULL OR first_seen_at >= $3::timestamptz)
		  AND ($4::timestamptz IS NULL OR first_seen_at < $4::timestamptz)
		ORDER BY first_seen_at DESC, lead_id`,
		workspaceID, filter.IncludeInactive, filter.From, filter.To)
	if err != nil {
		s.fail(ctx, workspaceID, actor, "", "list-leads", err)
		return nil, ErrStorage
	}
	return collect(rows, scanLead)
}

// GetLead returns a lead's revisions and every touch readable under it: its
// own, and those of each lead whose merge chain passes through it. Each touch
// appears once however many paths lead to it (FR-022).
func (s *ROIStore) GetLead(ctx context.Context, workspaceID, actor, leadID string) (LeadDetail, error) {
	if !s.ready() {
		return LeadDetail{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return LeadDetail{}, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+leadColumns+` FROM content_roi_lead_revision
		WHERE workspace_id=$1 AND lead_id=$2 ORDER BY revision`, workspaceID, leadID)
	if err != nil {
		return LeadDetail{}, ErrStorage
	}
	revisions, err := collect(rows, scanLead)
	if err != nil {
		return LeadDetail{}, err
	}
	if len(revisions) == 0 {
		return LeadDetail{}, ErrNotFound
	}
	members, err := s.mergedMembers(ctx, workspaceID, leadID)
	if err != nil {
		return LeadDetail{}, err
	}
	touchRows, err := s.DB.Query(ctx, `SELECT `+touchColumns+` FROM content_roi_touch_revision
		WHERE workspace_id=$1 AND lead_id = ANY($2::text[]) ORDER BY touch_id, revision`,
		workspaceID, members)
	if err != nil {
		return LeadDetail{}, ErrStorage
	}
	touchRevisions, err := collect(touchRows, scanTouch)
	if err != nil {
		return LeadDetail{}, err
	}
	return LeadDetail{
		LeadID: leadID, Current: revisions[len(revisions)-1], Revisions: revisions,
		Touches: groupTouches(touchRevisions),
	}, nil
}

// mergedMembers is leadID plus every lead whose current merge chain reaches
// it.
func (s *ROIStore) mergedMembers(ctx context.Context, workspaceID, leadID string) ([]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT lead_id, merged_into FROM (
			SELECT DISTINCT ON (lead_id) lead_id, merged_into
			FROM content_roi_lead_revision WHERE workspace_id=$1
			ORDER BY lead_id, revision DESC) latest
		WHERE merged_into <> ''`, workspaceID)
	if err != nil {
		return nil, ErrStorage
	}
	defer rows.Close()
	parent := map[string]string{}
	for rows.Next() {
		var child, into string
		if err = rows.Scan(&child, &into); err != nil {
			return nil, ErrStorage
		}
		parent[child] = into
	}
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	members := []string{leadID}
	for child := range parent {
		current := child
		for range maxMergeChain {
			next, merged := parent[current]
			if !merged {
				break
			}
			if next == leadID {
				members = append(members, child)
				break
			}
			current = next
		}
	}
	sort.Strings(members[1:])
	return members, nil
}

// ---------------------------------------------------------------- touches

const touchColumns = `workspace_id, touch_id, revision, voided, lead_id, evidence_type,
	platform, account_id, work_id, publication_record_id, role, paid, occurred_at,
	evidence_note, note, recorded_by, created_at`

func scanTouch(row scanner) (TouchRevision, error) {
	var touch TouchRevision
	var evidence, role string
	err := row.Scan(&touch.WorkspaceID, &touch.TouchID, &touch.Revision, &touch.Voided,
		&touch.LeadID, &evidence, &touch.Platform, &touch.AccountID, &touch.WorkID,
		&touch.PublicationRecordID, &role, &touch.Paid, &touch.OccurredAt,
		&touch.EvidenceNote, &touch.Note, &touch.RecordedBy, &touch.CreatedAt)
	touch.EvidenceType = EvidenceType(evidence)
	touch.Role = TouchRole(role)
	touch.OccurredAt = touch.OccurredAt.UTC()
	touch.CreatedAt = touch.CreatedAt.UTC()
	return touch, err
}

// groupTouches turns revisions ordered by (touch_id, revision) into one
// history per touch, ordered by when the touch happened.
func groupTouches(revisions []TouchRevision) []TouchHistory {
	histories := []TouchHistory{}
	for _, revision := range revisions {
		last := len(histories) - 1
		if last < 0 || histories[last].TouchID != revision.TouchID {
			histories = append(histories, TouchHistory{TouchID: revision.TouchID})
			last++
		}
		histories[last].Revisions = append(histories[last].Revisions, revision)
		histories[last].Current = revision
	}
	sort.SliceStable(histories, func(i, j int) bool {
		left, right := histories[i].Current, histories[j].Current
		if !left.OccurredAt.Equal(right.OccurredAt) {
			return left.OccurredAt.Before(right.OccurredAt)
		}
		return left.TouchID < right.TouchID
	})
	return histories
}

// AddTouch writes revision 1 of a new touch under a lead.
func (s *ROIStore) AddTouch(ctx context.Context, workspaceID, actor, leadID string, in TouchInput) (TouchRevision, error) {
	return s.writeTouch(ctx, workspaceID, actor, leadID, "", in, Revision{})
}

// ReviseTouch writes the next revision of a touch. The touch has to belong to
// the lead in the path; one that belongs elsewhere is answered as missing.
func (s *ROIStore) ReviseTouch(ctx context.Context, workspaceID, actor, leadID, touchID string, in TouchInput, revision Revision) (TouchRevision, error) {
	if touchID == "" {
		return TouchRevision{}, ErrNotFound
	}
	return s.writeTouch(ctx, workspaceID, actor, leadID, touchID, in, revision)
}

func (s *ROIStore) writeTouch(ctx context.Context, workspaceID, actor, leadID, touchID string, in TouchInput,
	revision Revision) (TouchRevision, error) {
	if leadID == "" {
		return TouchRevision{}, ErrNotFound
	}
	var written TouchRevision
	create := touchID == ""
	err := s.inTx(ctx, workspaceID, actor, writeStep("touch", create, revision.Voided),
		func(ctx context.Context, tx pgx.Tx) (string, error) {
			if _, err := latestLead(ctx, tx, workspaceID, leadID); err != nil {
				return touchID, err
			}
			previousRevision := 0
			if !create {
				previous, err := scanTouch(tx.QueryRow(ctx, `SELECT `+touchColumns+`
					FROM content_roi_touch_revision WHERE workspace_id=$1 AND touch_id=$2
					ORDER BY revision DESC LIMIT 1`, workspaceID, touchID))
				if errors.Is(err, pgx.ErrNoRows) || (err == nil && previous.LeadID != leadID) {
					return touchID, ErrNotFound
				}
				if err != nil {
					return touchID, ErrStorage
				}
				previousRevision = previous.Revision
			}
			if err := firstError(
				s.checkAccount(ctx, workspaceID, in.AccountID),
				s.checkWork(ctx, workspaceID, actor, in.WorkID),
				s.checkPublication(ctx, workspaceID, in.PublicationRecordID),
			); err != nil {
				return touchID, err
			}
			record, err := ValidateTouch(in)
			if err != nil {
				return touchID, err
			}
			record.Revision = 1
			if !create {
				if err = checkBase(revision, previousRevision); err != nil {
					return touchID, err
				}
				record.Revision = previousRevision + 1
			} else {
				touchID = s.newID()
			}
			record.WorkspaceID, record.TouchID, record.LeadID = workspaceID, touchID, leadID
			record.Voided, record.RecordedBy = revision.Voided, actor
			s.beforeInsert(ctx, "touch", touchID)
			if err = tx.QueryRow(ctx, `INSERT INTO content_roi_touch_revision
				(workspace_id, touch_id, revision, voided, lead_id, evidence_type, platform,
				 account_id, work_id, publication_record_id, role, paid, occurred_at,
				 evidence_note, note, recorded_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
				RETURNING created_at`,
				workspaceID, touchID, record.Revision, record.Voided, leadID,
				string(record.EvidenceType), record.Platform, record.AccountID, record.WorkID,
				record.PublicationRecordID, string(record.Role), record.Paid, record.OccurredAt,
				record.EvidenceNote, record.Note, actor).Scan(&record.CreatedAt); err != nil {
				return touchID, insertError(err)
			}
			record.CreatedAt = record.CreatedAt.UTC()
			written = record
			return touchID, nil
		})
	if err != nil {
		return TouchRevision{}, err
	}
	return written, nil
}

// ---------------------------------------------------------------- deals

const dealColumns = `workspace_id, deal_id, revision, voided, lead_id, order_ref,
	amount_minor, currency, closed_at, gross_basis, gross_profit_minor, cogs_minor,
	note, dedupe_key, not_duplicate_of, source_type, import_batch_id, recorded_by,
	created_at`

func scanDeal(row scanner) (DealRevision, error) {
	var deal DealRevision
	var amount int64
	var basis, source string
	err := row.Scan(&deal.WorkspaceID, &deal.DealID, &deal.Revision, &deal.Voided,
		&deal.LeadID, &deal.OrderRef, &amount, &deal.Currency, &deal.ClosedAt, &basis,
		&deal.grossProfitMinor, &deal.cogsMinor, &deal.Note, &deal.DedupeKey,
		&deal.NotDuplicateOf, &source, &deal.ImportBatchID, &deal.RecordedBy, &deal.CreatedAt)
	deal.AmountMinor = Minor(amount)
	deal.GrossBasis = GrossBasis(basis)
	deal.SourceType = RecordSource(source)
	deal.ClosedAt = deal.ClosedAt.UTC()
	deal.CreatedAt = deal.CreatedAt.UTC()
	deal.fill()
	return deal, err
}

func latestDeal(ctx context.Context, tx pgx.Tx, workspaceID, dealID string) (DealRevision, error) {
	deal, err := scanDeal(tx.QueryRow(ctx, `SELECT `+dealColumns+`
		FROM content_roi_deal_revision WHERE workspace_id=$1 AND deal_id=$2
		ORDER BY revision DESC LIMIT 1`, workspaceID, dealID))
	if errors.Is(err, pgx.ErrNoRows) {
		return DealRevision{}, ErrNotFound
	}
	if err != nil {
		return DealRevision{}, ErrStorage
	}
	return deal, nil
}

func (s *ROIStore) CreateDeal(ctx context.Context, workspaceID, actor string, in DealInput, source RecordSource) (DealRevision, error) {
	return s.writeDeal(ctx, workspaceID, actor, "", in, Revision{}, source)
}

func (s *ROIStore) ReviseDeal(ctx context.Context, workspaceID, actor, dealID string, in DealInput, revision Revision) (DealRevision, error) {
	if dealID == "" {
		return DealRevision{}, ErrNotFound
	}
	return s.writeDeal(ctx, workspaceID, actor, dealID, in, revision, RecordManual)
}

func (s *ROIStore) writeDeal(ctx context.Context, workspaceID, actor, dealID string, in DealInput,
	revision Revision, source RecordSource) (DealRevision, error) {
	var written DealRevision
	create := dealID == ""
	err := s.inTx(ctx, workspaceID, actor, writeStep("deal", create, revision.Voided),
		func(ctx context.Context, tx pgx.Tx) (string, error) {
			var previous *DealRevision
			if !create {
				if err := lockDeal(ctx, tx, workspaceID, dealID); err != nil {
					return dealID, err
				}
				latest, err := latestDeal(ctx, tx, workspaceID, dealID)
				if err != nil {
					return dealID, err
				}
				previous = &latest
			}
			if in.LeadID != "" {
				if _, err := latestLead(ctx, tx, workspaceID, in.LeadID); err != nil {
					return dealID, err
				}
			}
			record, err := ValidateDeal(in)
			if err != nil {
				return dealID, err
			}
			record.Revision = 1
			if previous != nil {
				if !revision.Voided {
					adjustments, adjErr := currentAdjustments(ctx, tx, workspaceID, dealID)
					if adjErr != nil {
						return dealID, adjErr
					}
					if err = checkNet(int64(record.AmountMinor), record.Currency, adjustments, "", nil); err != nil {
						return dealID, err
					}
				}
				if err = checkBase(revision, previous.Revision); err != nil {
					return dealID, err
				}
				record.Revision = previous.Revision + 1
				if in.NotDuplicateOf == nil {
					record.NotDuplicateOf = previous.NotDuplicateOf
				}
			} else {
				dealID = s.newID()
			}
			record.WorkspaceID, record.DealID, record.Voided = workspaceID, dealID, revision.Voided
			record.SourceType, record.RecordedBy = source, actor
			record.DedupeKey = DealDedupeKey(record.OrderRef, record.LeadID, record.ClosedAt,
				record.Currency, int64(record.AmountMinor), s.location(ctx, workspaceID))
			if !record.Voided {
				matches, err := queryIDs(ctx, tx, `SELECT r.deal_id FROM content_roi_deal_revision r
					WHERE r.workspace_id=$1 AND r.dedupe_key=$2 AND r.deal_id<>$3 AND NOT r.voided
					  AND r.revision = (SELECT l.revision FROM content_roi_deal_revision l
					      WHERE l.workspace_id=r.workspace_id AND l.deal_id=r.deal_id
					      ORDER BY l.revision DESC LIMIT 1)
					ORDER BY r.deal_id`, workspaceID, record.DedupeKey, dealID)
				if err != nil {
					return dealID, err
				}
				if err = s.confirmDuplicates(ctx, tx, workspaceID, actor, dealID, matches, record.NotDuplicateOf); err != nil {
					return dealID, err
				}
			}
			s.beforeInsert(ctx, "deal", dealID)
			if err = tx.QueryRow(ctx, `INSERT INTO content_roi_deal_revision
				(workspace_id, deal_id, revision, voided, lead_id, order_ref, amount_minor,
				 currency, closed_at, gross_basis, gross_profit_minor, cogs_minor, note,
				 dedupe_key, not_duplicate_of, source_type, import_batch_id, recorded_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'',$17)
				RETURNING created_at`,
				workspaceID, dealID, record.Revision, record.Voided, record.LeadID,
				record.OrderRef, int64(record.AmountMinor), record.Currency, record.ClosedAt,
				string(record.GrossBasis), record.grossProfitMinor, record.cogsMinor, record.Note,
				record.DedupeKey, record.NotDuplicateOf, string(source), actor).
				Scan(&record.CreatedAt); err != nil {
				return dealID, insertError(err)
			}
			record.CreatedAt = record.CreatedAt.UTC()
			record.fill()
			written = record
			return dealID, nil
		})
	if err != nil {
		return DealRevision{}, err
	}
	return written, nil
}

// checkNet is FR-024: the deal amount less every current refund/adjustment
// may not go below zero, and every one of them is in the deal's currency.
// skipID is the adjustment being revised, replaced by replacement (nil when
// it is being voided or there is none).
func checkNet(amount int64, currency string, adjustments []AdjustmentRevision, skipID string, replacement *AdjustmentRevision) error {
	net := big.NewInt(amount)
	for _, adjustment := range adjustments {
		if adjustment.AdjustmentID == skipID || adjustment.Voided {
			continue
		}
		if adjustment.Currency != currency {
			return FieldError{Field: "currency", Reason: "must match the deal's currency"}
		}
		net.Sub(net, big.NewInt(int64(adjustment.RevenueDeltaMinor)))
	}
	if replacement != nil {
		net.Sub(net, big.NewInt(int64(replacement.RevenueDeltaMinor)))
	}
	if net.Sign() < 0 {
		return FieldError{Field: "amount", Reason: "would take the deal's net below zero"}
	}
	return nil
}

// ListDeals returns the latest revision of each deal not voided, newest
// first.
func (s *ROIStore) ListDeals(ctx context.Context, workspaceID, actor string, filter ROIListFilter) ([]DealRevision, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+dealColumns+` FROM (
			SELECT DISTINCT ON (deal_id) `+dealColumns+`
			FROM content_roi_deal_revision WHERE workspace_id=$1
			ORDER BY deal_id, revision DESC) latest
		WHERE ($2::boolean OR NOT voided)
		  AND ($3::timestamptz IS NULL OR closed_at >= $3::timestamptz)
		  AND ($4::timestamptz IS NULL OR closed_at < $4::timestamptz)
		  AND ($5::text = '' OR lead_id = $5::text)
		ORDER BY closed_at DESC, deal_id`,
		workspaceID, filter.IncludeInactive, filter.From, filter.To, filter.LeadID)
	if err != nil {
		s.fail(ctx, workspaceID, actor, "", "list-deals", err)
		return nil, ErrStorage
	}
	return collect(rows, scanDeal)
}

// GetDeal returns a deal's revisions and its refunds/adjustments.
func (s *ROIStore) GetDeal(ctx context.Context, workspaceID, actor, dealID string) (DealDetail, error) {
	if !s.ready() {
		return DealDetail{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return DealDetail{}, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+dealColumns+` FROM content_roi_deal_revision
		WHERE workspace_id=$1 AND deal_id=$2 ORDER BY revision`, workspaceID, dealID)
	if err != nil {
		return DealDetail{}, ErrStorage
	}
	revisions, err := collect(rows, scanDeal)
	if err != nil {
		return DealDetail{}, err
	}
	if len(revisions) == 0 {
		return DealDetail{}, ErrNotFound
	}
	adjustmentRows, err := s.DB.Query(ctx, `SELECT `+adjustmentColumns+`
		FROM content_roi_adjustment_revision WHERE workspace_id=$1 AND deal_id=$2
		ORDER BY adjustment_id, revision`, workspaceID, dealID)
	if err != nil {
		return DealDetail{}, ErrStorage
	}
	adjustmentRevisions, err := collect(adjustmentRows, scanAdjustment)
	if err != nil {
		return DealDetail{}, err
	}
	histories := []AdjustmentHistory{}
	for _, revision := range adjustmentRevisions {
		last := len(histories) - 1
		if last < 0 || histories[last].AdjustmentID != revision.AdjustmentID {
			histories = append(histories, AdjustmentHistory{AdjustmentID: revision.AdjustmentID})
			last++
		}
		histories[last].Revisions = append(histories[last].Revisions, revision)
		histories[last].Current = revision
	}
	sort.SliceStable(histories, func(i, j int) bool {
		left, right := histories[i].Current, histories[j].Current
		if !left.OccurredAt.Equal(right.OccurredAt) {
			return left.OccurredAt.Before(right.OccurredAt)
		}
		return left.AdjustmentID < right.AdjustmentID
	})
	return DealDetail{
		DealID: dealID, Current: revisions[len(revisions)-1], Revisions: revisions,
		Adjustments: histories,
	}, nil
}

// ---------------------------------------------------------------- adjustments

const adjustmentColumns = `workspace_id, adjustment_id, revision, voided, deal_id, kind,
	revenue_delta_minor, gross_delta_minor, currency, occurred_at, note, recorded_by,
	created_at`

func scanAdjustment(row scanner) (AdjustmentRevision, error) {
	var adjustment AdjustmentRevision
	var delta int64
	var kind string
	err := row.Scan(&adjustment.WorkspaceID, &adjustment.AdjustmentID, &adjustment.Revision,
		&adjustment.Voided, &adjustment.DealID, &kind, &delta, &adjustment.grossDeltaMinor,
		&adjustment.Currency, &adjustment.OccurredAt, &adjustment.Note, &adjustment.RecordedBy,
		&adjustment.CreatedAt)
	adjustment.Kind = AdjustmentKind(kind)
	adjustment.RevenueDeltaMinor = Minor(delta)
	adjustment.OccurredAt = adjustment.OccurredAt.UTC()
	adjustment.CreatedAt = adjustment.CreatedAt.UTC()
	adjustment.fill()
	return adjustment, err
}

// currentAdjustments is the latest revision of each adjustment on a deal,
// voided ones included; checkNet skips those.
func currentAdjustments(ctx context.Context, tx pgx.Tx, workspaceID, dealID string) ([]AdjustmentRevision, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (adjustment_id) `+adjustmentColumns+`
		FROM content_roi_adjustment_revision WHERE workspace_id=$1 AND deal_id=$2
		ORDER BY adjustment_id, revision DESC`, workspaceID, dealID)
	if err != nil {
		return nil, ErrStorage
	}
	return collect(rows, scanAdjustment)
}

// AddAdjustment writes revision 1 of a refund or adjustment on a deal.
func (s *ROIStore) AddAdjustment(ctx context.Context, workspaceID, actor, dealID string, in AdjustmentInput) (AdjustmentRevision, error) {
	return s.writeAdjustment(ctx, workspaceID, actor, dealID, "", in, Revision{})
}

// ReviseAdjustment writes the next revision of an adjustment that belongs to
// the deal in the path.
func (s *ROIStore) ReviseAdjustment(ctx context.Context, workspaceID, actor, dealID, adjustmentID string,
	in AdjustmentInput, revision Revision) (AdjustmentRevision, error) {
	if adjustmentID == "" {
		return AdjustmentRevision{}, ErrNotFound
	}
	return s.writeAdjustment(ctx, workspaceID, actor, dealID, adjustmentID, in, revision)
}

func (s *ROIStore) writeAdjustment(ctx context.Context, workspaceID, actor, dealID, adjustmentID string,
	in AdjustmentInput, revision Revision) (AdjustmentRevision, error) {
	if dealID == "" {
		return AdjustmentRevision{}, ErrNotFound
	}
	var written AdjustmentRevision
	create := adjustmentID == ""
	err := s.inTx(ctx, workspaceID, actor, writeStep("adjustment", create, revision.Voided),
		func(ctx context.Context, tx pgx.Tx) (string, error) {
			if err := lockDeal(ctx, tx, workspaceID, dealID); err != nil {
				return adjustmentID, err
			}
			deal, err := latestDeal(ctx, tx, workspaceID, dealID)
			if err != nil {
				return adjustmentID, err
			}
			adjustments, err := currentAdjustments(ctx, tx, workspaceID, dealID)
			if err != nil {
				return adjustmentID, err
			}
			previousRevision := 0
			if !create {
				for _, adjustment := range adjustments {
					if adjustment.AdjustmentID == adjustmentID {
						previousRevision = adjustment.Revision
					}
				}
				// Not on this deal - or not anywhere - reads the same.
				if previousRevision == 0 {
					return adjustmentID, ErrNotFound
				}
			}
			record, err := ValidateAdjustment(in)
			if err != nil {
				return adjustmentID, err
			}
			if record.Currency != deal.Currency {
				return adjustmentID, FieldError{Field: "currency", Reason: "must match the deal's currency"}
			}
			if !revision.Voided {
				if err = checkNet(int64(deal.AmountMinor), deal.Currency, adjustments, adjustmentID, &record); err != nil {
					return adjustmentID, err
				}
			}
			record.Revision = 1
			if !create {
				if err = checkBase(revision, previousRevision); err != nil {
					return adjustmentID, err
				}
				record.Revision = previousRevision + 1
			} else {
				adjustmentID = s.newID()
			}
			record.WorkspaceID, record.AdjustmentID, record.DealID = workspaceID, adjustmentID, dealID
			record.Voided, record.RecordedBy = revision.Voided, actor
			s.beforeInsert(ctx, "adjustment", adjustmentID)
			if err = tx.QueryRow(ctx, `INSERT INTO content_roi_adjustment_revision
				(workspace_id, adjustment_id, revision, voided, deal_id, kind,
				 revenue_delta_minor, gross_delta_minor, currency, occurred_at, note,
				 recorded_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
				RETURNING created_at`,
				workspaceID, adjustmentID, record.Revision, record.Voided, dealID,
				string(record.Kind), int64(record.RevenueDeltaMinor), record.grossDeltaMinor,
				record.Currency, record.OccurredAt, record.Note, actor).
				Scan(&record.CreatedAt); err != nil {
				return adjustmentID, insertError(err)
			}
			record.CreatedAt = record.CreatedAt.UTC()
			written = record
			return adjustmentID, nil
		})
	if err != nil {
		return AdjustmentRevision{}, err
	}
	return written, nil
}
