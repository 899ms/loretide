package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	reviewdelivery "github.com/multica-ai/multica/server/internal/content/review-delivery"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	workeditor "github.com/multica-ai/multica/server/internal/content/work-editor"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// Brand/account operating diagnosis (specs/035 PR 1): report versions and
// work marks under /api/content-operating-diagnosis.
//
// This file is the adapter. feedback-learning owns the diagnosis and imports
// none of the modules it reads: whether an account exists and what its
// profile says (ip-profile), the brand's operating rules and timezone
// (workspace-core settings), which account a topic card is for
// (topic-planning), a work (work-editor), and the publication records,
// reviews and delivery tasks (review-delivery) are all answered below, each
// through the owning module's public read. Nothing here reads a source or a
// knowledge entry.
//
// Every decision, refusals included, goes through workspace-core's
// Authorize first (feedbackScope, the same scope 027 and 034 use); only then
// is the store built. Generating a version and recording a mark are the two
// writes, and each adds one row.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §6, §7

// opdiagAccounts answers from ip-profile's public reads.
type opdiagAccounts struct{ service *ipprofile.Service }

func (a opdiagAccounts) AccountExists(ctx context.Context, workspaceID, accountID string) error {
	return roiAccounts(a).AccountExists(ctx, workspaceID, accountID)
}

func (a opdiagAccounts) Account(ctx context.Context, workspaceID, accountID string) (feedbacklearning.DiagAccount, error) {
	if a.service == nil {
		return feedbacklearning.DiagAccount{}, feedbacklearning.ErrStorage
	}
	account, err := a.service.Get(ctx, workspaceID, accountID)
	switch {
	case err == nil:
		return feedbacklearning.DiagAccount{
			AccountID: account.AccountID, Platform: account.Platform, DisplayName: account.DisplayName,
		}, nil
	case errors.Is(err, ipprofile.ErrNotFound):
		return feedbacklearning.DiagAccount{}, feedbacklearning.ErrNotFound
	default:
		return feedbacklearning.DiagAccount{}, feedbacklearning.ErrStorage
	}
}

// CurrentProfile answers the current revision's id and each item's status,
// never an item's text. An account with no revision answers "" and no
// statuses: every item then counts as not confirmed.
func (a opdiagAccounts) CurrentProfile(ctx context.Context, workspaceID, accountID string) (feedbacklearning.DiagProfile, error) {
	if a.service == nil {
		return feedbacklearning.DiagProfile{}, feedbacklearning.ErrStorage
	}
	revision, err := a.service.CurrentPersonaRevision(ctx, workspaceID, accountID)
	switch {
	case errors.Is(err, ipprofile.ErrNotFound):
		return feedbacklearning.DiagProfile{FieldStatus: map[string]string{}}, nil
	case err != nil:
		return feedbacklearning.DiagProfile{}, feedbacklearning.ErrStorage
	}
	profile := revision.Profile
	return feedbacklearning.DiagProfile{
		RevisionID: revision.RevisionID,
		FieldStatus: map[string]string{
			"audience":              string(profile.Audience.Status),
			"common_questions":      string(profile.CommonQuestions.Status),
			"experience":            string(profile.Experience.Status),
			"positioning":           string(profile.Positioning.Status),
			"content_pillars":       string(profile.ContentPillars.Status),
			"expression_style":      string(profile.ExpressionStyle.Status),
			"forbidden_expressions": string(profile.ForbiddenExpressions.Status),
			"content_goals":         string(profile.ContentGoals.Status),
			"primary_channels":      string(profile.PrimaryChannels.Status),
			"weekly_hours":          string(profile.WeeklyHours.Status),
			"style_samples":         string(profile.StyleSamples.Status),
		},
	}, nil
}

// opdiagRules answers the brand's operating rules through workspace-core's
// store, and its timezone the way the ROI records read it.
type opdiagRules struct {
	store     *workspacecore.Store
	timezones roiTimezones
}

func (r opdiagRules) OperatingRules(ctx context.Context, workspaceID string) (feedbacklearning.DiagOperatingRules, error) {
	if r.store == nil {
		return feedbacklearning.DiagOperatingRules{}, feedbacklearning.ErrStorage
	}
	rules, err := r.store.ReadRules(ctx, workspaceID)
	if err != nil {
		return feedbacklearning.DiagOperatingRules{}, feedbacklearning.ErrStorage
	}
	return feedbacklearning.DiagOperatingRules{Cadence: rules.Cadence, Observation: rules.Observation}, nil
}

func (r opdiagRules) Location(ctx context.Context, workspaceID string) *time.Location {
	return r.timezones.Location(ctx, workspaceID)
}

// opdiagTopics answers which account a topic card is for, through
// topic-planning's public read.
type opdiagTopics struct{ store *topicplanning.Store }

func (t opdiagTopics) TopicCardAccount(ctx context.Context, workspaceID, actor, topicCardID string) (string, error) {
	if t.store == nil {
		return "", feedbacklearning.ErrStorage
	}
	card, err := t.store.Get(ctx, workspaceID, actor, topicCardID)
	switch {
	case err == nil:
		if card.AccountID == nil {
			return "", nil
		}
		return *card.AccountID, nil
	case errors.Is(err, topicplanning.ErrNotFound), errors.Is(err, topicplanning.ErrInvalid):
		return "", feedbacklearning.ErrNotFound
	default:
		return "", feedbacklearning.ErrStorage
	}
}

// opdiagWorks answers from work-editor's public read.
type opdiagWorks struct{ store *workeditor.Store }

func (w opdiagWorks) WorkExists(ctx context.Context, workspaceID, actor, workID string) error {
	return roiWorks(w).WorkExists(ctx, workspaceID, actor, workID)
}

func (w opdiagWorks) Work(ctx context.Context, workspaceID, actor, workID string) (feedbacklearning.DiagWork, error) {
	if w.store == nil {
		return feedbacklearning.DiagWork{}, feedbacklearning.ErrStorage
	}
	work, err := w.store.GetWork(ctx, workspaceID, actor, workID)
	switch {
	case err == nil:
		return feedbacklearning.DiagWork{
			WorkID: work.WorkID, TopicCardID: work.TopicCardID, Title: work.Title,
			HistoricalImport: work.HistoricalImport,
		}, nil
	case errors.Is(err, workeditor.ErrNotFound), errors.Is(err, workeditor.ErrInvalid):
		return feedbacklearning.DiagWork{}, feedbacklearning.ErrNotFound
	default:
		return feedbacklearning.DiagWork{}, feedbacklearning.ErrStorage
	}
}

// opdiagDelivery answers from review-delivery's three public lists, each
// over the whole workspace (artifact id "").
type opdiagDelivery struct{ store *reviewdelivery.Store }

func (d opdiagDelivery) Publications(ctx context.Context, workspaceID, actor string) ([]feedbacklearning.DiagPublication, error) {
	if d.store == nil {
		return nil, feedbacklearning.ErrStorage
	}
	records, err := d.store.ListPublications(ctx, workspaceID, actor, "")
	if err != nil {
		return nil, feedbacklearning.ErrStorage
	}
	out := make([]feedbacklearning.DiagPublication, 0, len(records))
	for _, record := range records {
		out = append(out, feedbacklearning.DiagPublication{
			PublicationRecordID: record.PublicationRecordID, WorkID: record.WorkID,
			Channel: string(record.Channel), Status: string(record.Status), PublishedAt: record.PublishedAt,
		})
	}
	return out, nil
}

func (d opdiagDelivery) Reviews(ctx context.Context, workspaceID, actor string) ([]feedbacklearning.DiagReview, error) {
	if d.store == nil {
		return nil, feedbacklearning.ErrStorage
	}
	reviews, err := d.store.ListReviews(ctx, workspaceID, actor, "", "")
	if err != nil {
		return nil, feedbacklearning.ErrStorage
	}
	out := make([]feedbacklearning.DiagReview, 0, len(reviews))
	for _, review := range reviews {
		out = append(out, feedbacklearning.DiagReview{
			ReviewRequestID: review.ReviewRequestID, AccountID: review.AccountID, Status: string(review.Status),
			RequestedAt: review.RequestedAt, DecidedAt: review.DecidedAt,
		})
	}
	return out, nil
}

func (d opdiagDelivery) Tasks(ctx context.Context, workspaceID, actor string) ([]feedbacklearning.DiagDeliveryTask, error) {
	if d.store == nil {
		return nil, feedbacklearning.ErrStorage
	}
	tasks, err := d.store.ListTasks(ctx, workspaceID, actor, "", "")
	if err != nil {
		return nil, feedbacklearning.ErrStorage
	}
	out := make([]feedbacklearning.DiagDeliveryTask, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, feedbacklearning.DiagDeliveryTask{
			DeliveryTaskID: task.DeliveryTaskID, ReviewRequestID: task.ReviewRequestID,
			Status: string(task.Status), Due: task.Due,
		})
	}
	return out, nil
}

// opdiagStore wires the diagnosis store: 027's store, and one adapter per
// module the diagnosis reads.
func (h *Handler) opdiagStore() *feedbacklearning.DiagnosisStore {
	return &feedbacklearning.DiagnosisStore{
		Store:    h.feedbackStore(),
		Accounts: opdiagAccounts{service: h.contentAccountService()},
		Rules:    opdiagRules{store: h.operatingRulesStore(), timezones: roiTimezones{db: h.DB}},
		Topics:   opdiagTopics{store: h.topicPlanningStore()},
		Works:    opdiagWorks{store: h.workEditorStore()},
		Delivery: opdiagDelivery{store: h.reviewDeliveryStore()},
	}
}

// opdiagReportBody is DiagnosisReportRequest (contract §4). The timezone
// and generation time inside params are the server's to write.
type opdiagReportBody struct {
	feedbacklearning.DiagnosisReportRequest
}

// opdiagMarkBody is WorkMarkInput. The fields the server writes are
// accepted and dropped, so a client that echoes a mark back does not fail on
// them and cannot choose them.
type opdiagMarkBody struct {
	feedbacklearning.WorkMarkInput
	ProfileRevisionID json.RawMessage `json:"profile_revision_id"`
	RecordedBy        json.RawMessage `json:"recorded_by"`
	MarkID            json.RawMessage `json:"mark_id"`
	CreatedAt         json.RawMessage `json:"created_at"`
}

// decodeOpdiagBody decodes strictly. An unknown member - a "role" or an
// "industry", which this diagnosis does not have (FR-004) - is refused
// naming that member; a value of the wrong JSON type is refused naming its
// field. Only JSON names reach the response, never a Go type name.
func decodeOpdiagBody(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		return nil
	}
	if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		if field := jsonFieldPath(typeErr.Field); field != "" {
			return feedbacklearning.FieldError{Field: field, Reason: "wrong JSON type"}
		}
	}
	if field := unknownJSONField(err); field != "" {
		return feedbacklearning.FieldError{Field: field, Reason: "unknown field"}
	}
	return feedbacklearning.ErrInvalid
}

// unknownJSONField reads the member name out of encoding/json's
// DisallowUnknownFields error, `json: unknown field "role"`. The name is the
// caller's own JSON key; anything that does not look like one is not echoed.
func unknownJSONField(err error) string {
	const prefix = `json: unknown field "`
	text := err.Error()
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, `"`) {
		return ""
	}
	name := strings.TrimSuffix(strings.TrimPrefix(text, prefix), `"`)
	if name == "" || len(name) > 64 || strings.ContainsFunc(name, func(r rune) bool {
		return !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		return ""
	}
	return name
}

// ---------------------------------------------------------------- reports

// Each handler takes the store through a function called only after the
// caller has been authorized: a refusal builds nothing and reads nothing.

func (h *Handler) ListContentOpDiagReports(w http.ResponseWriter, r *http.Request) {
	h.listOpdiagReports(w, r, h.opdiagStore)
}

func (h *Handler) listOpdiagReports(w http.ResponseWriter, r *http.Request, store func() *feedbacklearning.DiagnosisStore) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	reports, err := store().ListDiagnosisReports(r.Context(), workspace, actor)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

// CreateContentOpDiagReport generates version 1 of a new report.
func (h *Handler) CreateContentOpDiagReport(w http.ResponseWriter, r *http.Request) {
	h.createOpdiagReport(w, r, h.opdiagStore)
}

func (h *Handler) createOpdiagReport(w http.ResponseWriter, r *http.Request, store func() *feedbacklearning.DiagnosisStore) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body opdiagReportBody
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	version, err := store().CreateDiagnosisReport(r.Context(), workspace, actor, body.DiagnosisReportRequest, time.Now())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

// ListContentOpDiagReportVersions answers every version of one report.
func (h *Handler) ListContentOpDiagReportVersions(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	reportID := chi.URLParam(r, "reportId")
	versions, err := h.opdiagStore().ListDiagnosisReportVersions(r.Context(), workspace, actor, reportID)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report_id": reportID, "versions": versions})
}

// GenerateContentOpDiagReportVersion generates the next version from the
// inputs as they are now; absent title and params reuse the previous
// version's.
func (h *Handler) GenerateContentOpDiagReportVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body opdiagReportBody
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	version, err := h.opdiagStore().GenerateDiagnosisReportVersion(r.Context(), workspace, actor,
		chi.URLParam(r, "reportId"), body.DiagnosisReportRequest, time.Now())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

// GetContentOpDiagReportVersion answers one version as stored, with
// inputs_changed derived on read. A {versionNo} that is not a positive
// integer answers exactly like a version that does not exist.
func (h *Handler) GetContentOpDiagReportVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	versionNo, valid := feedbacklearning.ParseVersionNo(chi.URLParam(r, "versionNo"))
	if !valid {
		h.roiError(w, feedbacklearning.ErrNotFound)
		return
	}
	version, err := h.opdiagStore().GetDiagnosisReportVersion(r.Context(), workspace, actor,
		chi.URLParam(r, "reportId"), versionNo, time.Now())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}

// ---------------------------------------------------------------- work marks

// ListContentOpDiagWorkMarks answers the current marks, by work_id or by
// account_id.
func (h *Handler) ListContentOpDiagWorkMarks(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	marks, err := h.opdiagStore().ListWorkMarks(r.Context(), workspace, actor, feedbacklearning.WorkMarkFilter{
		WorkID: query.Get("work_id"), AccountID: query.Get("account_id"),
	})
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"marks": marks})
}

// RecordContentOpDiagWorkMark appends one mark.
func (h *Handler) RecordContentOpDiagWorkMark(w http.ResponseWriter, r *http.Request) {
	h.recordOpdiagWorkMark(w, r, h.opdiagStore)
}

func (h *Handler) recordOpdiagWorkMark(w http.ResponseWriter, r *http.Request, store func() *feedbacklearning.DiagnosisStore) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body opdiagMarkBody
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	mark, err := store().RecordWorkMark(r.Context(), workspace, actor, body.WorkMarkInput)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mark)
}
