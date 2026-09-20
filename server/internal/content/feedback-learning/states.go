package feedbacklearning

import (
	"time"

	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// The card's rules, away from the store and the handler.
//
// Everything here is a decision that would otherwise sit inside a write path
// or inside JSX: whether a row is usable, whether a batch may land, whether a
// publication record still needs numbers. Pure functions, so each has one
// place to be true and one place to be tested.
//
// Contract: specs/027-feedback-manual/contracts/feedback-manual.md

// MetricInput is one row on its way in, before it becomes a ManualMetric.
//
// SourceType is absent on purpose: the server sets it from the entry point.
// RecordedBy is absent for the same reason - it comes from the session.
type MetricInput struct {
	PublicationRecordID string   `json:"publication_record_id"`
	Platform            Platform `json:"platform"`
	AccountID           string   `json:"account_id"`
	Metric              Metric   `json:"metric"`
	// Value nil means "the platform does not show me this". A pointer to 0
	// means "I looked, it is zero". The two must survive all the way to the
	// column, and this is the first place they could be collapsed.
	Value        *int64 `json:"value"`
	Unit         string `json:"unit"`
	StatWindow   string `json:"stat_window"`
	SampledAt    string `json:"sampled_at"`
	EvidenceNote string `json:"evidence_note"`
}

// ValidateMetricInput checks one row. row is 1-based for a batch and 0 for a
// single form submission, so the error can say which line was wrong.
func ValidateMetricInput(input MetricInput, row int) error {
	field := func(name string) error {
		if row > 0 {
			return invalidRowField(row, name)
		}
		return invalidField(name)
	}
	if input.PublicationRecordID == "" {
		return field("publication_record_id")
	}
	if !oneOf(string(input.Platform), Platforms) {
		return field("platform")
	}
	if !oneOf(string(input.Metric), Metrics) {
		return field("metric")
	}
	if input.SampledAt == "" {
		return field("sampled_at")
	}
	if _, err := time.Parse(time.RFC3339, input.SampledAt); err != nil {
		return FieldError{Field: "sampled_at", Reason: "not an RFC 3339 timestamp", Row: row}
	}
	for name, value := range map[string]string{
		"unit": input.Unit, "stat_window": input.StatWindow, "account_id": input.AccountID,
	} {
		if err := ValidateShort(name, value); err != nil {
			return FieldError{Field: name, Reason: "too long", Row: row}
		}
	}
	if err := ValidateNote("evidence_note", input.EvidenceNote); err != nil {
		return FieldError{Field: "evidence_note", Reason: "too long", Row: row}
	}
	return nil
}

// ValidateMetricBatch checks every row and reports the FIRST bad one.
//
// All or nothing: the caller writes none of them unless every row passes. A
// partial write leaves the person believing all forty rows landed, and "how
// many got in" is the only question this feature has to answer.
func ValidateMetricBatch(rows []MetricInput) error {
	if len(rows) == 0 {
		return invalidField("metrics")
	}
	for index, row := range rows {
		if err := ValidateMetricInput(row, index+1); err != nil {
			return err
		}
	}
	return nil
}

// ExcerptInput is one excerpt on its way in.
type ExcerptInput struct {
	PublicationRecordID string        `json:"publication_record_id"`
	SourceType          ExcerptSource `json:"source_type"`
	RedactedExcerpt     string        `json:"redacted_excerpt"`
	Interpretation      string        `json:"interpretation"`
	Tags                []string      `json:"tags"`
	OccurredAt          string        `json:"occurred_at"`
}

// ValidateExcerptInput checks one excerpt.
//
// Both text fields may be empty on their own - an excerpt nobody has an
// opinion about yet, and an observation not worth quoting anyone for, are both
// ordinary - but at least one has to say something, or the row records nothing.
func ValidateExcerptInput(input ExcerptInput) error {
	if input.PublicationRecordID == "" {
		return invalidField("publication_record_id")
	}
	if !oneOf(string(input.SourceType), ExcerptSources) {
		return invalidField("source_type")
	}
	if input.OccurredAt == "" {
		return invalidField("occurred_at")
	}
	if _, err := time.Parse(time.RFC3339, input.OccurredAt); err != nil {
		return FieldError{Field: "occurred_at", Reason: "not an RFC 3339 timestamp"}
	}
	if input.RedactedExcerpt == "" && input.Interpretation == "" {
		return invalidField("redacted_excerpt")
	}
	if err := ValidateNote("redacted_excerpt", input.RedactedExcerpt); err != nil {
		return err
	}
	if err := ValidateNote("interpretation", input.Interpretation); err != nil {
		return err
	}
	if len(input.Tags) > MaxTags {
		return FieldError{Field: "tags", Reason: "too many"}
	}
	for _, tag := range input.Tags {
		if err := ValidateShort("tags", tag); err != nil {
			return err
		}
	}
	return nil
}

// NeedsRegistration reports whether a publication record is still waiting for
// its numbers.
//
// Two conditions that have always been here - the piece went out, and nobody
// has recorded a single metric for it - and a third that arrived with
// specs/029: the brand's observation window has elapsed.
//
// The window comes from workspace-core, which reads it out of the brand's
// settings. Nothing here decides how long to wait. The comment this replaces
// said a hard-coded number of days "would be a rule the SOP never stated,
// sitting in code where no operator can see or change it" - that is still
// true, and it is why `due` is a parameter rather than a constant.
//
// DueUnknown takes the SAME branch as DuePassed, and that is the whole of this
// function worth reading twice. Not knowing whether the window has elapsed is
// not a reason to hide a record: a brand that has set no window, and a
// publication record that carries no publication time (025 allows that), are
// exactly the pieces somebody should go and look at. Treating unknown as
// "not yet" would make them leave the workbench silently, which is the one
// failure mode nobody would notice.
//
// failed / removed / unknown records are not waiting for anything: there are
// no real results to copy down for a piece that did not go out.
func NeedsRegistration(publicationStatus string, metricCount int, due workspacecore.Due) bool {
	if metricCount > 0 {
		return false
	}
	if due == workspacecore.DueNotYet {
		return false
	}
	return publicationStatus == "reported_published" || publicationStatus == "verified_published"
}

// PublishedStatuses are the two NeedsRegistration accepts, exported so the
// query that fetches candidates and the predicate cannot drift apart.
var PublishedStatuses = []string{"reported_published", "verified_published"}

// ReviewStateFor is the whole of this card's AI review: there is no report, so
// there is not enough to review.
//
// A function rather than a constant so that the day EP-08 lands, the callers
// already ask instead of assuming. It can only ever return StatePendingData
// here, and a guard test asserts no other value is produced anywhere in this
// package.
func ReviewStateFor(reportCount int) ReviewState {
	if reportCount > 0 {
		// Unreachable in this phase: nothing can write a report. Kept so the
		// shape of the question is right when something can.
		return StateGenerated
	}
	return StatePendingData
}

// SameValue compares two metric values the way storage must.
//
// It exists so "empty is not zero" has a function to point at: nil equals nil,
// 0 equals 0, and nil never equals 0. Written out because the obvious
// `a == b` on two pointers compares addresses, and the obvious deref crashes.
func SameValue(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// DescribeValue renders a metric value for a caller that needs text.
//
// An absent value is NOT "0" and not "". It is unknown, and it says so, because
// a blank cell in a report gets read as zero by the next person along.
func DescribeValue(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return itoa64(*value)
}

func itoa64(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
