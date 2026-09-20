// Package reviewdelivery owns the manual half of SOP 8-9.2: asking a person to
// review one frozen version, scheduling the handover of an approved one, and
// recording by hand what happened to it out on a platform.
//
// Three things this package deliberately cannot do, each with a test:
//
//   - It makes no outbound request of any kind, holds no platform credential
//     and offers no publish interface (SOP 9.2). "Publishing" here is a row a
//     person typed.
//   - It runs no scheduler. A delivery task's scheduled_at is a time a person
//     reads; SOP 9.1's "到期后产生站内待办" is a comparison made on read.
//   - It calls no model. SOP 8: "AI 的自检报告作为审核参考，不能执行人工通过动作".
//
// Contract: specs/025-review-delivery-manual/contracts/review-delivery.md
package reviewdelivery

import (
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"
)

var (
	// ErrInvalid is unusable input: a value outside a controlled set, a
	// required field left out, an illegal transition. Answered as 400 with a
	// diagnostic error object naming the field.
	ErrInvalid = errors.New("invalid review delivery input")
	// ErrNotFound is "no such request, task or document here". Answered
	// identically to a refusal, so a caller cannot learn from it whether the
	// thing exists in another workspace.
	ErrNotFound = errors.New("review request, delivery task or document not found")
	// ErrStorage is anything the database said. Never carries its text.
	ErrStorage = errors.New("review delivery storage unavailable")
)

// FieldError names the field that was wrong, so a 400 can say which one.
//
// SC-004 asks for three separate cases that each name the field they are
// missing; a bare ErrInvalid could only ever produce "参数错误", which tells the
// person filling the form nothing about what to fix.
type FieldError struct {
	Field  string
	Reason string
}

func (e FieldError) Error() string { return e.Reason + ": " + e.Field }
func (e FieldError) Unwrap() error { return ErrInvalid }

func invalidField(field string) error {
	return FieldError{Field: field, Reason: "invalid or missing"}
}

// Channel is SOP 8's first four channels. Adding one is a migration,
// deliberately: what a piece can be delivered to should change in a reviewable
// edit, and the CHECK in migration 504 has to move with it.
//
// This package does NOT import ip-profile - review-delivery's declared
// dependencies are workspace-core, source-inbox, work-editor and diagnostics.
// The list is kept honest by a test that reads ip-profile's Go source and
// compares, rather than by widening the module graph for one enum.
type Channel string

const (
	ChannelXiaohongshu Channel = "xiaohongshu"
	ChannelWechatMP    Channel = "wechat_mp"
	ChannelDouyin      Channel = "douyin"
	ChannelShipinhao   Channel = "shipinhao"
)

var Channels = []Channel{ChannelXiaohongshu, ChannelWechatMP, ChannelDouyin, ChannelShipinhao}

// ReviewStatus is SOP 7.1's review row, exactly.
type ReviewStatus string

const (
	ReviewPending          ReviewStatus = "pending"
	ReviewChangesRequested ReviewStatus = "changes_requested"
	ReviewApproved         ReviewStatus = "approved"
	ReviewRejected         ReviewStatus = "rejected"
	ReviewCancelled        ReviewStatus = "cancelled"
)

var ReviewStatuses = []ReviewStatus{
	ReviewPending, ReviewChangesRequested, ReviewApproved, ReviewRejected, ReviewCancelled,
}

// DeliveryStatus is SOP 7.1's delivery row, exactly.
type DeliveryStatus string

const (
	DeliveryDraft     DeliveryStatus = "draft"
	DeliveryReady     DeliveryStatus = "ready"
	DeliveryScheduled DeliveryStatus = "scheduled"
	DeliveryHandedOff DeliveryStatus = "handed_off"
	DeliveryCancelled DeliveryStatus = "cancelled"
	DeliveryHeld      DeliveryStatus = "held"
)

var DeliveryStatuses = []DeliveryStatus{
	DeliveryDraft, DeliveryReady, DeliveryScheduled,
	DeliveryHandedOff, DeliveryCancelled, DeliveryHeld,
}

// PublicationStatus is SOP 7.1's publication row, exactly. There is no state
// machine over it - see states.go.
type PublicationStatus string

const (
	PublicationReported PublicationStatus = "reported_published"
	PublicationVerified PublicationStatus = "verified_published"
	PublicationFailed   PublicationStatus = "failed"
	PublicationRemoved  PublicationStatus = "removed"
	PublicationUnknown  PublicationStatus = "unknown"
)

var PublicationStatuses = []PublicationStatus{
	PublicationReported, PublicationVerified, PublicationFailed,
	PublicationRemoved, PublicationUnknown,
}

// HandoffMethod is SOP 9.1's three actions. None of the three means published:
// "导出成功、复制完成或交接给他人都不自动等于发布成功".
type HandoffMethod string

const (
	HandoffExport     HandoffMethod = "export"
	HandoffCopy       HandoffMethod = "copy"
	HandoffToOperator HandoffMethod = "handed_to_operator"
)

var HandoffMethods = []HandoffMethod{HandoffExport, HandoffCopy, HandoffToOperator}

// VersionMatch is SOP 9.1's three states. unknown is named in the SOP itself:
// "无法拿到完整正文时将版本匹配标为 unknown". It is not a failure and must not
// block anything.
type VersionMatch string

const (
	VersionMatched VersionMatch = "matched"
	VersionDiffers VersionMatch = "differs"
	VersionUnknown VersionMatch = "unknown"
)

var VersionMatches = []VersionMatch{VersionMatched, VersionDiffers, VersionUnknown}

// SubjectKind is which of the two things a transition row belongs to.
type SubjectKind string

const (
	SubjectReviewRequest SubjectKind = "review_request"
	SubjectDeliveryTask  SubjectKind = "delivery_task"
)

var SubjectKinds = []SubjectKind{SubjectReviewRequest, SubjectDeliveryTask}

// There are exactly six controlled sets in this package: Channels, the three
// status sets, HandoffMethods, VersionMatches and SubjectKinds.
//
// There is deliberately NO controlled set for who declared a publication or for
// how it was verified. SOP 9.2 names neither, and inventing one would put words
// in the SOP's mouth that every stored row then has to be valid against. A test
// asserts no seventh set appears here later - the day someone reads two free
// text fields and decides "completing" them is helpful.

// MaxNoteRunes bounds a free text note. Counted in runes for the same reason
// the persona prompt is: a byte limit gives a Chinese note a third of the room
// an English one gets.
const MaxNoteRunes = 20000

// MaxRefRunes bounds an identifier-shaped field (a link, a content id, an
// account name).
const MaxRefRunes = 2000

// DeliverySnapshot is what submitting for review freezes. Exactly eight keys,
// each one a clause of SOP 8's "具体渠道、具体文档版本及附件、具体交付配置的快照".
//
// It is written once and never updated: no write path in this package names the
// snapshot column after the insert, and a test asserts it is byte-for-byte
// identical before and after a decision.
type DeliverySnapshot struct {
	Channel    Channel `json:"channel"`
	WorkID     string  `json:"work_id"`
	ArtifactID string  `json:"artifact_id"`
	VersionID  string  `json:"version_id"`
	AccountID  string  `json:"account_id"`
	// StartSnapshotID is "" when the work was not started from a snapshot. A
	// real state: SOP 6.2 requires writing to work with nothing else in place.
	StartSnapshotID string `json:"start_snapshot_id"`
	// Attachments is always an empty slice in this phase: attachment storage is
	// W-03 and there is nothing to reference yet. Empty, not nil - a nil slice
	// marshals to null and a consumer should not have to know which it got. A
	// negative test asserts no path can put anything in it.
	Attachments []string `json:"attachments"`
	// DeliveryConfig is SOP 8's "具体交付配置" as an extension object. It carries
	// the channel template identifier now; it is here rather than added later
	// because a key in a jsonb costs nothing today and a migration plus a
	// backfill tomorrow.
	DeliveryConfig map[string]string `json:"delivery_config"`
}

// SnapshotKeys is the exact key list, for the test that pins it. Stated once so
// that the assertion and the struct cannot drift apart silently.
var SnapshotKeys = []string{
	"channel", "work_id", "artifact_id", "version_id",
	"account_id", "start_snapshot_id", "attachments", "delivery_config",
}

// ReviewRequest is one "please look at this version".
type ReviewRequest struct {
	ReviewRequestID string  `json:"review_request_id"`
	WorkspaceID     string  `json:"workspace_id"`
	WorkID          string  `json:"work_id"`
	ArtifactID      string  `json:"artifact_id"`
	VersionID       string  `json:"version_id"`
	AccountID       string  `json:"account_id"`
	Channel         Channel `json:"channel"`
	// Snapshot is frozen at submit time. Nothing updates it.
	Snapshot     DeliverySnapshot `json:"snapshot"`
	Status       ReviewStatus     `json:"status"`
	RequestedBy  string           `json:"requested_by"`
	RequestedAt  time.Time        `json:"requested_at"`
	DecidedBy    string           `json:"decided_by"`
	DecidedAt    *time.Time       `json:"decided_at"`
	DecisionNote string           `json:"decision_note"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// Transition is one status change, of either subject. Append-only.
type Transition struct {
	TransitionID string      `json:"transition_id"`
	WorkspaceID  string      `json:"workspace_id"`
	SubjectKind  SubjectKind `json:"subject_kind"`
	SubjectID    string      `json:"subject_id"`
	// FromStatus is "" when the subject was created.
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Reason     string    `json:"reason"`
	ActorID    string    `json:"actor_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// DeliveryTask is one "hand this over" to-do. It is not a scheduler's input.
type DeliveryTask struct {
	DeliveryTaskID string `json:"delivery_task_id"`
	WorkspaceID    string `json:"workspace_id"`
	WorkID         string `json:"work_id"`
	ArtifactID     string `json:"artifact_id"`
	// ReviewRequestID may be "" while the task is a draft.
	ReviewRequestID string         `json:"review_request_id"`
	Channel         Channel        `json:"channel"`
	Status          DeliveryStatus `json:"status"`
	// ScheduledAt is read by people, never by code that acts on it.
	ScheduledAt   *time.Time    `json:"scheduled_at"`
	HandoffMethod HandoffMethod `json:"handoff_method"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`

	// Due and PendingRegistration are DERIVED on read and are not stored. See
	// states.go: storing either would be a second truth, and SOP 9.2's own
	// sentence is "不推断平台状态".
	Due                 bool `json:"due"`
	PendingRegistration bool `json:"pending_registration"`
}

// PublicationRecord is one person's statement about what happened on a
// platform. Append-only; the current status is the latest one.
type PublicationRecord struct {
	PublicationRecordID string            `json:"publication_record_id"`
	WorkspaceID         string            `json:"workspace_id"`
	WorkID              string            `json:"work_id"`
	ArtifactID          string            `json:"artifact_id"`
	DeliveryTaskID      string            `json:"delivery_task_id"`
	Channel             Channel           `json:"channel"`
	Status              PublicationStatus `json:"status"`
	// ActorID is taken from the session. The request body cannot set it.
	ActorID string `json:"actor_id"`
	// DeclaredBy is who SAID it, free text - often not the person who typed it.
	DeclaredBy         string       `json:"declared_by"`
	PageURLOrContentID string       `json:"page_url_or_content_id"`
	ReceiptNote        string       `json:"receipt_note"`
	VerificationNote   string       `json:"verification_note"`
	PublishedAt        *time.Time   `json:"published_at"`
	PlatformAccount    string       `json:"platform_account"`
	PlatformEdited     bool         `json:"platform_edited"`
	EditNote           string       `json:"edit_note"`
	VersionMatch       VersionMatch `json:"version_match"`
	CreatedAt          time.Time    `json:"created_at"`
}

// oneOf accepts an exact match only: no trimming, no case folding. Tolerating
// "Xiaohongshu" beside "xiaohongshu" would give one value two spellings in
// storage, and every reader would then have to know both.
func oneOf[T ~string](value string, allowed []T) bool {
	for _, candidate := range allowed {
		if string(candidate) == value {
			return true
		}
	}
	return false
}

func ValidateChannel(value string) error {
	if !oneOf(value, Channels) {
		return invalidField("channel")
	}
	return nil
}

func ValidateReviewStatus(value string) error {
	if !oneOf(value, ReviewStatuses) {
		return invalidField("status")
	}
	return nil
}

func ValidateDeliveryStatus(value string) error {
	if !oneOf(value, DeliveryStatuses) {
		return invalidField("status")
	}
	return nil
}

func ValidatePublicationStatus(value string) error {
	if !oneOf(value, PublicationStatuses) {
		return invalidField("status")
	}
	return nil
}

func ValidateHandoffMethod(value string) error {
	if !oneOf(value, HandoffMethods) {
		return invalidField("handoff_method")
	}
	return nil
}

func ValidateVersionMatch(value string) error {
	if !oneOf(value, VersionMatches) {
		return invalidField("version_match")
	}
	return nil
}

func ValidateSubjectKind(value string) error {
	if !oneOf(value, SubjectKinds) {
		return invalidField("subject_kind")
	}
	return nil
}

// ValidateNote bounds a free text field. Empty is legitimate everywhere the
// field is not conditionally required - which of them are is states.go's job,
// not this one's.
func ValidateNote(field, value string) error {
	if utf8.RuneCountInString(value) > MaxNoteRunes {
		return FieldError{Field: field, Reason: "too long"}
	}
	return nil
}

func ValidateRef(field, value string) error {
	if utf8.RuneCountInString(value) > MaxRefRunes {
		return FieldError{Field: field, Reason: "too long"}
	}
	return nil
}

// NewDeliverySnapshot builds the eight keys. It is the only way one is made, so
// that "exactly eight" has a single place to be true.
func NewDeliverySnapshot(channel Channel, workID, artifactID, versionID, accountID, startSnapshotID string) DeliverySnapshot {
	return DeliverySnapshot{
		Channel:    channel,
		WorkID:     workID,
		ArtifactID: artifactID,
		VersionID:  versionID,
		AccountID:  accountID,
		// "" when the work was not started from a snapshot. Real state.
		StartSnapshotID: startSnapshotID,
		// W-03 has not landed: there is no attachment to name, and no path
		// here can add one.
		Attachments:    []string{},
		DeliveryConfig: map[string]string{},
	}
}

// SnapshotKeysOf reports the keys a marshalled snapshot actually carries, for
// the test that pins the list at exactly eight.
func SnapshotKeysOf(snapshot DeliverySnapshot) ([]string, error) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	var generic map[string]json.RawMessage
	if err = json.Unmarshal(payload, &generic); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(generic))
	for key := range generic {
		keys = append(keys, key)
	}
	return keys, nil
}
