// Package sourceinbox owns the manual half of SOP §4: taking something a person
// pasted or linked, keeping it with the facts about how it was collected, and
// letting them organise it later.
//
// Four things this package deliberately cannot do, each with a test:
//
//   - It makes no outbound request of any kind. A url source stores the link
//     and whatever the person typed about it; nothing here fetches the page
//     (spec FR-002).
//   - It calls no model. §4 step 4's "建议摘要、实体、观点、问题与关联资料" is
//     the parser's column, and there is no parser here.
//   - It never merges or deletes a duplicate. §4 says "重复素材先提示合并关联"
//     and R-011 says "内容相同不删除独立的收藏上下文与批注" - so this offers the
//     hint and stops there.
//   - It has no parse-status column. SOP §7.1 lists pending → processing →
//     ready / partial / failed; nothing here would advance it, and a column
//     frozen at one value is a decoration that reads as a claim.
//
// Contract: specs/028-source-inbox-manual/contracts/source-inbox.md
package sourceinbox

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrInvalid is unusable input: a value outside a controlled set, a
	// required field left out, an immutable column someone tried to change.
	// Answered as 400 with a diagnostic error object naming the field.
	ErrInvalid = errors.New("invalid source inbox input")
	// ErrNotFound is "no such source here". Answered identically to a refusal,
	// so a caller cannot learn from it whether the thing exists in another
	// workspace.
	ErrNotFound = errors.New("source not found")
	// ErrStorage is anything the database said. Never carries its text.
	ErrStorage = errors.New("source inbox storage unavailable")
)

// FieldError names the field that was wrong, so a 400 can say which one.
// "参数错误" tells the person filling the form nothing about what to fix.
type FieldError struct {
	Field  string
	Reason string
}

func (e FieldError) Error() string { return e.Reason + ": " + e.Field }
func (e FieldError) Unwrap() error { return ErrInvalid }

func invalidField(field string) error {
	return FieldError{Field: field, Reason: "invalid or missing"}
}

// Kind is what was collected. §4 step 1 lists pasting a link, a quick note,
// uploading a file and picking exported material; this card's inputs are the
// first two, and a quick note is a pasted text.
//
// Files and exports (W-03) are out of scope, so this set has two values and
// adding a third is a migration - the CHECK in 516 moves with it.
type Kind string

const (
	KindPastedText Kind = "pasted_text"
	KindURL        Kind = "url"
)

var Kinds = []Kind{KindPastedText, KindURL}

// Status is where an item is in being organised. It is NOT SOP §7.1's parse
// status: that one describes what a parser did to the material, this one
// describes what a person decided about it. §7.1 itself says "解析状态与知识是
// 否确认分开".
type Status string

const (
	StatusInbox     Status = "inbox"
	StatusOrganized Status = "organized"
	StatusArchived  Status = "archived"
)

var Statuses = []Status{StatusInbox, StatusOrganized, StatusArchived}

// MaxContentRunes bounds a pasted body. Over it the save is refused, never
// truncated: the hash is computed over what is stored, so truncating would
// silently give a hash to a body nobody meant to save - and that hash is the
// only thing duplicate detection has to go on.
const MaxContentRunes = 200000

const MaxFieldRunes = 4000

// Source is one act of collecting. The first five fields describe the event
// and never change; the rest are what organising touches.
type Source struct {
	SourceID          string    `json:"source_id"`
	WorkspaceID       string    `json:"workspace_id"`
	Kind              Kind      `json:"kind"`
	URL               string    `json:"url"`
	CapturedAt        time.Time `json:"captured_at"`
	RecordedBy        string    `json:"recorded_by"`
	HistoricalImport  bool      `json:"historical_import"`
	Title             string    `json:"title"`
	Tags              []string  `json:"tags"`
	Annotation        string    `json:"annotation"`
	PersonalJudgement string    `json:"personal_judgement"`
	Status            Status    `json:"status"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Snapshot is the material itself plus its hash. Append-only.
type Snapshot struct {
	SnapshotID  string    `json:"snapshot_id"`
	WorkspaceID string    `json:"workspace_id"`
	SourceID    string    `json:"source_id"`
	Content     string    `json:"content"`
	ContentHash string    `json:"content_hash"`
	CapturedAt  time.Time `json:"captured_at"`
}

// Revision is one act of organising. It names the fields that changed rather
// than copying the row.
type Revision struct {
	RevisionID    string    `json:"revision_id"`
	WorkspaceID   string    `json:"workspace_id"`
	SourceID      string    `json:"source_id"`
	ChangedFields []string  `json:"changed_fields"`
	ActorID       string    `json:"actor_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// NewSource is what a caller may set when collecting. Note what is absent:
// there is no status (everything starts in the inbox) and no way to set
// captured_at or recorded_by, which the store fills from the request.
type NewSource struct {
	Kind              Kind
	URL               string
	Content           string
	Title             string
	Tags              []string
	Annotation        string
	PersonalJudgement string
	HistoricalImport  bool
}

// Organize is the mutable half, as a patch. A nil field is "leave it".
type Organize struct {
	Title             *string
	Tags              *[]string
	Annotation        *string
	PersonalJudgement *string
	Status            *Status
}

// ContentHash is what duplicate detection compares. sha256 over the exact
// bytes stored, hex encoded.
//
// No normalisation - not trimming, not case folding, not collapsing
// whitespace. Two bodies that differ by a trailing newline are two different
// texts, and deciding they are "the same" is exactly the merge judgement §4
// leaves to a person.
func ContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// oneOf accepts an exact match only: no trimming, no case folding. Tolerating
// "URL" beside "url" would give one value two spellings in storage, and every
// reader would then have to know both.
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
		return invalidField("kind")
	}
	return nil
}

func ValidateStatus(value string) error {
	if !oneOf(value, Statuses) {
		return invalidField("status")
	}
	return nil
}

// ValidateNew checks one collection request as a whole, because the rules are
// about combinations: a url without a link and a pasted text with one are both
// wrong, and neither field can tell on its own.
func ValidateNew(input NewSource) error {
	if err := ValidateKind(string(input.Kind)); err != nil {
		return err
	}
	switch input.Kind {
	case KindURL:
		if err := validateURL(input.URL); err != nil {
			return err
		}
		// A url source has no body here: there is nothing to snapshot until
		// something fetches the page, and this card fetches nothing. Accepting
		// a body would store text the link does not vouch for.
		if input.Content != "" {
			return invalidField("content")
		}
	case KindPastedText:
		if strings.TrimSpace(input.Content) == "" {
			return invalidField("content")
		}
		if utf8.RuneCountInString(input.Content) > MaxContentRunes {
			return FieldError{Field: "content", Reason: "too long"}
		}
		if input.URL != "" {
			return invalidField("url")
		}
	}
	return validateFields(input.Title, input.Annotation, input.PersonalJudgement, input.Tags)
}

func validateURL(value string) error {
	if value == "" {
		return invalidField("url")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return invalidField("url")
	}
	// http and https only. A javascript: or file: link is not a source a
	// person can go back and read, and storing one invites a page to follow it.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return invalidField("url")
	}
	return nil
}

func validateFields(title, annotation, judgement string, tags []string) error {
	for field, value := range map[string]string{
		"title": title, "annotation": annotation, "personal_judgement": judgement,
	} {
		if utf8.RuneCountInString(value) > MaxFieldRunes {
			return FieldError{Field: field, Reason: "too long"}
		}
	}
	for _, tag := range tags {
		if strings.TrimSpace(tag) == "" || utf8.RuneCountInString(tag) > 100 {
			return invalidField("tags")
		}
	}
	return nil
}

// ValidateOrganize checks a patch. Only the mutable half can appear here; the
// handler rejects an immutable column before this is reached, and the store's
// UPDATE never names one.
func ValidateOrganize(patch Organize) error {
	if patch.Status != nil {
		if err := ValidateStatus(string(*patch.Status)); err != nil {
			return err
		}
	}
	title, annotation, judgement := "", "", ""
	if patch.Title != nil {
		title = *patch.Title
	}
	if patch.Annotation != nil {
		annotation = *patch.Annotation
	}
	if patch.PersonalJudgement != nil {
		judgement = *patch.PersonalJudgement
	}
	var tags []string
	if patch.Tags != nil {
		tags = *patch.Tags
	}
	return validateFields(title, annotation, judgement, tags)
}

// ChangedFields names what a patch touches, in a fixed order so a test can
// compare without sorting.
func ChangedFields(patch Organize) []string {
	fields := []string{}
	if patch.Title != nil {
		fields = append(fields, "title")
	}
	if patch.Tags != nil {
		fields = append(fields, "tags")
	}
	if patch.Annotation != nil {
		fields = append(fields, "annotation")
	}
	if patch.PersonalJudgement != nil {
		fields = append(fields, "personal_judgement")
	}
	if patch.Status != nil {
		fields = append(fields, "status")
	}
	return fields
}
