package feedbacklearning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// Importing pasted costs, leads and deals (specs/034 PR 3; FR-026 to FR-029,
// SC-007, D14-V11).
//
// One import is one transaction, in this order, and the order is the point:
//
//  1. the workspace delete fence (begin), first;
//  2. a fresh import_batch_id, then - when the caller sent an Idempotency-Key
//     and this is not a dry run - the claim on that key, carrying the batch
//     id (contract §1.9). A key already claimed by a committed request with
//     the same input answers with that request's response, rebuilt from its
//     batch row, and writes nothing; with different input it is a 409;
//  3. every row checked - references, then values - and the first bad row
//     refuses the whole import naming its row and field (FR-027). Nothing is
//     written, and the claim is rolled back with everything else, so a retry
//     with the row fixed goes through;
//  4. each row's dedupe key compared with the current records, including the
//     rows this import has already written. A match the row does not confirm
//     in not_duplicate_of holds that row back as 'duplicate'; nothing is
//     merged or dropped without the batch saying so (FR-026);
//  5. the records, the batch row and the audit event, then commit.
//
// The response is built by one function from the batch row as the database
// holds it, both the first time and on every replay. That is what makes a
// replay byte for byte the original.
//
// Idempotency is built here, in this module, on purpose: feedback-learning
// may not import the idempotency module and its dependency list is not to
// change. A guard test fails if that import appears.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §1.8, §1.9, §6

// ImportRecordKind is what one import holds: costs, leads or deals.
type ImportRecordKind string

const (
	ImportCosts ImportRecordKind = "cost"
	ImportLeads ImportRecordKind = "lead"
	ImportDeals ImportRecordKind = "deal"
)

// ImportRecordKinds is contract §1.8's record_kind.
var ImportRecordKinds = []ImportRecordKind{ImportCosts, ImportLeads, ImportDeals}

// ImportOutcome is what happened to one row.
type ImportOutcome string

const (
	// OutcomeWritten: no current record has this row's dedupe key; written.
	OutcomeWritten ImportOutcome = "written"
	// OutcomeDuplicate: the row matched a current record (or an earlier row of
	// this import) and did not confirm it is different; held back.
	OutcomeDuplicate ImportOutcome = "duplicate"
	// OutcomeConfirmedNotDuplicate: the row matched, and named every match in
	// not_duplicate_of; written, and the confirmation audited.
	OutcomeConfirmedNotDuplicate ImportOutcome = "confirmed_not_duplicate"
)

// ImportOutcomes is contract §1.8's rows[].outcome.
var ImportOutcomes = []ImportOutcome{OutcomeWritten, OutcomeDuplicate, OutcomeConfirmedNotDuplicate}

const (
	// MaxImportRows bounds one paste. A month of a brand's records is tens to
	// hundreds of rows (plan.md, Scale).
	MaxImportRows = 1000
	// MaxIdempotencyKeyBytes is the header's limit (FR-028), in bytes.
	MaxIdempotencyKeyBytes = 255
	// IdempotencyKeyField names the header in a refusal.
	IdempotencyKeyField = "Idempotency-Key"
)

// IdempotencyConflict is a key already used for an import with different
// input (409, naming the header).
type IdempotencyConflict struct{}

func (IdempotencyConflict) Error() string { return "idempotency key used with different input" }
func (IdempotencyConflict) Unwrap() error { return ErrConflict }

// ImportInput is one import: the record kind and its rows, in paste order.
// Only the slice for RecordKind is read.
type ImportInput struct {
	RecordKind string
	Costs      []CostInput
	Leads      []LeadInput
	Deals      []DealInput
}

func (in ImportInput) rows() any {
	switch ImportRecordKind(in.RecordKind) {
	case ImportCosts:
		return in.Costs
	case ImportLeads:
		return in.Leads
	default:
		return in.Deals
	}
}

func (in ImportInput) rowCount() int {
	switch ImportRecordKind(in.RecordKind) {
	case ImportCosts:
		return len(in.Costs)
	case ImportLeads:
		return len(in.Leads)
	default:
		return len(in.Deals)
	}
}

// ImportRowResult is one row's outcome. Row is 1-based, counting data rows.
// DuplicateOf lists the current records the row matched - for a held-back row
// what it duplicates, for a confirmed one what the operator said it is not.
// DuplicateOfRows is a dry run's answer for a match against an earlier row of
// the same paste, which has no record id yet; a real import names the record
// that row wrote in DuplicateOf instead.
type ImportRowResult struct {
	Row             int           `json:"row"`
	Outcome         ImportOutcome `json:"outcome"`
	RecordID        string        `json:"record_id"`
	DuplicateOf     []string      `json:"duplicate_of"`
	DuplicateOfRows []int         `json:"duplicate_of_rows,omitempty"`
}

// ImportBatch is one import as stored (contract §1.8).
type ImportBatch struct {
	ImportBatchID  string            `json:"import_batch_id"`
	WorkspaceID    string            `json:"workspace_id"`
	RecordKind     ImportRecordKind  `json:"record_kind"`
	RowCount       int               `json:"row_count"`
	WrittenCount   int               `json:"written_count"`
	SkippedCount   int               `json:"skipped_count"`
	Rows           []ImportRowResult `json:"rows"`
	IdempotencyKey string            `json:"idempotency_key"`
	RecordedBy     string            `json:"recorded_by"`
	CreatedAt      time.Time         `json:"created_at"`
}

// ImportResult is POST /imports' answer. A real import is its batch; a dry
// run has no batch id and no time, because nothing was stored.
type ImportResult struct {
	DryRun        bool              `json:"dry_run"`
	ImportBatchID string            `json:"import_batch_id"`
	RecordKind    ImportRecordKind  `json:"record_kind"`
	RowCount      int               `json:"row_count"`
	WrittenCount  int               `json:"written_count"`
	SkippedCount  int               `json:"skipped_count"`
	Rows          []ImportRowResult `json:"rows"`
	RecordedBy    string            `json:"recorded_by"`
	CreatedAt     *time.Time        `json:"created_at"`
}

// importResponse is the ONLY way a real import's answer is made: from the
// batch row read back from the database. The first request and every replay
// call it on the same row, so they answer alike byte for byte.
func importResponse(batch ImportBatch) ImportResult {
	created := batch.CreatedAt
	return ImportResult{
		ImportBatchID: batch.ImportBatchID, RecordKind: batch.RecordKind,
		RowCount: batch.RowCount, WrittenCount: batch.WrittenCount, SkippedCount: batch.SkippedCount,
		Rows: batch.Rows, RecordedBy: batch.RecordedBy, CreatedAt: &created,
	}
}

// importFingerprint is the SHA-256 of the request's normalized JSON: the
// record kind and the decoded rows, each with its not_duplicate_of. Decoding
// first makes key order and whitespace in what the client sent irrelevant.
func importFingerprint(in ImportInput) (string, error) {
	payload, err := json.Marshal(struct {
		RecordKind string `json:"record_kind"`
		Rows       any    `json:"rows"`
	}{in.RecordKind, in.rows()})
	if err != nil {
		return "", ErrInvalid
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// atRow puts a row number on a field refusal.
func atRow(err error, row int) error {
	if fieldErr, ok := errors.AsType[FieldError](err); ok {
		fieldErr.Row = row
		return fieldErr
	}
	return err
}

// checkImportRequest is everything about an import that can be refused before
// the database is touched: the kind, the key, and how many rows.
func checkImportRequest(in ImportInput, key string) error {
	if err := checkSet("record_kind", in.RecordKind, ImportRecordKinds); err != nil {
		return err
	}
	if len(key) > MaxIdempotencyKeyBytes {
		return FieldError{Field: IdempotencyKeyField, Reason: "longer than 255 bytes"}
	}
	switch count := in.rowCount(); {
	case count == 0:
		return FieldError{Field: "rows", Reason: "invalid or missing"}
	case count > MaxImportRows:
		return FieldError{Field: "rows", Reason: "too many"}
	}
	return nil
}

// Import runs one import (see the top of this file). key is the
// Idempotency-Key header, "" when none was sent. A dry run goes as far as the
// duplicate check and writes nothing - no record, no batch, no claim, no
// audit event.
func (s *ROIStore) Import(ctx context.Context, workspaceID, actor string, in ImportInput, key string, dryRun bool) (ImportResult, error) {
	if !s.ready() {
		return ImportResult{}, ErrStorage
	}
	step := "import-" + in.RecordKind
	if workspaceID == "" || actor == "" {
		s.fail(ctx, workspaceID, actor, "", step, ErrInvalid)
		return ImportResult{}, ErrInvalid
	}
	if err := checkImportRequest(in, key); err != nil {
		s.fail(ctx, workspaceID, actor, "", step, err)
		return ImportResult{}, err
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.fail(ctx, workspaceID, actor, "", step, err)
		return ImportResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	batchID := s.newID()
	result, err := s.importInTx(ctx, tx, workspaceID, actor, in, key, dryRun, batchID)
	if err != nil {
		s.fail(ctx, workspaceID, actor, batchID, step, err)
		return ImportResult{}, err
	}
	if dryRun || result.ImportBatchID != batchID {
		// A dry run, or a replay of an earlier batch: nothing to keep.
		return result, nil
	}
	if err = tx.Commit(ctx); err != nil {
		s.fail(ctx, workspaceID, actor, batchID, step, err)
		return ImportResult{}, ErrStorage
	}
	return result, nil
}

func (s *ROIStore) importInTx(ctx context.Context, tx pgx.Tx, workspaceID, actor string, in ImportInput,
	key string, dryRun bool, batchID string) (ImportResult, error) {
	kind := ImportRecordKind(in.RecordKind)
	if key != "" && !dryRun {
		fingerprint, err := importFingerprint(in)
		if err != nil {
			return ImportResult{}, err
		}
		prior, replay, err := claimImport(ctx, tx, workspaceID, kind, key, fingerprint, batchID)
		if err != nil {
			return ImportResult{}, err
		}
		if replay {
			batch, loadErr := loadImportBatch(ctx, tx, workspaceID, prior)
			if loadErr != nil {
				// The claim names a batch the same transaction wrote; one
				// that is missing is storage trouble, not a caller's error.
				return ImportResult{}, ErrStorage
			}
			return importResponse(batch), nil
		}
		if s.AfterImportClaim != nil {
			s.AfterImportClaim(ctx)
		}
	}

	items, err := s.prepareImport(ctx, tx, workspaceID, actor, in, batchID)
	if err != nil {
		return ImportResult{}, err
	}
	rows, err := s.importItems(ctx, tx, workspaceID, actor, items, dryRun)
	if err != nil {
		return ImportResult{}, err
	}
	written := 0
	for _, row := range rows {
		if row.Outcome != OutcomeDuplicate {
			written++
		}
	}
	if dryRun {
		return ImportResult{
			DryRun: true, RecordKind: kind, RowCount: len(rows), WrittenCount: written,
			SkippedCount: len(rows) - written, Rows: rows, RecordedBy: actor,
		}, nil
	}

	payload, err := json.Marshal(rows)
	if err != nil {
		return ImportResult{}, ErrStorage
	}
	if _, err = tx.Exec(ctx, `INSERT INTO content_roi_import_batch
		(import_batch_id, workspace_id, record_kind, row_count, written_count, skipped_count,
		 rows, idempotency_key, recorded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9)`,
		batchID, workspaceID, string(kind), len(rows), written, len(rows)-written,
		string(payload), key, actor); err != nil {
		return ImportResult{}, ErrStorage
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, batchID, "import-"+string(kind)); err != nil {
		return ImportResult{}, ErrStorage
	}
	batch, err := loadImportBatch(ctx, tx, workspaceID, batchID)
	if err != nil {
		return ImportResult{}, ErrStorage
	}
	return importResponse(batch), nil
}

// claimImport inserts the claim on (workspace, kind, key), or - when a
// committed request holds it - answers which batch that request wrote.
//
// A request holding the same key in a transaction that has not ended makes
// the INSERT wait on the unique index: if that one rolls back, this INSERT
// succeeds; if it commits, this INSERT does nothing, and the SELECT, a new
// statement, sees the committed claim.
func claimImport(ctx context.Context, tx pgx.Tx, workspaceID string, kind ImportRecordKind,
	key, fingerprint, batchID string) (prior string, replay bool, err error) {
	var claimed string
	err = tx.QueryRow(ctx, `INSERT INTO content_roi_import_claim
		(workspace_id, record_kind, idempotency_key, request_fingerprint, import_batch_id)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (workspace_id, record_kind, idempotency_key) DO NOTHING
		RETURNING import_batch_id`,
		workspaceID, string(kind), key, fingerprint, batchID).Scan(&claimed)
	if err == nil {
		return "", false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrStorage
	}
	var stored string
	if err = tx.QueryRow(ctx, `SELECT request_fingerprint, import_batch_id
		FROM content_roi_import_claim
		WHERE workspace_id=$1 AND record_kind=$2 AND idempotency_key=$3`,
		workspaceID, string(kind), key).Scan(&stored, &prior); err != nil {
		return "", false, ErrStorage
	}
	if stored != fingerprint {
		return "", false, IdempotencyConflict{}
	}
	return prior, true, nil
}

// importItem is one checked row, ready for the duplicate check and the write.
type importItem struct {
	dedupeKey     string
	confirmations []string
	duplicates    func(ctx context.Context, tx pgx.Tx) ([]string, error)
	insert        func(ctx context.Context, tx pgx.Tx, recordID string) error
}

// prepareImport is decision steps 2 to 5 over the whole batch: every
// reference of every row first, then every row's values. The first failure
// refuses the import; a field refusal names its row.
func (s *ROIStore) prepareImport(ctx context.Context, tx pgx.Tx, workspaceID, actor string, in ImportInput, batchID string) ([]importItem, error) {
	location := s.location(ctx, workspaceID)
	var items []importItem
	switch ImportRecordKind(in.RecordKind) {
	case ImportCosts:
		for _, row := range in.Costs {
			if err := firstError(
				s.checkAccount(ctx, workspaceID, row.AccountID),
				s.checkWork(ctx, workspaceID, actor, row.WorkID),
			); err != nil {
				return nil, err
			}
		}
		for index, row := range in.Costs {
			if row.Allocations != nil {
				// A split names works and accounts and is checked against
				// them; it is set on the cost afterwards, not in a paste.
				return nil, FieldError{Field: "allocations", Reason: "not accepted in an import", Row: index + 1}
			}
			record, err := ValidateCost(row)
			if err != nil {
				return nil, atRow(err, index+1)
			}
			record.WorkspaceID, record.Revision = workspaceID, 1
			record.SourceType, record.ImportBatchID, record.RecordedBy = RecordImport, batchID, actor
			record.DedupeKey = CostDedupeKey(record, location)
			items = append(items, importItem{
				dedupeKey: record.DedupeKey, confirmations: record.NotDuplicateOf,
				duplicates: func(ctx context.Context, tx pgx.Tx) ([]string, error) {
					return costDuplicates(ctx, tx, workspaceID, record.DedupeKey, "")
				},
				insert: func(ctx context.Context, tx pgx.Tx, recordID string) error {
					written := record
					written.CostID = recordID
					return insertCost(ctx, tx, &written)
				},
			})
		}
	case ImportLeads:
		for index, row := range in.Leads {
			record, err := ValidateLead(row)
			if err != nil {
				return nil, atRow(err, index+1)
			}
			record.WorkspaceID, record.Revision = workspaceID, 1
			record.SourceType, record.ImportBatchID, record.RecordedBy = RecordImport, batchID, actor
			record.DedupeKey = LeadDedupeKey(record.CustomerRef, record.FirstSeenAt, location)
			items = append(items, importItem{
				dedupeKey: record.DedupeKey, confirmations: record.NotDuplicateOf,
				duplicates: func(ctx context.Context, tx pgx.Tx) ([]string, error) {
					return leadDuplicates(ctx, tx, workspaceID, record.DedupeKey, "")
				},
				insert: func(ctx context.Context, tx pgx.Tx, recordID string) error {
					written := record
					written.LeadID = recordID
					return insertLead(ctx, tx, &written)
				},
			})
		}
	case ImportDeals:
		for _, row := range in.Deals {
			if row.LeadID != "" {
				if _, err := latestLead(ctx, tx, workspaceID, row.LeadID); err != nil {
					return nil, err
				}
			}
		}
		for index, row := range in.Deals {
			record, err := ValidateDeal(row)
			if err != nil {
				return nil, atRow(err, index+1)
			}
			record.WorkspaceID, record.Revision = workspaceID, 1
			record.SourceType, record.ImportBatchID, record.RecordedBy = RecordImport, batchID, actor
			record.DedupeKey = DealDedupeKey(record.OrderRef, record.LeadID, record.ClosedAt,
				record.Currency, int64(record.AmountMinor), location)
			items = append(items, importItem{
				dedupeKey: record.DedupeKey, confirmations: record.NotDuplicateOf,
				duplicates: func(ctx context.Context, tx pgx.Tx) ([]string, error) {
					return dealDuplicates(ctx, tx, workspaceID, record.DedupeKey, "")
				},
				insert: func(ctx context.Context, tx pgx.Tx, recordID string) error {
					written := record
					written.DealID = recordID
					return insertDeal(ctx, tx, &written)
				},
			})
		}
	}
	return items, nil
}

// importItems is decision step 7 and the write, row by row in paste order.
// The duplicate query runs in this transaction, so a row sees the records
// the rows before it wrote; a dry run writes nothing and tracks the earlier
// rows itself.
func (s *ROIStore) importItems(ctx context.Context, tx pgx.Tx, workspaceID, actor string, items []importItem, dryRun bool) ([]ImportRowResult, error) {
	results := make([]ImportRowResult, 0, len(items))
	earlier := map[string]int{} // dry run: dedupe key -> first row that would be written
	for index, item := range items {
		result := ImportRowResult{Row: index + 1, DuplicateOf: []string{}}
		if item.dedupeKey != "" {
			matches, err := item.duplicates(ctx, tx)
			if err != nil {
				return nil, err
			}
			result.DuplicateOf = matches
			if row, seen := earlier[item.dedupeKey]; dryRun && seen {
				result.DuplicateOfRows = []int{row}
			}
		}
		unconfirmed := slices.ContainsFunc(result.DuplicateOf, func(match string) bool {
			return !slices.Contains(item.confirmations, match)
		})
		switch {
		case unconfirmed || len(result.DuplicateOfRows) > 0:
			result.Outcome = OutcomeDuplicate
		case len(result.DuplicateOf) > 0:
			result.Outcome = OutcomeConfirmedNotDuplicate
		default:
			result.Outcome = OutcomeWritten
		}
		if result.Outcome != OutcomeDuplicate {
			if dryRun {
				if _, seen := earlier[item.dedupeKey]; !seen && item.dedupeKey != "" {
					earlier[item.dedupeKey] = index + 1
				}
			} else {
				result.RecordID = s.newID()
				if err := item.insert(ctx, tx, result.RecordID); err != nil {
					return nil, err
				}
				if result.Outcome == OutcomeConfirmedNotDuplicate {
					// FR-026: saying "this is not a duplicate" is audited.
					if _, err := s.audit(ctx, tx, workspaceID, actor, result.RecordID, "confirm-not-duplicate"); err != nil {
						return nil, ErrStorage
					}
				}
			}
		}
		results = append(results, result)
	}
	return results, nil
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

const importBatchColumns = `import_batch_id, workspace_id, record_kind, row_count,
	written_count, skipped_count, rows, idempotency_key, recorded_by, created_at`

func scanImportBatch(row scanner) (ImportBatch, error) {
	var batch ImportBatch
	var kind string
	var rows []byte
	if err := row.Scan(&batch.ImportBatchID, &batch.WorkspaceID, &kind, &batch.RowCount,
		&batch.WrittenCount, &batch.SkippedCount, &rows, &batch.IdempotencyKey,
		&batch.RecordedBy, &batch.CreatedAt); err != nil {
		return ImportBatch{}, err
	}
	batch.RecordKind = ImportRecordKind(kind)
	batch.CreatedAt = batch.CreatedAt.UTC()
	if err := json.Unmarshal(rows, &batch.Rows); err != nil {
		return ImportBatch{}, ErrStorage
	}
	if batch.Rows == nil {
		batch.Rows = []ImportRowResult{}
	}
	return batch, nil
}

func loadImportBatch(ctx context.Context, q rowQuerier, workspaceID, batchID string) (ImportBatch, error) {
	batch, err := scanImportBatch(q.QueryRow(ctx, `SELECT `+importBatchColumns+`
		FROM content_roi_import_batch WHERE workspace_id=$1 AND import_batch_id=$2`,
		workspaceID, batchID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportBatch{}, ErrNotFound
	}
	if err != nil {
		return ImportBatch{}, ErrStorage
	}
	return batch, nil
}

// maxListedImports bounds GET /imports: the newest imports are the ones a
// person checks; older ones are still readable by id.
const maxListedImports = 100

// ListImports returns this workspace's imports, newest first.
func (s *ROIStore) ListImports(ctx context.Context, workspaceID, actor string) ([]ImportBatch, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+importBatchColumns+`
		FROM content_roi_import_batch WHERE workspace_id=$1
		ORDER BY created_at DESC, import_batch_id LIMIT $2`, workspaceID, maxListedImports)
	if err != nil {
		s.fail(ctx, workspaceID, actor, "", "list-imports", err)
		return nil, ErrStorage
	}
	return collect(rows, scanImportBatch)
}

// GetImport returns one import with its per-row outcomes.
func (s *ROIStore) GetImport(ctx context.Context, workspaceID, actor, batchID string) (ImportBatch, error) {
	if !s.ready() {
		return ImportBatch{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return ImportBatch{}, ErrInvalid
	}
	return loadImportBatch(ctx, s.DB, workspaceID, batchID)
}
