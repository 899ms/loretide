package feedbacklearning

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Report versions (specs/034 PR 4: FR-041, FR-053 to FR-058, FR-060 to
// FR-063; SC-009 to SC-011; D14-V13 traceability, D14-V14 "更新记录不覆盖旧报告
// 输入").
//
// Generating a report writes one new row and changes none. The row holds
// three things kept apart, as R-061 asks: the parameters as used, the inputs
// (which record revisions were read, and a full copy of each), and the
// computed result under its calc version. Raw observations stay in the record
// tables; an AI inference would be a third store, and this card builds none.
//
// Nothing about a stored version is ever recomputed on read. What IS derived
// on read is whether the records it used have moved on since (FR-055): its
// included set is compared with the included set generating now with the
// same params would have. That flag has no column.
//
// The records are read in a repeatable-read snapshot before the write
// transaction opens; the write transaction still takes the workspace delete
// fence as its first statement. A record written between the two is not
// lost: the next read of the version reports it through inputs_changed.
//
// A later AI explanation layer keys on (report_id, version_no) and may read
// ReportSummary and nothing else (FR-062). Its state here is always
// pending_data (FR-061); there is no table for it and no path from a report
// into topics, budget todos or business memory (FR-063).
//
// Contract: specs/034-roi-review/contracts/roi-review.md §1.10, §6, §7

// MaxReportTitleRunes bounds a report's title (contract §1.10).
const MaxReportTitleRunes = 200

// ErrCalcVersionUnavailable is a stored version whose calc version this build
// does not carry. Such a version is shown as stored; it is never recomputed
// with the current formulas instead (FR-041).
var ErrCalcVersionUnavailable = errors.New("roi calc version not available")

// calculators are the calc versions this build can recompute. A formula or
// rounding change adds "roi-calc/2" beside the first; it does not replace it.
var calculators = map[string]func(ReportInput) (Result, error){
	CalcVersion: CalculateROI,
}

// ReportRecordKey is one record revision a report read.
type ReportRecordKey struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}

// ReportInputs is the stored inputs column. Records is the included set:
// the (kind, id, revision) of every record the calculator actually used
// (contract §1.10). The copies are every revision that was read, used or
// not, so a stored result can be recomputed without the record tables
// (FR-057).
type ReportInputs struct {
	Records      []ReportRecordKey     `json:"records"`
	Costs        []CostRevision        `json:"costs"`
	Leads        []LeadRevision        `json:"leads"`
	Touches      []TouchRevision       `json:"touches"`
	Deals        []DealRevision        `json:"deals"`
	Adjustments  []AdjustmentRevision  `json:"adjustments"`
	Attributions []AttributionRevision `json:"attributions"`
}

// ReportRequest is the body of POST /reports and POST
// /reports/{reportId}/versions. On a new version, an absent title or absent
// params reuse the previous version's (FR-056).
type ReportRequest struct {
	Title  *string       `json:"title"`
	Params *ReportParams `json:"params"`
}

// ReportVersionHeader is one version as a list shows it.
type ReportVersionHeader struct {
	ReportID    string    `json:"report_id"`
	VersionNo   int       `json:"version_no"`
	Title       string    `json:"title"`
	CalcVersion string    `json:"calc_version"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// ChangedInput is one record that differs between a version's included set
// and the included set generating now with the same params would have.
// ReportRevision nil: the version did not use it (it is new, or has newly
// entered the window or scope). CurrentRevision nil: a new version would no
// longer use it (voided, merged away, or no longer in the window or scope).
type ChangedInput struct {
	Kind            string `json:"kind"`
	ID              string `json:"id"`
	ReportRevision  *int   `json:"report_revision"`
	CurrentRevision *int   `json:"current_revision"`
}

// ReportVersion is one stored version as read. Params, Inputs and Result are
// the stored columns, unchanged. InputsChanged and ChangedInputs are derived
// on read; AIExplanation is always pending_data.
type ReportVersion struct {
	ReportVersionHeader
	Params        json.RawMessage `json:"params"`
	Inputs        json.RawMessage `json:"inputs"`
	Result        json.RawMessage `json:"result"`
	InputsChanged bool            `json:"inputs_changed"`
	ChangedInputs []ChangedInput  `json:"changed_inputs"`
	AIExplanation ReviewState     `json:"ai_explanation"`
}

// reportVersionRow is what one generation writes.
type reportVersionRow struct {
	params      []byte
	inputs      []byte
	calcVersion string
	result      []byte
}

// ReportAIExplanation is the AI explanation state of any report version:
// pending_data, because the explanation layer is not built and the real
// model runner stays disabled (FR-060, FR-061).
func ReportAIExplanation() ReviewState {
	return StatePendingData
}

// ---------------------------------------------------------------- pure part

// includedRecords is a report's included set: the latest revision of every
// record the calculator uses for these params, by the calculator's own
// window, scope and merge rules (the same costLines, dealLines and lead roots
// CalculateROI walks). A record outside the window or scope is not in it, so
// revising one cannot flag a version as out of date; a record that enters the
// window or scope is, so it does.
//
//   - cost: every cost with a share (or its whole amount) in the window and
//     scope;
//   - deal: every deal closed in the window (deals are brand level; scope
//     does not narrow them), with its refunds/adjustments up to the
//     generation time, its latest judgement, and the touches that judgement
//     accepted - all of them, since out-of-scope touches still decide how the
//     deal is split;
//   - lead: every lead that counts at brand level and was first seen in the
//     window, and every lead merged into one, at its latest revision (a
//     lead's stages come from all its revisions, so any new revision changes
//     it).
func includedRecords(input ReportInput) ([]ReportRecordKey, error) {
	p, err := prepareReportParams(input.Params)
	if err != nil {
		return nil, err
	}
	keys := []ReportRecordKey{}
	for _, line := range p.costLines(input.Costs) {
		keys = append(keys, ReportRecordKey{"cost", line.record.ID, line.record.Revision})
	}
	judgements := map[string]AttributionRevision{}
	for _, judgement := range latestByID(input.Attributions, func(a AttributionRevision) string { return a.DealID },
		func(a AttributionRevision) int { return a.Revision }) {
		judgements[judgement.DealID] = judgement
	}
	for _, line := range p.dealLines(input) {
		keys = append(keys, ReportRecordKey{"deal", line.deal.DealID, line.deal.Revision})
		for _, adjustment := range line.adjustments {
			keys = append(keys, ReportRecordKey{"adjustment", adjustment.ID, adjustment.Revision})
		}
		for _, touch := range line.touches {
			keys = append(keys, ReportRecordKey{"touch", touch.TouchID, touch.Revision})
		}
		if judgement, ok := judgements[line.deal.DealID]; ok {
			keys = append(keys, ReportRecordKey{"attribution", judgement.DealID, judgement.Revision})
		}
	}
	counted, _ := leadStages(input.Leads)
	latest := map[string]LeadRevision{}
	for _, lead := range latestByID(input.Leads, func(l LeadRevision) string { return l.LeadID },
		func(l LeadRevision) int { return l.Revision }) {
		latest[lead.LeadID] = lead
	}
	root := func(id string) string {
		for range maxMergeChain {
			lead, ok := latest[id]
			if !ok || lead.MergedInto == "" {
				return id
			}
			id = lead.MergedInto
		}
		return id
	}
	for id, lead := range latest {
		if owner, ok := counted[root(id)]; ok && p.inWindow(owner.FirstSeenAt) {
			keys = append(keys, ReportRecordKey{"lead", id, lead.Revision})
		}
	}
	slices.SortFunc(keys, func(left, right ReportRecordKey) int {
		return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID),
			cmp.Compare(left.Revision, right.Revision))
	})
	return slices.Compact(keys), nil
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// buildReportVersion computes a report and serializes the three stored
// columns. The result is computed from exactly the input that is stored, so
// RecomputeReport over the stored columns gives it back (FR-057).
func buildReportVersion(input ReportInput) (reportVersionRow, error) {
	result, err := CalculateROI(input)
	if err != nil {
		return reportVersionRow{}, err
	}
	included, err := includedRecords(input)
	if err != nil {
		return reportVersionRow{}, err
	}
	inputs := ReportInputs{
		Records:      included,
		Costs:        nonNil(input.Costs),
		Leads:        nonNil(input.Leads),
		Touches:      nonNil(input.Touches),
		Deals:        nonNil(input.Deals),
		Adjustments:  nonNil(input.Adjustments),
		Attributions: nonNil(input.Attributions),
	}
	row := reportVersionRow{calcVersion: result.CalcVersion}
	if row.params, err = json.Marshal(input.Params); err != nil {
		return reportVersionRow{}, ErrStorage
	}
	if row.inputs, err = json.Marshal(inputs); err != nil {
		return reportVersionRow{}, ErrStorage
	}
	if row.result, err = json.Marshal(result); err != nil {
		return reportVersionRow{}, ErrStorage
	}
	return row, nil
}

// RecomputeReport recomputes a stored version from its stored params and
// inputs with its stored calc version - never the current one. It is the
// FR-057 check, not something a page calls: a stored version is shown as
// stored.
func RecomputeReport(calcVersion string, params, inputs []byte) (Result, error) {
	calculate, ok := calculators[calcVersion]
	if !ok {
		return Result{}, ErrCalcVersionUnavailable
	}
	var decodedParams ReportParams
	if err := json.Unmarshal(params, &decodedParams); err != nil {
		return Result{}, ErrStorage
	}
	var decoded ReportInputs
	if err := json.Unmarshal(inputs, &decoded); err != nil {
		return Result{}, ErrStorage
	}
	return calculate(ReportInput{
		Params: decodedParams, Costs: decoded.Costs, Leads: decoded.Leads, Touches: decoded.Touches,
		Deals: decoded.Deals, Adjustments: decoded.Adjustments, Attributions: decoded.Attributions,
	})
}

// CanonicalJSON rewrites a JSON document with sorted keys and no spacing, so
// a document read back from a jsonb column compares byte for byte with the
// one that was written. Numbers are kept as written.
func CanonicalJSON(document []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func latestRevisions(keys []ReportRecordKey) map[ReportRecordKey]int {
	latest := map[ReportRecordKey]int{}
	for _, key := range keys {
		id := ReportRecordKey{Kind: key.Kind, ID: key.ID}
		latest[id] = max(latest[id], key.Revision)
	}
	return latest
}

// diffReportInputs lists every record whose revision differs between a
// version's included set and the included set generating now would have,
// sorted by kind and id.
func diffReportInputs(stored, current []ReportRecordKey) []ChangedInput {
	before, after := latestRevisions(stored), latestRevisions(current)
	changed := []ChangedInput{}
	for id, revision := range before {
		if now, ok := after[id]; !ok {
			changed = append(changed, ChangedInput{Kind: id.Kind, ID: id.ID, ReportRevision: &revision})
		} else if now != revision {
			changed = append(changed, ChangedInput{Kind: id.Kind, ID: id.ID, ReportRevision: &revision, CurrentRevision: &now})
		}
	}
	for id, revision := range after {
		if _, ok := before[id]; !ok {
			changed = append(changed, ChangedInput{Kind: id.Kind, ID: id.ID, CurrentRevision: &revision})
		}
	}
	slices.SortFunc(changed, func(left, right ChangedInput) int {
		return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID))
	})
	return changed
}

// ParseVersionNo reads a {versionNo} path segment. Anything that is not a
// positive decimal integer is answered like a version that does not exist.
func ParseVersionNo(text string) (int, bool) {
	if text == "" || text[0] == '+' || text[0] == '-' {
		return 0, false
	}
	number, err := strconv.Atoi(text)
	if err != nil || number < 1 {
		return 0, false
	}
	return number, true
}

// ---------------------------------------------------------------- generation

// CreateReport generates version 1 of a new report from the given params.
func (s *ROIStore) CreateReport(ctx context.Context, workspaceID, actor string, in ReportRequest, now time.Time) (ReportVersion, error) {
	return s.generateReport(ctx, workspaceID, actor, "", in, now)
}

// GenerateReportVersion generates the next version of an existing report
// from the records as they are now. The previous versions stay as they were.
func (s *ROIStore) GenerateReportVersion(ctx context.Context, workspaceID, actor, reportID string, in ReportRequest, now time.Time) (ReportVersion, error) {
	if reportID == "" {
		return ReportVersion{}, ErrNotFound
	}
	return s.generateReport(ctx, workspaceID, actor, reportID, in, now)
}

// stampParams writes what a person must not choose: the brand's timezone and
// the generation time, and who entered each rate given in this request and
// when. Rates carried over from the previous version keep their own stamps.
func (s *ROIStore) stampParams(ctx context.Context, workspaceID, actor string, params ReportParams, fromRequest bool, now time.Time) ReportParams {
	params.Window.Timezone = s.location(ctx, workspaceID).String()
	params.GeneratedAt = now.UTC().Format(time.RFC3339)
	params.Rates = slices.Clone(params.Rates)
	if fromRequest {
		for i := range params.Rates {
			params.Rates[i].EnteredBy = actor
			params.Rates[i].EnteredAt = params.GeneratedAt
		}
	}
	return params
}

func (s *ROIStore) generateReport(ctx context.Context, workspaceID, actor, reportID string, in ReportRequest, now time.Time) (ReportVersion, error) {
	step := "generate-report"
	if reportID != "" {
		step = "generate-report-version"
	}
	if !s.ready() {
		return ReportVersion{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return ReportVersion{}, ErrInvalid
	}

	// Decision step 2: the report in the path exists here. Its latest
	// version supplies the defaults.
	title, params, fromRequest := "", ReportParams{}, true
	if reportID != "" {
		previous, err := s.readReportVersion(ctx, workspaceID, reportID, 0)
		if err != nil {
			return ReportVersion{}, err
		}
		title = previous.Title
		if err = json.Unmarshal(previous.Params, &params); err != nil {
			return ReportVersion{}, ErrStorage
		}
		fromRequest = false
	}
	if in.Title != nil {
		title = *in.Title
	}
	if in.Params != nil {
		params, fromRequest = *in.Params, true
	} else if reportID == "" {
		return ReportVersion{}, FieldError{Field: "params", Reason: "required"}
	}
	if err := checkRunes("title", title, MaxReportTitleRunes); err != nil {
		return ReportVersion{}, err
	}
	params = s.stampParams(ctx, workspaceID, actor, params, fromRequest, now)

	// The records, read in one repeatable-read snapshot, and the result
	// computed from exactly the copy that is stored.
	input, err := s.LoadReportInput(ctx, workspaceID, actor, params)
	if err != nil {
		return ReportVersion{}, err
	}
	row, err := buildReportVersion(input)
	if err != nil {
		return ReportVersion{}, err
	}

	newReport := reportID == ""
	if newReport {
		reportID = s.newID()
	}
	written := ReportVersion{
		ReportVersionHeader: ReportVersionHeader{
			ReportID: reportID, Title: title, CalcVersion: row.calcVersion, CreatedBy: actor,
		},
		Params: row.params, Inputs: row.inputs, Result: row.result,
		ChangedInputs: []ChangedInput{}, AIExplanation: ReportAIExplanation(),
	}
	// The write: the workspace delete fence first (inTx), then the next
	// version number under a per-report lock, then one INSERT - never a
	// change to an earlier version.
	err = s.inTx(ctx, workspaceID, actor, step, func(ctx context.Context, tx pgx.Tx) (string, error) {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			"content-roi-report:"+workspaceID+":"+reportID); err != nil {
			return reportID, ErrStorage
		}
		latest := 0
		err := tx.QueryRow(ctx, `SELECT version_no FROM content_roi_report_version
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
		if err = tx.QueryRow(ctx, `INSERT INTO content_roi_report_version
			(workspace_id, report_id, version_no, title, params, inputs, calc_version, result, created_by)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8::jsonb, $9) RETURNING created_at`,
			workspaceID, reportID, written.VersionNo, title, string(row.params), string(row.inputs),
			row.calcVersion, string(row.result), actor).Scan(&written.CreatedAt); err != nil {
			if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
				return reportID, RevisionConflict{Field: "version_no"}
			}
			return reportID, ErrStorage
		}
		written.CreatedAt = written.CreatedAt.UTC()
		return reportID, nil
	})
	if err != nil {
		return ReportVersion{}, err
	}
	return written, nil
}

// ---------------------------------------------------------------- reading

const reportHeaderColumns = `report_id, version_no, title, calc_version, created_by, created_at`

func scanReportHeader(row scanner, extra ...any) (ReportVersionHeader, error) {
	var header ReportVersionHeader
	targets := append([]any{&header.ReportID, &header.VersionNo, &header.Title, &header.CalcVersion,
		&header.CreatedBy, &header.CreatedAt}, extra...)
	if err := row.Scan(targets...); err != nil {
		return ReportVersionHeader{}, err
	}
	header.CreatedAt = header.CreatedAt.UTC()
	return header, nil
}

// readReportVersion reads one stored version; versionNo 0 is the latest.
func (s *ROIStore) readReportVersion(ctx context.Context, workspaceID, reportID string, versionNo int) (ReportVersion, error) {
	var params, inputs, result string
	query := `SELECT ` + reportHeaderColumns + `, params::text, inputs::text, result::text
		FROM content_roi_report_version WHERE workspace_id=$1 AND report_id=$2`
	args := []any{workspaceID, reportID}
	if versionNo > 0 {
		query += ` AND version_no=$3`
		args = append(args, versionNo)
	} else {
		query += ` ORDER BY version_no DESC LIMIT 1`
	}
	header, err := scanReportHeader(s.DB.QueryRow(ctx, query, args...), &params, &inputs, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportVersion{}, ErrNotFound
	}
	if err != nil {
		return ReportVersion{}, ErrStorage
	}
	return ReportVersion{
		ReportVersionHeader: header,
		Params:              json.RawMessage(params),
		Inputs:              json.RawMessage(inputs),
		Result:              json.RawMessage(result),
		ChangedInputs:       []ChangedInput{},
		AIExplanation:       ReportAIExplanation(),
	}, nil
}

// ListReports answers the latest version of every report, newest first.
func (s *ROIStore) ListReports(ctx context.Context, workspaceID, actor string) ([]ReportVersionHeader, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT ON (report_id) `+reportHeaderColumns+`
		FROM content_roi_report_version WHERE workspace_id=$1
		ORDER BY report_id, version_no DESC`, workspaceID)
	if err != nil {
		return nil, ErrStorage
	}
	headers, err := collect(rows, func(row scanner) (ReportVersionHeader, error) { return scanReportHeader(row) })
	if err != nil {
		return nil, err
	}
	slices.SortFunc(headers, func(left, right ReportVersionHeader) int {
		return cmp.Or(right.CreatedAt.Compare(left.CreatedAt), cmp.Compare(left.ReportID, right.ReportID))
	})
	return headers, nil
}

// ListReportVersions answers every version of one report, oldest first.
func (s *ROIStore) ListReportVersions(ctx context.Context, workspaceID, actor, reportID string) ([]ReportVersionHeader, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT `+reportHeaderColumns+`
		FROM content_roi_report_version WHERE workspace_id=$1 AND report_id=$2
		ORDER BY version_no`, workspaceID, reportID)
	if err != nil {
		return nil, ErrStorage
	}
	headers, err := collect(rows, func(row scanner) (ReportVersionHeader, error) { return scanReportHeader(row) })
	if err != nil {
		return nil, err
	}
	if len(headers) == 0 {
		return nil, ErrNotFound
	}
	return headers, nil
}

// GetReportVersion answers one stored version as it was stored, with
// inputs_changed derived now (FR-055): the version's included set against
// the included set generating with the same params would have today. A
// revision outside the window or scope changes neither, and flags nothing.
func (s *ROIStore) GetReportVersion(ctx context.Context, workspaceID, actor, reportID string, versionNo int) (ReportVersion, error) {
	if !s.ready() {
		return ReportVersion{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return ReportVersion{}, ErrInvalid
	}
	if versionNo < 1 {
		return ReportVersion{}, ErrNotFound
	}
	version, err := s.readReportVersion(ctx, workspaceID, reportID, versionNo)
	if err != nil {
		return ReportVersion{}, err
	}
	var params ReportParams
	var inputs ReportInputs
	if json.Unmarshal(version.Params, &params) != nil || json.Unmarshal(version.Inputs, &inputs) != nil {
		return ReportVersion{}, ErrStorage
	}
	current, err := s.LoadReportInput(ctx, workspaceID, actor, params)
	if err != nil {
		return ReportVersion{}, ErrStorage
	}
	included, err := includedRecords(current)
	if err != nil {
		return ReportVersion{}, ErrStorage
	}
	version.ChangedInputs = diffReportInputs(inputs.Records, included)
	version.InputsChanged = len(version.ChangedInputs) > 0
	return version, nil
}

// ---------------------------------------------------------------- public summary

// ReportSummaryFields is contract §7's allow-list: every JSON field name that
// may appear anywhere in a ReportSummary, nested ones included. A reflection
// test holds ReportSummary to it.
var ReportSummaryFields = []string{
	"report_id", "version_no", "title", "calc_version", "created_at",
	"params", "window", "start", "end", "timezone", "report_currency", "rates", "from", "to", "rate",
	"entered_at", "attribution_method", "conversion", "from_stage", "to_stage", "booking_stage",
	"scope", "account_ids", "work_ids", "campaign_labels", "generated_at",
	"metrics", "status", "display", "reason", "formula", "records",
	"kind", "id", "revision", "amount_minor", "currency", "converted_minor",
	"breakdown", "by_work", "by_account", "attributed_net_revenue", "attributed_gross_profit",
	"value", "deals_touched",
	"rules",
}

// ReportSummary is what a later reader - BO-02's diagnosis, or an AI
// explanation layer - may see of one report version (contract §7, FR-058,
// FR-062). It has no customer_ref, order_ref, note, evidence note, recorder
// or any other text of a lead or deal.
type ReportSummary struct {
	ReportID    string                     `json:"report_id"`
	VersionNo   int                        `json:"version_no"`
	Title       string                     `json:"title"`
	CalcVersion string                     `json:"calc_version"`
	Params      SummaryParams              `json:"params"`
	Metrics     map[MetricID]SummaryMetric `json:"metrics"`
	Breakdown   SummaryBreakdown           `json:"breakdown"`
	Rules       []string                   `json:"rules"`
	CreatedAt   time.Time                  `json:"created_at"`
}

// SummaryParams are a report's params without each rate's note and enterer.
type SummaryParams struct {
	Window            ReportWindow     `json:"window"`
	ReportCurrency    string           `json:"report_currency"`
	Rates             []SummaryRate    `json:"rates"`
	AttributionMethod string           `json:"attribution_method"`
	Conversion        ConversionStages `json:"conversion"`
	BookingStage      string           `json:"booking_stage"`
	Scope             ReportScope      `json:"scope"`
	GeneratedAt       string           `json:"generated_at"`
}

// SummaryRate is one rate as the summary shows it.
type SummaryRate struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Rate      string `json:"rate"`
	EnteredAt string `json:"entered_at"`
}

// SummaryMetric is one metric's status, display, reason, formula and the
// records it came from (kind, id, revision and amounts only).
type SummaryMetric struct {
	Status  string      `json:"status"`
	Display string      `json:"display"`
	Reason  ReasonCode  `json:"reason"`
	Formula string      `json:"formula"`
	Records []RecordRef `json:"records"`
}

// SummaryBreakdown is the amounts and counts per work and per account.
type SummaryBreakdown struct {
	ByWork    []BreakdownRow `json:"by_work"`
	ByAccount []BreakdownRow `json:"by_account"`
}

// summarize builds a summary from a stored version's header, params and
// result, copying only the allowed fields.
func summarize(header ReportVersionHeader, params ReportParams, result Result) ReportSummary {
	summary := ReportSummary{
		ReportID: header.ReportID, VersionNo: header.VersionNo, Title: header.Title,
		CalcVersion: header.CalcVersion, CreatedAt: header.CreatedAt,
		Params: SummaryParams{
			Window: params.Window, ReportCurrency: params.ReportCurrency, Rates: []SummaryRate{},
			AttributionMethod: params.AttributionMethod, Conversion: params.Conversion,
			BookingStage: params.BookingStage, Scope: params.Scope, GeneratedAt: params.GeneratedAt,
		},
		Metrics:   map[MetricID]SummaryMetric{},
		Breakdown: SummaryBreakdown{ByWork: nonNil(result.Breakdown.ByWork), ByAccount: nonNil(result.Breakdown.ByAccount)},
		Rules:     nonNil(result.Rules),
	}
	for _, rate := range params.Rates {
		summary.Params.Rates = append(summary.Params.Rates, SummaryRate{
			From: rate.From, To: rate.To, Rate: rate.Rate, EnteredAt: rate.EnteredAt,
		})
	}
	for id, metric := range result.Metrics {
		summary.Metrics[id] = SummaryMetric{
			Status: metric.Status, Display: metric.Display, Reason: metric.Reason,
			Formula: metric.Formula, Records: nonNil(metric.Records),
		}
	}
	return summary
}

// ReportSummary answers the public summary of one stored version (contract
// §7). It reads the stored params and result; it recomputes nothing.
func (s *Store) ReportSummary(ctx context.Context, workspaceID, reportID string, versionNo int) (ReportSummary, error) {
	if s == nil || s.DB == nil {
		return ReportSummary{}, ErrStorage
	}
	if workspaceID == "" || reportID == "" || versionNo < 1 {
		return ReportSummary{}, ErrNotFound
	}
	store := &ROIStore{Store: s}
	version, err := store.readReportVersion(ctx, workspaceID, reportID, versionNo)
	if err != nil {
		return ReportSummary{}, err
	}
	var params ReportParams
	var result Result
	if json.Unmarshal(version.Params, &params) != nil || json.Unmarshal(version.Result, &result) != nil {
		return ReportSummary{}, ErrStorage
	}
	return summarize(version.ReportVersionHeader, params, result), nil
}
