package feedbacklearning

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Operating diagnosis report versions (specs/035 PR 1: FR-040 to FR-046,
// FR-070 to FR-073, FR-080 to FR-086; SC-004, SC-005, SC-010, SC-012;
// D14-V05 traceability).
//
// Generating a diagnosis writes one new row and changes none. The row holds
// the params as used, a copy of the fields the calculator needed from every
// input with its fingerprint, and the result under its calc version. Nothing
// about a stored version is ever recomputed on read. What IS derived on read
// is whether its inputs have moved on since (FR-043): the inputs generating
// now with the same params would read are compared with the stored ones by
// fingerprint. That flag has no column.
//
// The inputs are read before the write transaction opens; the write
// transaction still takes the workspace delete fence as its first statement,
// then the next version number under a per-report lock, then one INSERT.
// Two generations of the same report at once get consecutive version
// numbers - the lock makes the second wait for the first - and the unique
// index behind it turns any other collision into a conflict rather than a
// second row with the same number.
//
// A later AI judgement layer keys on (report_id, version_no) and may read
// DiagnosisSummary and nothing else (FR-072). Its state here is always
// pending_data (FR-071); there is no table for it.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §1.1, §6,
// §7, §8

// DiagnosisStore is the operating diagnosis half of this module. It shares
// Store's database, fence and audit sink, and adds the read adapters the
// diagnosis needs; each is answered in the handler by the module that owns
// the data.
type DiagnosisStore struct {
	*Store
	Accounts DiagAccounts
	Rules    DiagRules
	Topics   DiagTopics
	Works    DiagWorks
	Delivery DiagDelivery
	// own reads this module's metrics, excerpts and work marks; nil is the
	// database. A test replaces it to hold the whole collection order
	// without one.
	own diagnosisOwnRecords
	// roiSummaries reads a 034 ROI report summary; nil is Store's own read.
	roiSummaries diagnosisROISummaries
}

// DiagnosisReportRequest is the body of POST /reports and POST
// /reports/{reportId}/versions. On a new version, an absent title or absent
// params reuse the previous version's (FR-044).
type DiagnosisReportRequest struct {
	Title  *string          `json:"title"`
	Params *DiagnosisParams `json:"params"`
}

// DiagnosisReportHeader is one version as a list shows it: no params, no
// inputs, no result.
type DiagnosisReportHeader struct {
	ReportID    string         `json:"report_id"`
	VersionNo   int            `json:"version_no"`
	ScopeKind   DiagnosisScope `json:"scope_kind"`
	AccountIDs  []string       `json:"account_ids"`
	Title       string         `json:"title"`
	CalcVersion string         `json:"calc_version"`
	CreatedBy   string         `json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
}

// DiagnosisReportVersion is one stored version as read. Params, Inputs and
// Result are the stored columns, unchanged; InputsChanged is derived on
// read; AIJudgementState is always pending_data.
type DiagnosisReportVersion struct {
	DiagnosisReportHeader
	Params           json.RawMessage        `json:"params"`
	Inputs           json.RawMessage        `json:"inputs"`
	Result           json.RawMessage        `json:"result"`
	InputsChanged    DiagnosisInputsChanged `json:"inputs_changed"`
	AIJudgementState ReviewState            `json:"ai_judgement_state"`
}

// DiagnosisAIJudgementState is the AI judgement state of any diagnosis
// version: pending_data, because the judgement layer is not built and the
// real model runner stays disabled (FR-071).
func DiagnosisAIJudgementState() ReviewState {
	return StatePendingData
}

func (s *DiagnosisStore) ready() bool {
	return s != nil && s.Store != nil && s.DB != nil
}

func (s *DiagnosisStore) roi() *ROIStore {
	var timezones Timezones
	if s.Rules != nil {
		timezones = s.Rules
	}
	return &ROIStore{Store: s.Store, Timezones: timezones}
}

// ownDiagnosisRecords reads this module's metrics and excerpts of the given
// publication records and the marks on the given works, in one snapshot.
// Rows are read as they are; nothing is added up here.
func (s *DiagnosisStore) ownDiagnosisRecords(ctx context.Context, workspaceID string, publicationIDs, workIDs []string) (
	[]ManualMetric, []FeedbackExcerpt, []WorkMark, error) {
	metrics, excerpts, marks := []ManualMetric{}, []FeedbackExcerpt{}, []WorkMark{}
	if !s.ready() {
		return nil, nil, nil, ErrStorage
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, nil, nil, ErrStorage
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY`); err != nil {
		return nil, nil, nil, ErrStorage
	}
	if len(publicationIDs) > 0 {
		rows, queryErr := tx.Query(ctx, metricSelect+` WHERE workspace_id=$1 AND publication_record_id = ANY($2)
			ORDER BY manual_metric_id`, workspaceID, publicationIDs)
		if queryErr != nil {
			return nil, nil, nil, ErrStorage
		}
		if metrics, err = collect(rows, scanMetric); err != nil {
			return nil, nil, nil, err
		}
		if rows, queryErr = tx.Query(ctx, excerptSelect+` WHERE workspace_id=$1 AND publication_record_id = ANY($2)
			ORDER BY feedback_excerpt_id`, workspaceID, publicationIDs); queryErr != nil {
			return nil, nil, nil, ErrStorage
		}
		if excerpts, err = collect(rows, scanExcerpt); err != nil {
			return nil, nil, nil, err
		}
	}
	if len(workIDs) > 0 {
		rows, queryErr := tx.Query(ctx, `SELECT `+workMarkColumns+` FROM content_opdiag_work_mark
			WHERE workspace_id=$1 AND work_id = ANY($2) ORDER BY created_at, mark_id`, workspaceID, workIDs)
		if queryErr != nil {
			return nil, nil, nil, ErrStorage
		}
		if marks, err = collect(rows, scanWorkMark); err != nil {
			return nil, nil, nil, err
		}
	}
	return metrics, excerpts, marks, nil
}

// ---------------------------------------------------------------- generation

// CreateDiagnosisReport generates version 1 of a new report.
func (s *DiagnosisStore) CreateDiagnosisReport(ctx context.Context, workspaceID, actor string, in DiagnosisReportRequest, now time.Time) (DiagnosisReportVersion, error) {
	return s.generateDiagnosis(ctx, workspaceID, actor, "", in, now)
}

// GenerateDiagnosisReportVersion generates the next version of an existing
// report from the inputs as they are now. Earlier versions stay as they
// were.
func (s *DiagnosisStore) GenerateDiagnosisReportVersion(ctx context.Context, workspaceID, actor, reportID string, in DiagnosisReportRequest, now time.Time) (DiagnosisReportVersion, error) {
	if reportID == "" {
		return DiagnosisReportVersion{}, ErrNotFound
	}
	return s.generateDiagnosis(ctx, workspaceID, actor, reportID, in, now)
}

func (s *DiagnosisStore) generateDiagnosis(ctx context.Context, workspaceID, actor, reportID string, in DiagnosisReportRequest, now time.Time) (DiagnosisReportVersion, error) {
	step := "generate-diagnosis"
	if reportID != "" {
		step = "generate-diagnosis-version"
	}
	if !s.ready() {
		return DiagnosisReportVersion{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return DiagnosisReportVersion{}, ErrInvalid
	}

	// Decision step 2: the report in the path exists here. Its latest
	// version supplies the defaults.
	title, params := "", DiagnosisParams{}
	if reportID != "" {
		previous, err := s.readDiagnosisVersion(ctx, workspaceID, reportID, 0)
		if err != nil {
			return DiagnosisReportVersion{}, err
		}
		title = previous.Title
		if err = json.Unmarshal(previous.Params, &params); err != nil {
			return DiagnosisReportVersion{}, ErrStorage
		}
	}
	if in.Params != nil {
		params = *in.Params
	} else if reportID == "" {
		return DiagnosisReportVersion{}, FieldError{Field: "params", Reason: "required"}
	}
	if in.Title != nil {
		title = *in.Title
	}
	// What a person does not choose: the brand's timezone and the time of
	// generation.
	params.Window.Timezone = s.roi().location(ctx, workspaceID).String()
	params.GeneratedAt = now.UTC().Format(time.RFC3339)

	// Steps 2 to 6 of the params, then every input, then the result
	// computed from exactly the copy that is stored.
	prepared, inputs, err := s.gatherDiagnosisInputs(ctx, workspaceID, actor, params, false)
	if err != nil {
		return DiagnosisReportVersion{}, err
	}
	if err = checkRuneLimit("title", title, MaxDiagnosisTitleRunes); err != nil {
		return DiagnosisReportVersion{}, err
	}
	result, err := CalculateDiagnosis(params, inputs)
	if err != nil {
		return DiagnosisReportVersion{}, err
	}
	encodedParams, paramsErr := json.Marshal(params)
	encodedInputs, inputsErr := json.Marshal(inputs)
	encodedResult, resultErr := json.Marshal(result)
	if paramsErr != nil || inputsErr != nil || resultErr != nil {
		return DiagnosisReportVersion{}, ErrStorage
	}

	newReport := reportID == ""
	if newReport {
		reportID = s.newID()
	}
	written := DiagnosisReportVersion{
		DiagnosisReportHeader: DiagnosisReportHeader{
			ReportID: reportID, ScopeKind: params.Scope.Kind, AccountIDs: prepared.accountIDs,
			Title: title, CalcVersion: result.CalcVersion, CreatedBy: actor,
		},
		Params: encodedParams, Inputs: encodedInputs, Result: encodedResult,
		InputsChanged: noDiagnosisChanges(), AIJudgementState: DiagnosisAIJudgementState(),
	}
	// The write: the workspace delete fence first (inTx), then the next
	// version number under a per-report lock, then one INSERT - never a
	// change to an earlier version.
	err = s.roi().inTx(ctx, workspaceID, actor, step, func(ctx context.Context, tx pgx.Tx) (string, error) {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			"content-opdiag-report:"+workspaceID+":"+reportID); err != nil {
			return reportID, ErrStorage
		}
		latest := 0
		err := tx.QueryRow(ctx, `SELECT version_no FROM content_opdiag_report_version
			WHERE workspace_id=$1 AND report_id=$2 ORDER BY version_no DESC LIMIT 1`,
			workspaceID, reportID).Scan(&latest)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if !newReport {
				return reportID, ErrNotFound
			}
		case err != nil:
			return reportID, ErrStorage
		}
		written.VersionNo = latest + 1
		if err = tx.QueryRow(ctx, `INSERT INTO content_opdiag_report_version
			(workspace_id, report_id, version_no, scope_kind, account_ids, title, params, inputs,
			 calc_version, result, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10::jsonb, $11) RETURNING created_at`,
			workspaceID, reportID, written.VersionNo, string(params.Scope.Kind), prepared.accountIDs, title,
			string(encodedParams), string(encodedInputs), result.CalcVersion, string(encodedResult),
			actor).Scan(&written.CreatedAt); err != nil {
			if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
				return reportID, RevisionConflict{Field: "version_no"}
			}
			return reportID, ErrStorage
		}
		written.CreatedAt = written.CreatedAt.UTC()
		return reportID, nil
	})
	if err != nil {
		return DiagnosisReportVersion{}, err
	}
	return written, nil
}

// ---------------------------------------------------------------- preview

// PreviewDiagnosis computes a diagnosis from the given params and the
// inputs as they are now, and stores nothing (POST /preview, T051): no row,
// no audit entry, no fence. It reads in the same order as generating does,
// so a foreign account is refused before anything about any account is
// read, and it answers exactly what generating would store as the result.
func (s *DiagnosisStore) PreviewDiagnosis(ctx context.Context, workspaceID, actor string, params *DiagnosisParams, now time.Time) (DiagnosisResult, error) {
	if s == nil || s.Store == nil {
		return DiagnosisResult{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return DiagnosisResult{}, ErrInvalid
	}
	if params == nil {
		return DiagnosisResult{}, FieldError{Field: "params", Reason: "required"}
	}
	used := *params
	used.Window.Timezone = s.roi().location(ctx, workspaceID).String()
	used.GeneratedAt = now.UTC().Format(time.RFC3339)
	_, inputs, err := s.gatherDiagnosisInputs(ctx, workspaceID, actor, used, false)
	if err != nil {
		return DiagnosisResult{}, err
	}
	return CalculateDiagnosis(used, inputs)
}

// ---------------------------------------------------------------- reading

const diagnosisHeaderColumns = `report_id, version_no, scope_kind, account_ids, title, calc_version, created_by, created_at`

func scanDiagnosisHeader(row scanner, extra ...any) (DiagnosisReportHeader, error) {
	var header DiagnosisReportHeader
	var scopeKind string
	targets := append([]any{&header.ReportID, &header.VersionNo, &scopeKind, &header.AccountIDs,
		&header.Title, &header.CalcVersion, &header.CreatedBy, &header.CreatedAt}, extra...)
	if err := row.Scan(targets...); err != nil {
		return DiagnosisReportHeader{}, err
	}
	header.ScopeKind = DiagnosisScope(scopeKind)
	header.AccountIDs = nonNil(header.AccountIDs)
	header.CreatedAt = header.CreatedAt.UTC()
	return header, nil
}

// readDiagnosisVersion reads one stored version; versionNo 0 is the latest.
func (s *DiagnosisStore) readDiagnosisVersion(ctx context.Context, workspaceID, reportID string, versionNo int) (DiagnosisReportVersion, error) {
	var params, inputs, result string
	query := `SELECT ` + diagnosisHeaderColumns + `, params::text, inputs::text, result::text
		FROM content_opdiag_report_version WHERE workspace_id=$1 AND report_id=$2`
	args := []any{workspaceID, reportID}
	if versionNo > 0 {
		query += ` AND version_no=$3`
		args = append(args, versionNo)
	} else {
		query += ` ORDER BY version_no DESC LIMIT 1`
	}
	header, err := scanDiagnosisHeader(s.DB.QueryRow(ctx, query, args...), &params, &inputs, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return DiagnosisReportVersion{}, ErrNotFound
	}
	if err != nil {
		return DiagnosisReportVersion{}, ErrStorage
	}
	return DiagnosisReportVersion{
		DiagnosisReportHeader: header,
		Params:                json.RawMessage(params),
		Inputs:                json.RawMessage(inputs),
		Result:                json.RawMessage(result),
		InputsChanged:         noDiagnosisChanges(),
		AIJudgementState:      DiagnosisAIJudgementState(),
	}, nil
}

// ListDiagnosisReports answers the latest version of every report, newest
// first.
func (s *DiagnosisStore) ListDiagnosisReports(ctx context.Context, workspaceID, actor string) ([]DiagnosisReportHeader, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT ON (report_id) `+diagnosisHeaderColumns+`
		FROM content_opdiag_report_version WHERE workspace_id=$1
		ORDER BY report_id, version_no DESC`, workspaceID)
	if err != nil {
		return nil, ErrStorage
	}
	headers, err := collect(rows, func(row scanner) (DiagnosisReportHeader, error) { return scanDiagnosisHeader(row) })
	if err != nil {
		return nil, err
	}
	slices.SortFunc(headers, func(left, right DiagnosisReportHeader) int {
		return cmp.Or(right.CreatedAt.Compare(left.CreatedAt), cmp.Compare(left.ReportID, right.ReportID))
	})
	return headers, nil
}

// ListDiagnosisReportVersions answers every version of one report, oldest
// first.
func (s *DiagnosisStore) ListDiagnosisReportVersions(ctx context.Context, workspaceID, actor, reportID string) ([]DiagnosisReportHeader, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+diagnosisHeaderColumns+`
		FROM content_opdiag_report_version WHERE workspace_id=$1 AND report_id=$2
		ORDER BY version_no`, workspaceID, reportID)
	if err != nil {
		return nil, ErrStorage
	}
	headers, err := collect(rows, func(row scanner) (DiagnosisReportHeader, error) { return scanDiagnosisHeader(row) })
	if err != nil {
		return nil, err
	}
	if len(headers) == 0 {
		return nil, ErrNotFound
	}
	return headers, nil
}

// GetDiagnosisReportVersion answers one stored version as it was stored,
// with inputs_changed derived now (FR-043): the inputs generating with the
// same params would read today, against the stored ones.
func (s *DiagnosisStore) GetDiagnosisReportVersion(ctx context.Context, workspaceID, actor, reportID string, versionNo int, now time.Time) (DiagnosisReportVersion, error) {
	if !s.ready() {
		return DiagnosisReportVersion{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return DiagnosisReportVersion{}, ErrInvalid
	}
	if versionNo < 1 {
		return DiagnosisReportVersion{}, ErrNotFound
	}
	version, err := s.readDiagnosisVersion(ctx, workspaceID, reportID, versionNo)
	if err != nil {
		return DiagnosisReportVersion{}, err
	}
	var params DiagnosisParams
	var stored []DiagnosisInput
	if json.Unmarshal(version.Params, &params) != nil || json.Unmarshal(version.Inputs, &stored) != nil {
		return DiagnosisReportVersion{}, ErrStorage
	}
	params.GeneratedAt = now.UTC().Format(time.RFC3339)
	_, current, err := s.gatherDiagnosisInputs(ctx, workspaceID, actor, params, true)
	if errors.Is(err, ErrNotFound) {
		// The workspace itself is gone: answered like any missing record.
		return DiagnosisReportVersion{}, ErrNotFound
	}
	if err != nil {
		return DiagnosisReportVersion{}, ErrStorage
	}
	version.InputsChanged = diffDiagnosisInputs(stored, current)
	return version, nil
}

// ---------------------------------------------------------------- public summary

// DiagnosisSummaryFields is contract §8's allow-list: every JSON field name
// that may appear anywhere in a DiagnosisSummary, nested ones included, up
// to roi_reference (the ROI summary, held to its own list) and facts (a
// dimension's facts, PR 2). A reflection test holds DiagnosisSummary to it;
// widening it needs the controller's approval.
var DiagnosisSummaryFields = []string{
	"report_id", "version_no", "calc_version", "data_origin", "created_at",
	"scope", "accounts", "account_id", "platform", "display_name", "profile_revision_id",
	"window", "start", "end", "timezone", "comparison_window",
	"input_counts", "publications", "works", "metrics", "excerpts", "work_marks", "reviews", "delivery_tasks",
	"sections", "section", "dimensions", "status", "reason", "facts",
	"completeness", "expected", "present", "gap_keys", "limits",
	"gaps", "gap_key", "kind", "dimension", "ref", "id",
	"rules", "roi_reference",
}

// DiagnosisSummary is what a later reader - the AI judgement layer, or the
// today dashboard - may see of one diagnosis report version (contract §8,
// FR-046, FR-072). It has no inputs, no text a person wrote, no creator or
// recorder, no excerpt, no profile item's text and no customer or order
// reference.
type DiagnosisSummary struct {
	ReportID     string                    `json:"report_id"`
	VersionNo    int                       `json:"version_no"`
	CalcVersion  string                    `json:"calc_version"`
	DataOrigin   DataOrigin                `json:"data_origin"`
	Scope        DiagnosisSummaryScope     `json:"scope"`
	Sections     []DiagnosisSummarySection `json:"sections"`
	Gaps         []DiagnosisSummaryGap     `json:"gaps"`
	Rules        []string                  `json:"rules"`
	ROIReference *ReportSummary            `json:"roi_reference"`
	CreatedAt    time.Time                 `json:"created_at"`
}

// DiagnosisSummaryScope is the scope's accounts, windows and input counts.
type DiagnosisSummaryScope struct {
	Accounts         []DiagnosisScopeAccount `json:"accounts"`
	Window           DiagnosisWindow         `json:"window"`
	ComparisonWindow *DiagnosisDateRange     `json:"comparison_window"`
	InputCounts      DiagnosisInputCounts    `json:"input_counts"`
}

// DiagnosisSummarySection is one section's dimensions without their record
// lists.
type DiagnosisSummarySection struct {
	Section    string                                           `json:"section"`
	AccountID  string                                           `json:"account_id"`
	Dimensions map[DiagnosisDimension]DiagnosisSummaryDimension `json:"dimensions"`
}

// DiagnosisSummaryDimension is status, reason, facts, completeness and
// limits.
type DiagnosisSummaryDimension struct {
	Status       string                `json:"status"`
	Reason       string                `json:"reason"`
	Facts        json.RawMessage       `json:"facts"`
	Completeness DimensionCompleteness `json:"completeness"`
	Limits       []string              `json:"limits"`
}

// DiagnosisSummaryGap is a gap without the page route.
type DiagnosisSummaryGap struct {
	GapKey    string             `json:"gap_key"`
	Kind      GapKind            `json:"kind"`
	Dimension string             `json:"dimension"`
	Ref       DiagnosisRecordRef `json:"ref"`
	AccountID string             `json:"account_id"`
}

// summarizeDiagnosis copies only the allowed fields of a stored result.
func summarizeDiagnosis(header DiagnosisReportHeader, result DiagnosisResult) DiagnosisSummary {
	summary := DiagnosisSummary{
		ReportID: header.ReportID, VersionNo: header.VersionNo, CalcVersion: header.CalcVersion,
		DataOrigin: result.DataOrigin, CreatedAt: header.CreatedAt,
		Scope: DiagnosisSummaryScope{
			Accounts: nonNil(result.Scope.Accounts), Window: result.Scope.Window,
			ComparisonWindow: result.Scope.ComparisonWindow, InputCounts: result.Scope.InputCounts,
		},
		Sections:     []DiagnosisSummarySection{},
		Gaps:         []DiagnosisSummaryGap{},
		Rules:        nonNil(result.Rules),
		ROIReference: result.ROIReference,
	}
	for _, section := range result.Sections {
		copied := DiagnosisSummarySection{
			Section: section.Section, AccountID: section.AccountID,
			Dimensions: map[DiagnosisDimension]DiagnosisSummaryDimension{},
		}
		for key, dimension := range section.Dimensions {
			copied.Dimensions[key] = DiagnosisSummaryDimension{
				Status: dimension.Status, Reason: dimension.Reason, Facts: dimension.Facts,
				Completeness: dimension.Completeness, Limits: nonNil(dimension.Limits),
			}
		}
		summary.Sections = append(summary.Sections, copied)
	}
	for _, gap := range result.Gaps {
		summary.Gaps = append(summary.Gaps, DiagnosisSummaryGap{
			GapKey: gap.GapKey, Kind: gap.Kind, Dimension: gap.Dimension, Ref: gap.Ref, AccountID: gap.AccountID,
		})
	}
	return summary
}

// DiagnosisSummary answers the public summary of one stored version
// (contract §8). It reads the stored result; it recomputes nothing.
func (s *Store) DiagnosisSummary(ctx context.Context, workspaceID, reportID string, versionNo int) (DiagnosisSummary, error) {
	if s == nil || s.DB == nil {
		return DiagnosisSummary{}, ErrStorage
	}
	if workspaceID == "" || reportID == "" || versionNo < 1 {
		return DiagnosisSummary{}, ErrNotFound
	}
	store := &DiagnosisStore{Store: s}
	version, err := store.readDiagnosisVersion(ctx, workspaceID, reportID, versionNo)
	if err != nil {
		return DiagnosisSummary{}, err
	}
	var result DiagnosisResult
	if json.Unmarshal(version.Result, &result) != nil {
		return DiagnosisSummary{}, ErrStorage
	}
	return summarizeDiagnosis(version.DiagnosisReportHeader, result), nil
}
