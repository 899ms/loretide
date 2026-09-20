// Package feedbacklearning owns the manual half of SOP 10.1: numbers a person
// copied off a platform, and things people said about the piece.
//
// Four things this package deliberately cannot do, each with a test:
//
//   - It makes no outbound request, holds no platform credential and calls no
//     model. Every number here was typed in by a person; the system never went
//     and looked.
//   - It aggregates nothing. No sum, no average, no ranking, no scoring. That
//     is 10.2's business, and this card only records facts.
//   - It never merges "read" and "play". SOP 10.1: "不同平台的阅读和播放分别
//     保留，不直接合并排名".
//   - It redacts nothing. 10.1 supports a person redacting an excerpt; it does
//     not do it for them.
//
// Contract: specs/027-feedback-manual/contracts/feedback-manual.md
package feedbacklearning

import (
	"errors"
	"time"
	"unicode/utf8"
)

var (
	// ErrInvalid is unusable input: a value outside a controlled set, a
	// required field left out, an over-long note. Answered as 400 with a
	// diagnostic error object naming the field.
	ErrInvalid = errors.New("invalid feedback input")
	// ErrNotFound is "no such publication record here". Answered identically
	// to a refusal, so a caller cannot learn from it whether the thing exists
	// in another workspace.
	ErrNotFound = errors.New("publication record not found")
	// ErrStorage is anything the database said. Never carries its text.
	ErrStorage = errors.New("feedback storage unavailable")
)

// FieldError names the field that was wrong, and for a batch, the row.
//
// A bare ErrInvalid could only ever produce "参数错误", which tells the person
// filling the form nothing. For an import it would be worse: "some row is
// wrong" in a paste of forty rows is not a usable answer.
type FieldError struct {
	Field  string
	Reason string
	// Row is 1-based and 0 when the input was not a batch.
	Row int
}

func (e FieldError) Error() string {
	if e.Row > 0 {
		return e.Reason + ": row " + itoa(e.Row) + " " + e.Field
	}
	return e.Reason + ": " + e.Field
}
func (e FieldError) Unwrap() error { return ErrInvalid }

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func invalidField(field string) error {
	return FieldError{Field: field, Reason: "invalid or missing"}
}

func invalidRowField(row int, field string) error {
	return FieldError{Field: field, Reason: "invalid or missing", Row: row}
}

// Platform is the same four channels 025 delivers to. Defined here rather than
// imported: feedback-learning's declared dependencies are workspace-core,
// review-delivery and diagnostics, and ip-profile is not among them. A test
// reads ip-profile's Go source and compares, so the two cannot drift.
type Platform string

const (
	PlatformXiaohongshu Platform = "xiaohongshu"
	PlatformWechatMP    Platform = "wechat_mp"
	PlatformDouyin      Platform = "douyin"
	PlatformShipinhao   Platform = "shipinhao"
)

var Platforms = []Platform{
	PlatformXiaohongshu, PlatformWechatMP, PlatformDouyin, PlatformShipinhao,
}

// Metric is SOP 10.1's eleven names, exactly: 曝光、阅读、播放、完播、点赞、
// 评论、收藏、分享、关注、私信、转化.
//
// There is no 'other'. The SOP named these and stopped; an escape hatch would
// turn a controlled set into a suggestion, and the first thing anybody would
// put in it is a name that belongs in the list.
//
// MetricRead and MetricPlay are separate and stay separate. 10.1: "不同平台的
// 阅读和播放分别保留，不直接合并排名". Merging them looks like saving the reader
// trouble; it actually puts two incomparable platforms into one ranking.
type Metric string

const (
	MetricImpression    Metric = "impression"
	MetricRead          Metric = "read"
	MetricPlay          Metric = "play"
	MetricCompletion    Metric = "completion"
	MetricLike          Metric = "like"
	MetricComment       Metric = "comment"
	MetricFavorite      Metric = "favorite"
	MetricShare         Metric = "share"
	MetricFollow        Metric = "follow"
	MetricDirectMessage Metric = "direct_message"
	MetricConversion    Metric = "conversion"
)

var Metrics = []Metric{
	MetricImpression, MetricRead, MetricPlay, MetricCompletion, MetricLike,
	MetricComment, MetricFavorite, MetricShare, MetricFollow,
	MetricDirectMessage, MetricConversion,
}

// MetricSource is 10.1's "数据来源类型". Written by the server from which
// entry point was used, never read from a request body: a caller that could
// declare where its data came from is not reporting a source.
type MetricSource string

const (
	SourceManual    MetricSource = "manual"
	SourceCSVImport MetricSource = "csv_import"
)

var MetricSources = []MetricSource{SourceManual, SourceCSVImport}

// ExcerptSource is 10.1's three: 评论、私信、线索. No 'other', for the same
// reason Metrics has none.
type ExcerptSource string

const (
	ExcerptComment        ExcerptSource = "comment"
	ExcerptPrivateMessage ExcerptSource = "private_message"
	ExcerptLead           ExcerptSource = "lead"
)

var ExcerptSources = []ExcerptSource{ExcerptComment, ExcerptPrivateMessage, ExcerptLead}

// ReviewState is SOP 7.1's "AI 复盘" row, all seven values.
//
// This card produces exactly one of them, StatePendingData - there is no
// executor, so there is no report and nothing to queue. The other six are
// reserved so that EP-08 does not have to widen the set, which would mean
// revalidating every stored value; a test asserts nothing here can produce
// them.
//
// There is no report table. This phase cannot produce a single line of one,
// and a permanently empty table reads as "it will be filled in eventually".
// The state is derived: no report means pending_data.
type ReviewState string

const (
	// StatePendingData is the only value this card produces: there is not
	// enough to review yet, and no runner to review it with.
	StatePendingData ReviewState = "pending_data"
	StateQueued      ReviewState = "queued"
	StateGenerating  ReviewState = "generating"
	StateGenerated   ReviewState = "generated"
	StateFailed      ReviewState = "failed"
	StateEdited      ReviewState = "edited"
	StateSuperseded  ReviewState = "superseded"
)

var ReviewStates = []ReviewState{
	StatePendingData, StateQueued, StateGenerating, StateGenerated,
	StateFailed, StateEdited, StateSuperseded,
}

// There are exactly five controlled sets in this package: Platforms, Metrics,
// MetricSources, ExcerptSources and ReviewStates.
//
// unit, stat_window, evidence_note, redacted_excerpt, interpretation and tags
// are free text. SOP 10.1 names those fields but gives no values for them, and
// this card does not invent any - the same rule 025 settled on. A test asserts
// no sixth set appears here later.

// MaxNoteRunes bounds a free text note. Counted in runes for the same reason
// the persona prompt is: a byte limit gives a Chinese excerpt a third of the
// room an English one gets.
const MaxNoteRunes = 20000

// MaxShortRunes bounds a unit, a window description or a tag.
const MaxShortRunes = 500

// MaxTags bounds how many tags one excerpt carries.
const MaxTags = 50

// ManualMetric is one observation a person copied down.
//
// Value is a POINTER, and that is load-bearing. SOP 10.1: "未知填空；0 只表示
// 已确认的零值". nil is "the platform does not show me this"; a pointer to 0 is
// "I checked, it is zero". Anything that collapses the two puts invented data
// into every later aggregate with nothing to report it.
type ManualMetric struct {
	ManualMetricID      string   `json:"manual_metric_id"`
	WorkspaceID         string   `json:"workspace_id"`
	PublicationRecordID string   `json:"publication_record_id"`
	Platform            Platform `json:"platform"`
	AccountID           string   `json:"account_id"`
	Metric              Metric   `json:"metric"`
	Value               *int64   `json:"value"`
	Unit                string   `json:"unit"`
	// StatWindow is 10.1's 统计窗口. Free text: "发布后 14 天累计" and "上线首日"
	// cannot be expressed as a start and an end, and forcing two timestamps
	// would make someone invent a boundary they do not know.
	StatWindow   string    `json:"stat_window"`
	SampledAt    time.Time `json:"sampled_at"`
	RecordedBy   string    `json:"recorded_by"`
	EvidenceNote string    `json:"evidence_note"`
	// SourceType is set by the server from the entry point used.
	SourceType MetricSource `json:"source_type"`
	CreatedAt  time.Time    `json:"created_at"`

	// VersionID is resolved on READ, two hops through the delivery task and
	// the review request. "" means it could not be resolved - a history entry
	// with no delivery task, for one - and that is never a reason to refuse
	// the recording. Callers render it the way 025 renders version_match's
	// unknown.
	VersionID string `json:"version_id"`
}

// FeedbackExcerpt is one thing a person said, plus what the operator made of
// it. Two separate fields, deliberately.
type FeedbackExcerpt struct {
	FeedbackExcerptID   string        `json:"feedback_excerpt_id"`
	WorkspaceID         string        `json:"workspace_id"`
	PublicationRecordID string        `json:"publication_record_id"`
	SourceType          ExcerptSource `json:"source_type"`
	// RedactedExcerpt is what somebody else said, already redacted BY A PERSON.
	RedactedExcerpt string `json:"redacted_excerpt"`
	// Interpretation is what the operator makes of it. Kept apart so that a
	// later reader can still tell the evidence from the judgement.
	Interpretation string    `json:"interpretation"`
	Tags           []string  `json:"tags"`
	OccurredAt     time.Time `json:"occurred_at"`
	RecordedBy     string    `json:"recorded_by"`
	CreatedAt      time.Time `json:"created_at"`
}

// PendingRegistration is one publication record nobody has recorded numbers
// for. It is derived on read and stored nowhere.
type PendingRegistration struct {
	PublicationRecordID string    `json:"publication_record_id"`
	WorkID              string    `json:"work_id"`
	ArtifactID          string    `json:"artifact_id"`
	Channel             string    `json:"channel"`
	Status              string    `json:"status"`
	PublishedAt         *string   `json:"published_at"`
	CreatedAt           time.Time `json:"created_at"`
	// Due is workspace-core's three-valued answer (specs/029): passed, or
	// unknown because the brand set no window or the record carries no
	// publication time. not_yet never appears - a record in that state is not
	// on this list at all.
	Due string `json:"due"`
}

// oneOf accepts an exact match only: no trimming, no case folding. Tolerating
// "Read" beside "read" would give one value two spellings in storage, and
// every reader would then have to know both.
func oneOf[T ~string](value string, allowed []T) bool {
	for _, candidate := range allowed {
		if string(candidate) == value {
			return true
		}
	}
	return false
}

func ValidatePlatform(value string) error {
	if !oneOf(value, Platforms) {
		return invalidField("platform")
	}
	return nil
}

func ValidateMetric(value string) error {
	if !oneOf(value, Metrics) {
		return invalidField("metric")
	}
	return nil
}

func ValidateMetricSource(value string) error {
	if !oneOf(value, MetricSources) {
		return invalidField("source_type")
	}
	return nil
}

func ValidateExcerptSource(value string) error {
	if !oneOf(value, ExcerptSources) {
		return invalidField("source_type")
	}
	return nil
}

func ValidateReviewState(value string) error {
	if !oneOf(value, ReviewStates) {
		return invalidField("state")
	}
	return nil
}

// ValidateNote bounds a free text field. Empty is legitimate: an excerpt with
// no interpretation yet, or an interpretation of something not worth quoting,
// are both ordinary.
func ValidateNote(field, value string) error {
	if utf8.RuneCountInString(value) > MaxNoteRunes {
		return FieldError{Field: field, Reason: "too long"}
	}
	return nil
}

func ValidateShort(field, value string) error {
	if utf8.RuneCountInString(value) > MaxShortRunes {
		return FieldError{Field: field, Reason: "too long"}
	}
	return nil
}
