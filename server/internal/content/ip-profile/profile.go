package ipprofile

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// The account's expression profile: what SOP 3.1 asks the creator for, beyond
// the platform, the display name and the persona prompt.
//
// It travels on the same revision as the persona prompt, so one confirmation
// produces one snapshot and a run pins one revision_id. Every field carries its
// own status, because 3.1 is explicit that unconfirmed items stay pending -
// they are not errors, and they are not defaults either.
//
// Nothing here ever writes content the creator did not type. 3.1 names four
// things that must never be guessed - identity, experience, results and
// commercial promises - and an empty field stays empty.
//
// Contract: specs/021-account-expression-profile/contracts/expression-profile.md

// FieldStatus is whether the creator has confirmed this item.
type FieldStatus string

const (
	// FieldPending is "still to fill in". The default for everything.
	FieldPending FieldStatus = "pending"
	// FieldConfirmed means the creator looked at this value and said yes.
	FieldConfirmed FieldStatus = "confirmed"
)

// TextField is one free-text answer with its status.
type TextField struct {
	Value  string      `json:"value"`
	Status FieldStatus `json:"status"`
}

// ListField is a set of answers - channels, style samples.
type ListField struct {
	Values []string    `json:"values"`
	Status FieldStatus `json:"status"`
}

// HoursField is the weekly time budget, in hours.
//
// Hours rather than a free string: "a couple of evenings" cannot be compared
// against anything. Zero and "not filled in" are different states, which is why
// the status sits beside the number rather than being inferred from it.
type HoursField struct {
	Value  int         `json:"value"`
	Status FieldStatus `json:"status"`
}

// ExpressionProfile is the whole set. The field names follow SOP 3.1's list in
// its own order, so the two can be read side by side.
type ExpressionProfile struct {
	Audience             TextField  `json:"audience"`
	CommonQuestions      TextField  `json:"common_questions"`
	Experience           TextField  `json:"experience"`
	Positioning          TextField  `json:"positioning"`
	ContentPillars       TextField  `json:"content_pillars"`
	ExpressionStyle      TextField  `json:"expression_style"`
	ForbiddenExpressions TextField  `json:"forbidden_expressions"`
	ContentGoals         TextField  `json:"content_goals"`
	PrimaryChannels      ListField  `json:"primary_channels"`
	WeeklyHours          HoursField `json:"weekly_hours"`
	StyleSamples         ListField  `json:"style_samples"`
}

// MaxProfileFieldRunes bounds one text answer. Counted in runes for the same
// reason the persona prompt is: a byte limit gives a Chinese answer a third of
// the room an English one gets.
const MaxProfileFieldRunes = 20000

// MaxProfileListEntries bounds a list field. Generous enough that nobody
// legitimately hits it, small enough that a malformed client cannot store a
// megabyte of channels.
const MaxProfileListEntries = 64

// MaxWeeklyHours is a week. Anything above it is a typo, not a plan.
const MaxWeeklyHours = 168

var (
	// ErrProfile is an unusable profile: a status nothing recognises, a channel
	// outside the controlled set, an impossible time budget, an over-long
	// answer. Answered as 400 with a diagnostic error object.
	ErrProfile = errors.New("unusable expression profile")
)

// textFields returns every free-text field by pointer, so validation and
// normalisation do not have to list them twice and cannot fall out of step.
func (p *ExpressionProfile) textFields() []*TextField {
	return []*TextField{
		&p.Audience, &p.CommonQuestions, &p.Experience, &p.Positioning,
		&p.ContentPillars, &p.ExpressionStyle, &p.ForbiddenExpressions, &p.ContentGoals,
	}
}

func (p *ExpressionProfile) listFields() []*ListField {
	return []*ListField{&p.PrimaryChannels, &p.StyleSamples}
}

func validStatus(status FieldStatus) bool {
	return status == "" || status == FieldPending || status == FieldConfirmed
}

// validateProfileShape checks the caller's original structure before
// normalisation is allowed to lower statuses or discard blank list entries.
// Content normalisation is intentionally not part of this pass: a valid blank
// channel is removed later, but an invalid status or an oversized blank-only
// list must not disappear before it can be refused.
func validateProfileShape(profile ExpressionProfile) error {
	for _, field := range profile.textFields() {
		if !validStatus(field.Status) {
			return ErrProfile
		}
		if utf8.RuneCountInString(field.Value) > MaxProfileFieldRunes {
			return ErrProfile
		}
	}
	for _, field := range profile.listFields() {
		if !validStatus(field.Status) {
			return ErrProfile
		}
		if len(field.Values) > MaxProfileListEntries {
			return ErrProfile
		}
		for _, value := range field.Values {
			if utf8.RuneCountInString(value) > MaxProfileFieldRunes {
				return ErrProfile
			}
		}
	}
	if !validStatus(profile.WeeklyHours.Status) {
		return ErrProfile
	}
	if profile.WeeklyHours.Value < 0 || profile.WeeklyHours.Value > MaxWeeklyHours {
		return ErrProfile
	}
	return nil
}

// ValidateProfile refuses a profile that cannot be stored as it stands.
//
// It refuses shape problems and nonblank controlled values that the account
// cannot publish to. It never refuses a profile for being incomplete:
// incomplete is the normal state of a profile somebody is still filling in,
// and 3.1 says so.
func ValidateProfile(profile ExpressionProfile) error {
	if err := validateProfileShape(profile); err != nil {
		return err
	}
	// Channels are the one list whose values are controlled: they are the same
	// platforms an account can publish on, and a second list of platform names
	// would be a second thing to keep in step.
	for _, channel := range profile.PrimaryChannels.Values {
		if ValidatePlatform(channel) != nil {
			return ErrProfile
		}
	}
	return nil
}

// NormalizeProfile settles the statuses before storage.
//
// An empty value is pending, whatever the caller said. This only ever lowers a
// status and never writes a value, so it stays on the right side of "the system
// does not guess": marking a blank field confirmed would record a decision the
// creator did not make. Refusing the whole request instead would fail a profile
// because of one unrelated blank, which is the normal state of a form somebody
// is halfway through.
//
// An absent status on a field that HAS a value is left as pending too. Saying
// nothing is not confirming.
func NormalizeProfile(profile ExpressionProfile) ExpressionProfile {
	for _, field := range profile.textFields() {
		if strings.TrimSpace(field.Value) == "" {
			field.Status = FieldPending
			continue
		}
		if field.Status == "" {
			field.Status = FieldPending
		}
	}
	for _, field := range profile.listFields() {
		field.Values = nonEmpty(field.Values)
		if len(field.Values) == 0 {
			field.Status = FieldPending
			continue
		}
		if field.Status == "" {
			field.Status = FieldPending
		}
	}
	if profile.WeeklyHours.Status == "" {
		profile.WeeklyHours.Status = FieldPending
	}
	return profile
}

// nonEmpty drops blank entries. A list of one empty string is an empty list
// wearing a disguise, and it would otherwise satisfy "at least one channel".
func nonEmpty(values []string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			kept = append(kept, value)
		}
	}
	return kept
}

// Readiness is SOP 3.1's minimum condition to start, and what is still missing.
//
// A list rather than a bare bool: a caller has to be able to say WHAT is
// missing. "You cannot start yet" with no further detail is not something a
// creator can act on.
type Readiness struct {
	CanStart bool     `json:"can_start"`
	Missing  []string `json:"missing"`
}

// The four keys a caller sees in Missing. They are the field names, so a UI can
// point at the field rather than translating a sentence.
const (
	MissingAudience    = "audience"
	MissingPillars     = "content_pillars"
	MissingChannels    = "primary_channels"
	MissingWeeklyHours = "weekly_hours"
)

// ProfileReadiness answers whether this account can start.
//
// Only CONFIRMED fields count. A value the creator typed but has not confirmed
// is exactly the "pending" state 3.1 describes, and letting it satisfy a start
// condition would make confirmation decorative.
//
// content_pillars is 3.1's "content direction" (clarification Q3): of the ten
// fields it lists, that is the one that says what the account will keep
// publishing about.
func ProfileReadiness(profile ExpressionProfile) Readiness {
	readiness := Readiness{Missing: []string{}}

	if !confirmedText(profile.Audience) {
		readiness.Missing = append(readiness.Missing, MissingAudience)
	}
	if !confirmedText(profile.ContentPillars) {
		readiness.Missing = append(readiness.Missing, MissingPillars)
	}
	if profile.PrimaryChannels.Status != FieldConfirmed || len(nonEmpty(profile.PrimaryChannels.Values)) == 0 {
		readiness.Missing = append(readiness.Missing, MissingChannels)
	}
	// Zero confirmed hours is a statement that there is no time to spend, which
	// is not a condition to start under. This reading is an interpretation, not
	// something 3.1 says outright - it is recorded in the contract so it can be
	// argued with.
	if profile.WeeklyHours.Status != FieldConfirmed || profile.WeeklyHours.Value <= 0 {
		readiness.Missing = append(readiness.Missing, MissingWeeklyHours)
	}

	readiness.CanStart = len(readiness.Missing) == 0
	return readiness
}

func confirmedText(field TextField) bool {
	return field.Status == FieldConfirmed && strings.TrimSpace(field.Value) != ""
}

// UsesNeutralExpression is 3.1's "use neutral expression and mark it when there
// is no style sample".
//
// Computed, never stored. A stored flag and the samples themselves would be two
// answers to one question, and they would disagree the first time somebody adds
// a sample without touching the flag.
//
// It marks; it does not block. 3.1 is explicit that a missing sample must not
// stop material being recorded or writing being done by hand, so this never
// appears in ProfileReadiness.
func UsesNeutralExpression(profile ExpressionProfile) bool {
	return profile.StyleSamples.Status != FieldConfirmed ||
		len(nonEmpty(profile.StyleSamples.Values)) == 0
}
