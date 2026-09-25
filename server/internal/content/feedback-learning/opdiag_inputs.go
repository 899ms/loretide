package feedbacklearning

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// What an operating diagnosis reads, in which order, and what of it is kept
// (specs/035 contract §6, FR-041 to FR-043, FR-081, FR-083).
//
// Three rules the shape of this file exists to keep:
//
//   - Authorize first (the handler), then every account in the params is
//     checked to exist here, and only then is anything about any account
//     read. A foreign account id is refused like a missing one before its
//     profile, works, metrics or feedback are touched (FR-081). A test
//     records the calls and holds the order.
//   - Other modules are read through the small interfaces below, answered
//     by handler adapters with those modules' public reads; this module
//     imports none of them and reads none of their tables (FR-083). Only
//     its own metrics, excerpts and work marks are read here directly.
//   - An input keeps the fields the calculator needs and nothing else: a
//     profile item's status and never its text, an excerpt's tags and never
//     its words or interpretation, no evidence note, no recorder (FR-042).
//     Each input carries a fingerprint of those fields, so "the inputs have
//     changed since" is a comparison and never a stored flag (FR-043).
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §6

// ---------------------------------------------------------------- what the adapters answer

// DiagAccount is a content account as the diagnosis needs it.
type DiagAccount struct {
	AccountID   string
	Platform    string
	DisplayName string
}

// DiagProfile is an account's current expression profile revision: its id
// and each item's status, by the item's JSON name. RevisionID "" is an
// account with no revision yet; every item of it is then not confirmed.
type DiagProfile struct {
	RevisionID  string
	FieldStatus map[string]string
}

// DiagOperatingRules is the part of the brand's operating rules the
// diagnosis reads: the weekly cadence per channel (an absent channel is
// "nobody decided", not 0) and the feedback observation window.
type DiagOperatingRules struct {
	Cadence     map[string]int64          `json:"cadence"`
	Observation workspacecore.Observation `json:"observation"`
}

// DiagPublication is one publication record at its latest status.
type DiagPublication struct {
	PublicationRecordID string
	WorkID              string
	Channel             string
	Status              string
	PublishedAt         *time.Time
}

// DiagWork is one work.
type DiagWork struct {
	WorkID           string
	TopicCardID      string
	Title            string
	HistoricalImport bool
}

// DiagReview is one review request.
type DiagReview struct {
	ReviewRequestID string
	AccountID       string
	Status          string
	RequestedAt     time.Time
	DecidedAt       *time.Time
}

// DiagDeliveryTask is one delivery task; Due is review-delivery's own
// read-time derivation, taken as given.
type DiagDeliveryTask struct {
	DeliveryTaskID  string
	ReviewRequestID string
	Status          string
	Due             bool
}

// DiagAccounts answers from ip-profile. AccountExists is the only question
// asked before every account in the params is known to be here.
type DiagAccounts interface {
	Accounts
	Account(ctx context.Context, workspaceID, accountID string) (DiagAccount, error)
	CurrentProfile(ctx context.Context, workspaceID, accountID string) (DiagProfile, error)
}

// DiagRules answers the brand's operating rules and timezone from
// workspace-core's settings.
type DiagRules interface {
	OperatingRules(ctx context.Context, workspaceID string) (DiagOperatingRules, error)
	Location(ctx context.Context, workspaceID string) *time.Location
}

// DiagTopics answers which account a topic card is for ("" for none), or
// ErrNotFound.
type DiagTopics interface {
	TopicCardAccount(ctx context.Context, workspaceID, actor, topicCardID string) (string, error)
}

// DiagWorks answers from work-editor.
type DiagWorks interface {
	Works
	Work(ctx context.Context, workspaceID, actor, workID string) (DiagWork, error)
}

// DiagDelivery answers from review-delivery: every publication record,
// review request and delivery task of the workspace. There is no time filter
// on those reads, and at a brand's scale none is needed; the window is
// applied here, in Go.
type DiagDelivery interface {
	Publications(ctx context.Context, workspaceID, actor string) ([]DiagPublication, error)
	Reviews(ctx context.Context, workspaceID, actor string) ([]DiagReview, error)
	Tasks(ctx context.Context, workspaceID, actor string) ([]DiagDeliveryTask, error)
}

// diagnosisROISummaries answers a 034 ROI report version's public summary.
// It is this module's own read (Store.ReportSummary): the ROI reports live
// in feedback-learning, so no handler adapter sits in between. nil on a
// DiagnosisStore means that read; a test replaces it.
type diagnosisROISummaries interface {
	ReportSummary(ctx context.Context, workspaceID, reportID string, versionNo int) (ReportSummary, error)
}

// diagnosisOwnRecords reads this module's own tables for the diagnosis:
// the metrics and excerpts of the publications in scope and the marks on
// their works. nil on a DiagnosisStore means its database.
type diagnosisOwnRecords interface {
	ownDiagnosisRecords(ctx context.Context, workspaceID string, publicationIDs, workIDs []string) (
		[]ManualMetric, []FeedbackExcerpt, []WorkMark, error)
}

// ---------------------------------------------------------------- the stored copy

// DiagnosisInput is one input as stored in a report version: its kind and
// id, the fields the calculator needs, and the SHA-256 of those fields in
// canonical JSON.
type DiagnosisInput struct {
	Kind        string          `json:"kind"`
	ID          string          `json:"id"`
	Fingerprint string          `json:"fingerprint"`
	Fields      json.RawMessage `json:"fields"`
}

// Input kinds (contract §6).
const (
	inputAccount        = "account"
	inputProfile        = "profile"
	inputOperatingRules = "operating_rules"
	inputPublication    = "publication"
	inputWork           = "work"
	inputTopicCard      = "topic_card"
	inputMetric         = "metric"
	inputExcerpt        = "excerpt"
	inputReview         = "review"
	inputDeliveryTask   = "delivery_task"
	inputWorkMark       = "work_mark"
	inputROISummary     = "roi_summary"
)

// roiInputID is the id of the ROI summary input: the version it copies.
func roiInputID(ref DiagnosisROIRef) string {
	return ref.ReportID + "/" + strconv.Itoa(ref.VersionNo)
}

// operatingRulesInputID is the id of the one operating rules input: the
// rules have no version of their own, so the copy is the version.
const operatingRulesInputID = "operating_rules"

// The fields kept of each kind (contract §6). Times are RFC 3339 in UTC.
type (
	diagAccountFields struct {
		Platform    string `json:"platform"`
		DisplayName string `json:"display_name"`
	}
	// diagProfileFields keeps each item's status and never its text.
	diagProfileFields struct {
		RevisionID  string            `json:"revision_id"`
		FieldStatus map[string]string `json:"field_status"`
	}
	diagPublicationFields struct {
		WorkID      string  `json:"work_id"`
		Channel     string  `json:"channel"`
		Status      string  `json:"status"`
		PublishedAt *string `json:"published_at"`
	}
	diagWorkFields struct {
		TopicCardID      string `json:"topic_card_id"`
		HistoricalImport bool   `json:"historical_import"`
		Title            string `json:"title"`
	}
	diagTopicCardFields struct {
		AccountID string `json:"account_id"`
	}
	// diagMetricFields keeps a nil value nil: unknown is not zero.
	diagMetricFields struct {
		PublicationRecordID string `json:"publication_record_id"`
		Platform            string `json:"platform"`
		AccountID           string `json:"account_id"`
		Metric              string `json:"metric"`
		Value               *int64 `json:"value"`
		StatWindow          string `json:"stat_window"`
		SampledAt           string `json:"sampled_at"`
		CreatedAt           string `json:"created_at"`
	}
	// diagExcerptFields keeps the tags a person gave, never the words.
	diagExcerptFields struct {
		PublicationRecordID string   `json:"publication_record_id"`
		SourceType          string   `json:"source_type"`
		Tags                []string `json:"tags"`
		OccurredAt          string   `json:"occurred_at"`
	}
	diagReviewFields struct {
		AccountID   string  `json:"account_id"`
		Status      string  `json:"status"`
		RequestedAt string  `json:"requested_at"`
		DecidedAt   *string `json:"decided_at"`
	}
	diagDeliveryTaskFields struct {
		ReviewRequestID string `json:"review_request_id"`
		Status          string `json:"status"`
		Due             bool   `json:"due"`
	}
	diagWorkMarkFields struct {
		WorkID            string `json:"work_id"`
		Kind              string `json:"kind"`
		Item              string `json:"item"`
		Verdict           string `json:"verdict"`
		AccountID         string `json:"account_id"`
		ProfileRevisionID string `json:"profile_revision_id"`
		CreatedAt         string `json:"created_at"`
	}
)

func diagTime(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

func diagTimePtr(at *time.Time) *string {
	if at == nil {
		return nil
	}
	text := diagTime(*at)
	return &text
}

// newDiagnosisInput copies fields in canonical JSON and fingerprints them.
func newDiagnosisInput(kind, id string, fields any) (DiagnosisInput, error) {
	encoded, err := json.Marshal(fields)
	if err != nil {
		return DiagnosisInput{}, ErrStorage
	}
	canonical, err := CanonicalJSON(encoded)
	if err != nil {
		return DiagnosisInput{}, ErrStorage
	}
	return DiagnosisInput{Kind: kind, ID: id, Fingerprint: diagnosisFingerprint(canonical), Fields: canonical}, nil
}

// diagnosisFingerprint is the SHA-256 of an input's canonical fields. Field
// order does not change it; any field's value does.
func diagnosisFingerprint(canonicalFields []byte) string {
	sum := sha256.Sum256(canonicalFields)
	return hex.EncodeToString(sum[:])
}

func sortDiagnosisInputs(inputs []DiagnosisInput) {
	slices.SortFunc(inputs, func(left, right DiagnosisInput) int {
		return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID))
	})
}

// ---------------------------------------------------------------- inputs_changed

// DiagnosisInputsChanged is derived on read and never stored (FR-043): the
// inputs a stored version read, against the inputs generating now with the
// same params would read, matched by (kind, id) and compared by fingerprint.
type DiagnosisInputsChanged struct {
	Changed  bool                 `json:"changed"`
	Added    []DiagnosisRecordRef `json:"added"`
	Modified []DiagnosisRecordRef `json:"modified"`
	Removed  []DiagnosisRecordRef `json:"removed"`
}

func noDiagnosisChanges() DiagnosisInputsChanged {
	return DiagnosisInputsChanged{Added: []DiagnosisRecordRef{}, Modified: []DiagnosisRecordRef{}, Removed: []DiagnosisRecordRef{}}
}

func diffDiagnosisInputs(stored, current []DiagnosisInput) DiagnosisInputsChanged {
	changes := noDiagnosisChanges()
	before := map[DiagnosisRecordRef]string{}
	for _, input := range stored {
		before[DiagnosisRecordRef{Kind: input.Kind, ID: input.ID}] = input.Fingerprint
	}
	after := map[DiagnosisRecordRef]string{}
	for _, input := range current {
		after[DiagnosisRecordRef{Kind: input.Kind, ID: input.ID}] = input.Fingerprint
	}
	for key, fingerprint := range after {
		if old, ok := before[key]; !ok {
			changes.Added = append(changes.Added, key)
		} else if old != fingerprint {
			changes.Modified = append(changes.Modified, key)
		}
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			changes.Removed = append(changes.Removed, key)
		}
	}
	byKey := func(left, right DiagnosisRecordRef) int {
		return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID))
	}
	slices.SortFunc(changes.Added, byKey)
	slices.SortFunc(changes.Modified, byKey)
	slices.SortFunc(changes.Removed, byKey)
	changes.Changed = len(changes.Added)+len(changes.Modified)+len(changes.Removed) > 0
	return changes
}

// ---------------------------------------------------------------- collection

// Review and delivery states the collection reads as still open. A review
// that was rejected inside the window is read too; a delivery task that was
// handed off or cancelled is closed.
var (
	openReviewStatuses   = []string{"pending", "changes_requested"}
	rejectedReviewStatus = "rejected"
	closedTaskStatuses   = []string{"handed_off", "cancelled"}
	// currentProblemStatuses are the publication statuses the execution
	// flow lists as they are now: failed, unknown, removed.
	currentProblemStatuses = []string{"failed", "unknown", "removed"}
)

// diagnosisAccountIDs are the params' account ids as given, without blanks
// or repeats. Existence is checked for these before the params are checked
// at all, so a foreign id answers like a missing one whatever else is wrong.
func diagnosisAccountIDs(params DiagnosisParams) []string {
	ids := []string{}
	for _, id := range params.Scope.AccountIDs {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// checkDiagnosisDimensions refuses a dimension this build has no calculator
// for. PR 2 registers all six, so only a stored version from a build that
// knew a dimension this one does not would be refused.
func checkDiagnosisDimensions(params DiagnosisParams) error {
	for _, dimension := range params.Dimensions {
		if _, ok := dimensionCalculators[dimension.Key]; !ok {
			return FieldError{Field: "dimensions", Reason: "not available in this version"}
		}
	}
	return nil
}

func (s *DiagnosisStore) roiReports() diagnosisROISummaries {
	if s.roiSummaries != nil {
		return s.roiSummaries
	}
	return s.Store
}

func (s *DiagnosisStore) ownRecords() diagnosisOwnRecords {
	if s.own != nil {
		return s.own
	}
	return s
}

// gatherDiagnosisInputs is the contract's decision order and collection
// order in one place:
//
//  1. (the handler has authorized the caller)
//  2. every account in the params exists here - and nothing else about any
//     account has been read yet - and so does the ROI report version the
//     params name, if they name one;
//  3. to 5. the params' controlled sets, required fields and combinations,
//     and whether this build computes the selected dimensions;
//  6. accounts and profiles, operating rules, publication records (all of
//     the workspace's, narrowed here to the published ones in the windows),
//     their works, those works' topic cards, reviews and delivery tasks,
//     then this module's metrics, excerpts and work marks.
//
// A record whose account resolves to another account in the workspace is
// read to find that out and then left out; one no account can be found for
// is kept, because a brand summary shows it in its own section and every
// report names the historical-import limitation.
//
// lenient is for inputs_changed: an account that has gone since the version
// was generated is left out instead of refusing, so reading an old version
// still works and reports the account as removed.
func (s *DiagnosisStore) gatherDiagnosisInputs(ctx context.Context, workspaceID, actor string,
	params DiagnosisParams, lenient bool) (preparedDiagnosis, []DiagnosisInput, error) {
	if s.Accounts == nil || s.Rules == nil || s.Topics == nil || s.Works == nil || s.Delivery == nil {
		return preparedDiagnosis{}, nil, ErrStorage
	}
	present := []string{}
	for _, id := range diagnosisAccountIDs(params) {
		err := adapterError(s.Accounts.AccountExists(ctx, workspaceID, id))
		switch {
		case err == nil:
			present = append(present, id)
		case lenient && errors.Is(err, ErrNotFound):
		default:
			return preparedDiagnosis{}, nil, err
		}
	}
	// Decision step 2 for the ROI reference (Q6): the version exists here.
	// Its summary is read now, kept, and copied in last; a missing one is
	// refused like a missing account.
	var roiSummary *ReportSummary
	if ref := params.ROIReportRef; ref != nil {
		summary, readErr := s.roiReports().ReportSummary(ctx, workspaceID, ref.ReportID, ref.VersionNo)
		switch err := adapterError(readErr); {
		case err == nil:
			roiSummary = &summary
		case lenient && errors.Is(err, ErrNotFound):
		default:
			return preparedDiagnosis{}, nil, err
		}
	}
	prepared, err := prepareDiagnosisParams(params)
	if err != nil {
		return preparedDiagnosis{}, nil, err
	}
	if err = checkDiagnosisDimensions(params); err != nil {
		return preparedDiagnosis{}, nil, err
	}

	inputs := []DiagnosisInput{}
	add := func(kind, id string, fields any) error {
		input, addErr := newDiagnosisInput(kind, id, fields)
		if addErr != nil {
			return addErr
		}
		inputs = append(inputs, input)
		return nil
	}

	inScope := map[string]bool{"": true}
	for _, id := range present {
		inScope[id] = true
		account, readErr := s.Accounts.Account(ctx, workspaceID, id)
		if readErr != nil {
			return preparedDiagnosis{}, nil, adapterError(readErr)
		}
		profile, readErr := s.Accounts.CurrentProfile(ctx, workspaceID, id)
		if readErr != nil {
			return preparedDiagnosis{}, nil, adapterError(readErr)
		}
		status := map[string]string{}
		for _, key := range profileFieldKeys {
			status[key] = "pending"
			if profile.FieldStatus[key] == ProfileFieldConfirmed {
				status[key] = ProfileFieldConfirmed
			}
		}
		if err = add(inputAccount, id, diagAccountFields{Platform: account.Platform, DisplayName: account.DisplayName}); err != nil {
			return preparedDiagnosis{}, nil, err
		}
		if err = add(inputProfile, id, diagProfileFields{RevisionID: profile.RevisionID, FieldStatus: status}); err != nil {
			return preparedDiagnosis{}, nil, err
		}
	}

	rules, err := s.Rules.OperatingRules(ctx, workspaceID)
	if err != nil {
		return preparedDiagnosis{}, nil, adapterError(err)
	}
	if rules.Cadence == nil {
		rules.Cadence = map[string]int64{}
	}
	if err = add(inputOperatingRules, operatingRulesInputID, rules); err != nil {
		return preparedDiagnosis{}, nil, err
	}

	// Publication records: published inside the report window - and, only
	// when a selected dimension needs them, published inside the comparison
	// window (performance), published with no published_at (cadence, which
	// lists them as gaps) and at a failed, unknown or removed status now
	// (execution flow). A version with no dimension reads what PR 1 read.
	// Metrics and excerpts are read for the two windows' records only, so
	// what one dimension needs never changes another's result.
	publications, err := s.Delivery.Publications(ctx, workspaceID, actor)
	if err != nil {
		return preparedDiagnosis{}, nil, adapterError(err)
	}
	slices.SortFunc(publications, func(left, right DiagPublication) int {
		return cmp.Compare(left.PublicationRecordID, right.PublicationRecordID)
	})
	candidates := []DiagPublication{}
	inReportWindow, withRecords := map[string]bool{}, map[string]bool{}
	for _, publication := range publications {
		published := slices.Contains(PublishedStatuses, publication.Status)
		switch {
		case published && prepared.inWindow(publication.PublishedAt):
			inReportWindow[publication.PublicationRecordID] = true
			withRecords[publication.PublicationRecordID] = true
		case published && prepared.selected(DimensionPerformance) && prepared.inComparison(publication.PublishedAt):
			withRecords[publication.PublicationRecordID] = true
		case published && prepared.selected(DimensionCadence) && publication.PublishedAt == nil:
		case !published && prepared.selected(DimensionExecutionFlow) && slices.Contains(currentProblemStatuses, publication.Status):
		default:
			continue
		}
		candidates = append(candidates, publication)
	}

	// Their works, and the works' topic cards: the only way a publication
	// record reaches an account (FR-026).
	works := map[string]*DiagWork{}
	for _, publication := range candidates {
		if _, seen := works[publication.WorkID]; seen || publication.WorkID == "" {
			continue
		}
		work, readErr := s.Works.Work(ctx, workspaceID, actor, publication.WorkID)
		switch {
		case readErr == nil:
			works[publication.WorkID] = &work
		case errors.Is(adapterError(readErr), ErrNotFound):
			works[publication.WorkID] = nil
		default:
			return preparedDiagnosis{}, nil, adapterError(readErr)
		}
	}
	cards := map[string]*string{}
	for _, work := range works {
		if work == nil || work.TopicCardID == "" {
			continue
		}
		if _, seen := cards[work.TopicCardID]; seen {
			continue
		}
		accountID, readErr := s.Topics.TopicCardAccount(ctx, workspaceID, actor, work.TopicCardID)
		switch {
		case readErr == nil:
			cards[work.TopicCardID] = &accountID
		case errors.Is(adapterError(readErr), ErrNotFound):
			cards[work.TopicCardID] = nil
		default:
			return preparedDiagnosis{}, nil, adapterError(readErr)
		}
	}
	accountOf := func(publication DiagPublication) string {
		work := works[publication.WorkID]
		if work == nil || work.TopicCardID == "" {
			return ""
		}
		if accountID := cards[work.TopicCardID]; accountID != nil {
			return *accountID
		}
		return ""
	}
	keptPublications, keptWorks, keptCards := []string{}, []string{}, []string{}
	recordPublications, markWorks := []string{}, []string{}
	for _, publication := range candidates {
		if !inScope[accountOf(publication)] {
			continue
		}
		keptPublications = append(keptPublications, publication.PublicationRecordID)
		if withRecords[publication.PublicationRecordID] {
			recordPublications = append(recordPublications, publication.PublicationRecordID)
		}
		if inReportWindow[publication.PublicationRecordID] && publication.WorkID != "" &&
			!slices.Contains(markWorks, publication.WorkID) {
			markWorks = append(markWorks, publication.WorkID)
		}
		if err = add(inputPublication, publication.PublicationRecordID, diagPublicationFields{
			WorkID: publication.WorkID, Channel: publication.Channel, Status: publication.Status,
			PublishedAt: diagTimePtr(publication.PublishedAt),
		}); err != nil {
			return preparedDiagnosis{}, nil, err
		}
		work := works[publication.WorkID]
		if work == nil || slices.Contains(keptWorks, work.WorkID) {
			continue
		}
		keptWorks = append(keptWorks, work.WorkID)
		if err = add(inputWork, work.WorkID, diagWorkFields{
			TopicCardID: work.TopicCardID, HistoricalImport: work.HistoricalImport, Title: work.Title,
		}); err != nil {
			return preparedDiagnosis{}, nil, err
		}
		if accountID := cards[work.TopicCardID]; accountID != nil && !slices.Contains(keptCards, work.TopicCardID) {
			keptCards = append(keptCards, work.TopicCardID)
			if err = add(inputTopicCard, work.TopicCardID, diagTopicCardFields{AccountID: *accountID}); err != nil {
				return preparedDiagnosis{}, nil, err
			}
		}
	}

	// Reviews and delivery tasks: what is still open now, and reviews
	// rejected inside the window. A task reaches an account through its
	// review; the review it names is kept with it so that stays true of the
	// stored copy.
	reviews, err := s.Delivery.Reviews(ctx, workspaceID, actor)
	if err != nil {
		return preparedDiagnosis{}, nil, adapterError(err)
	}
	tasks, err := s.Delivery.Tasks(ctx, workspaceID, actor)
	if err != nil {
		return preparedDiagnosis{}, nil, adapterError(err)
	}
	reviewByID := map[string]DiagReview{}
	for _, review := range reviews {
		reviewByID[review.ReviewRequestID] = review
	}
	keptReviews := map[string]bool{}
	slices.SortFunc(tasks, func(left, right DiagDeliveryTask) int {
		return cmp.Compare(left.DeliveryTaskID, right.DeliveryTaskID)
	})
	for _, task := range tasks {
		if slices.Contains(closedTaskStatuses, task.Status) {
			continue
		}
		review, found := reviewByID[task.ReviewRequestID]
		if found && !inScope[review.AccountID] {
			continue
		}
		if found {
			keptReviews[review.ReviewRequestID] = true
		}
		if err = add(inputDeliveryTask, task.DeliveryTaskID, diagDeliveryTaskFields{
			ReviewRequestID: task.ReviewRequestID, Status: task.Status, Due: task.Due,
		}); err != nil {
			return preparedDiagnosis{}, nil, err
		}
	}
	for _, review := range reviews {
		open := slices.Contains(openReviewStatuses, review.Status) ||
			(review.Status == rejectedReviewStatus && prepared.inWindow(review.DecidedAt))
		if (open && inScope[review.AccountID]) || keptReviews[review.ReviewRequestID] {
			if err = add(inputReview, review.ReviewRequestID, diagReviewFields{
				AccountID: review.AccountID, Status: review.Status,
				RequestedAt: diagTime(review.RequestedAt), DecidedAt: diagTimePtr(review.DecidedAt),
			}); err != nil {
				return preparedDiagnosis{}, nil, err
			}
		}
	}

	// This module's own records: the metrics and excerpts of the two
	// windows' records, and the marks on the report window's works. A work
	// published in the window and also earlier keeps all its marks.
	metrics, excerpts, marks, err := s.ownRecords().ownDiagnosisRecords(ctx, workspaceID, recordPublications, markWorks)
	if err != nil {
		return preparedDiagnosis{}, nil, err
	}
	for _, metric := range metrics {
		if err = add(inputMetric, metric.ManualMetricID, diagMetricFields{
			PublicationRecordID: metric.PublicationRecordID, Platform: string(metric.Platform),
			AccountID: metric.AccountID, Metric: string(metric.Metric), Value: metric.Value,
			StatWindow: metric.StatWindow, SampledAt: diagTime(metric.SampledAt), CreatedAt: diagTime(metric.CreatedAt),
		}); err != nil {
			return preparedDiagnosis{}, nil, err
		}
	}
	for _, excerpt := range excerpts {
		if !inReportWindow[excerpt.PublicationRecordID] {
			// The comparison window is performance's; its records' feedback
			// is not this report's audience feedback.
			continue
		}
		if err = add(inputExcerpt, excerpt.FeedbackExcerptID, diagExcerptFields{
			PublicationRecordID: excerpt.PublicationRecordID, SourceType: string(excerpt.SourceType),
			Tags: nonNil(excerpt.Tags), OccurredAt: diagTime(excerpt.OccurredAt),
		}); err != nil {
			return preparedDiagnosis{}, nil, err
		}
	}
	for _, mark := range marks {
		if err = add(inputWorkMark, mark.MarkID, diagWorkMarkFields{
			WorkID: mark.WorkID, Kind: string(mark.Kind), Item: mark.Item, Verdict: string(mark.Verdict),
			AccountID: mark.AccountID, ProfileRevisionID: mark.ProfileRevisionID, CreatedAt: diagTime(mark.CreatedAt),
		}); err != nil {
			return preparedDiagnosis{}, nil, err
		}
	}
	if roiSummary != nil {
		if err = add(inputROISummary, roiInputID(*params.ROIReportRef), roiSummary); err != nil {
			return preparedDiagnosis{}, nil, err
		}
	}
	sortDiagnosisInputs(inputs)
	return prepared, inputs, nil
}
