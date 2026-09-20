// Package workeditor owns a work container, its documents, and the append-only
// version history of each document.
//
// This slice is the one SOP 6.2 requires to exist with no model at all: write,
// edit and keep a history by hand. Nothing here calls a model, and
// constitution IX keeps real executors disabled - `generated` is in the source
// set so that EP-08 does not have to change the controlled set to land, and a
// test asserts nothing in this package can produce it.
//
// Contract: specs/024-work-editor-manual/contracts/work-and-versions.md
package workeditor

import (
	"errors"
	"time"
	"unicode/utf8"
)

var (
	// ErrInvalid is unusable input: a value outside a controlled set, an
	// over-long body. Answered as 400 with a diagnostic error object.
	ErrInvalid = errors.New("invalid work editor input")
	// ErrNotFound is "no such work, document or version here". Answered
	// identically to a refusal, so a caller cannot learn from it whether the
	// thing exists in another workspace.
	ErrNotFound = errors.New("work, document or version not found")
	// ErrStorage is anything the database said. Never carries its text.
	ErrStorage = errors.New("work editor storage unavailable")
	// ErrConflict is two writers racing for one revision number. The loser
	// retries; it is not the caller's mistake.
	ErrConflict = errors.New("version number taken")
)

// Kind is what sort of document this is. The Go enum is authoritative and
// produces a 400; the CHECK in migration 497 is the backstop for anything that
// reaches the database another way - the split migration 477 already uses.
type Kind string

const (
	// KindBody is the piece itself.
	KindBody Kind = "body"
	// KindChannelDraft is a cut of it for one channel.
	KindChannelDraft Kind = "channel_draft"
)

// Kinds is the controlled set. Adding one is a migration, deliberately: the
// set of things a work can contain should change in a reviewable edit.
var Kinds = []Kind{KindBody, KindChannelDraft}

// DraftStatus is SOP 7.1's pair for the editing copy, and only that pair.
//
// The row's other states - drafting, in_review, approved, handed_off,
// published - come from other rows of 7.1 and belong to review-delivery and
// the handover card. "不用一个完成覆盖所有行为" is 7.1's own sentence, so there
// is no `done` here either.
type DraftStatus string

const (
	// DraftWorking means there are changes not yet saved as a version.
	DraftWorking DraftStatus = "working"
	// DraftSaved means the editing copy is byte-for-byte the latest version.
	DraftSaved DraftStatus = "saved"
)

var DraftStatuses = []DraftStatus{DraftWorking, DraftSaved}

// Source is where a version's content came from. Exactly three, per SOP 7.1.
type Source string

const (
	// SourceGenerated is a model's output. Nothing in this phase can produce
	// it; it is here so EP-08 does not have to widen the set, which would mean
	// revalidating every stored row.
	SourceGenerated Source = "generated"
	// SourceEdited is content a person wrote. A restore is still this: what
	// comes back is what the person wrote, just fetched from an older version.
	SourceEdited Source = "edited"
	// SourceAdopted is a version taken as the baseline for the next step.
	SourceAdopted Source = "adopted"
)

var Sources = []Source{SourceGenerated, SourceEdited, SourceAdopted}

// Action is what was done, recorded beside the source rather than folded into
// it. SOP 7.1 asks for "来源与动作记录" - two facts. Folding them would force a
// reader to consult the other column to learn what happened, and would make
// "restored" look like an answer to "who wrote this", which it is not.
type Action string

const (
	// ActionSaved is an ordinary save of the editing copy.
	ActionSaved Action = "saved"
	// ActionRestored is a save whose content came from an older version.
	ActionRestored Action = "restored"
	// ActionAdopted is taking a version as the next step's baseline.
	ActionAdopted Action = "adopted"
)

var Actions = []Action{ActionSaved, ActionRestored, ActionAdopted}

// MaxBodyRunes bounds one document. Counted in runes for the same reason the
// persona prompt is: a byte limit gives a Chinese draft a third of the room an
// English one gets.
const MaxBodyRunes = 200000

// MaxTitleRunes bounds a work or document title.
const MaxTitleRunes = 500

// Work is the container for everything produced from one topic card.
//
// It has no status. SOP 7.1's document-editing row gives two states and they
// belong to a document's editing copy; the rest are other rows' business.
type Work struct {
	WorkID      string `json:"work_id"`
	WorkspaceID string `json:"workspace_id"`
	TopicCardID string `json:"topic_card_id"`
	// SnapshotID is "" when the work was not started from a snapshot. A real
	// state: SOP 6.2 requires writing to work with nothing else in place.
	SnapshotID string    `json:"snapshot_id"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Artifact is one document, with its mutable editing copy.
type Artifact struct {
	ArtifactID  string `json:"artifact_id"`
	WorkID      string `json:"work_id"`
	WorkspaceID string `json:"workspace_id"`
	Kind        Kind   `json:"kind"`
	Title       string `json:"title"`
	Position    int64  `json:"position"`
	DraftBody   string `json:"draft_body"`
	// DraftStatus is stored rather than recomputed on read: recomputing means
	// fetching the latest version's full body, N of them when listing a work's
	// documents. The cost is that two truths can disagree, which is why every
	// path that writes it is asserted against DraftStatusFor.
	DraftStatus  DraftStatus `json:"draft_status"`
	DraftSavedAt time.Time   `json:"draft_saved_at"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// ArtifactVersion is one saved version. Append-only.
type ArtifactVersion struct {
	VersionID   string `json:"version_id"`
	ArtifactID  string `json:"artifact_id"`
	WorkID      string `json:"work_id"`
	WorkspaceID string `json:"workspace_id"`
	// Revision is a per-document counter for people to read and to order by.
	// Not the cross-table key: version 3 means a different thing for every
	// document. VersionID is what anything else references.
	Revision int64  `json:"revision"`
	Source   Source `json:"source"`
	Action   Action `json:"action"`
	Body     string `json:"body"`
	// RestoredFrom and AdoptedFrom are "" unless the matching action produced
	// this version. Most versions came from neither.
	RestoredFrom string    `json:"restored_from"`
	AdoptedFrom  string    `json:"adopted_from"`
	ActorID      string    `json:"actor_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// oneOf accepts an exact match only: no trimming, no case folding. Tolerating
// "Body" beside "body" would give one value two spellings in storage, and
// every reader would then have to know both.
func oneOf[T ~string](value string, allowed []T) bool {
	for _, candidate := range allowed {
		if string(candidate) == value {
			return true
		}
	}
	return false
}

func ValidateKind(value string) error {
	if !oneOf(value, Kinds) {
		return ErrInvalid
	}
	return nil
}

func ValidateDraftStatus(value string) error {
	if !oneOf(value, DraftStatuses) {
		return ErrInvalid
	}
	return nil
}

func ValidateSource(value string) error {
	if !oneOf(value, Sources) {
		return ErrInvalid
	}
	return nil
}

func ValidateAction(value string) error {
	if !oneOf(value, Actions) {
		return ErrInvalid
	}
	return nil
}

// ValidateBody bounds a document. Empty is legitimate - SOP 3.1's reading that
// a blank answer is pending rather than wrong applies here too.
func ValidateBody(value string) error {
	if utf8.RuneCountInString(value) > MaxBodyRunes {
		return ErrInvalid
	}
	return nil
}

func ValidateTitle(value string) error {
	if utf8.RuneCountInString(value) > MaxTitleRunes {
		return ErrInvalid
	}
	return nil
}

// DraftStatusFor is the definition of DraftStatus, as a function.
//
// It exists so the stored value has something to be checked against: every
// path that writes DraftStatus is asserted to agree with this. Without it,
// "saved" would be whatever the last person to touch the code thought it meant.
func DraftStatusFor(draftBody string, latest *ArtifactVersion) DraftStatus {
	if latest == nil {
		// No version yet. An untouched document is saved - there is nothing
		// unsaved about it - and one with typing in it is not.
		if draftBody == "" {
			return DraftSaved
		}
		return DraftWorking
	}
	if draftBody == latest.Body {
		return DraftSaved
	}
	return DraftWorking
}
