package topicplanning

import (
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// The input snapshot: what a start fixed, so that changing the account, the
// preference, the brief or the brand switch afterwards cannot rewrite it.
//
// Assembly is a pure function on purpose. Sixteen fields have sixteen sources,
// and the rules for them are the whole of what EP-04b produces; inside a
// handler they could only be reached through one endpoint case, which would
// give sixteen rules one failure message.
//
// Contract: specs/023-ep04b-start-snapshot/contracts/start-snapshot.md

// ExecutorDisabled is what the executor field records while constitution IX
// holds. Not the empty string: blank reads as "nobody filled this in", and
// this is a decision.
const ExecutorDisabled = "disabled"

// SnapshotInputs is everything the assembly needs, read by the caller.
//
// Nothing here is read from a database or a clock. The caller reads the four
// places, this function decides what they mean - the same split CanRead uses
// for its Now.
type SnapshotInputs struct {
	// ChosenScope is the material scope selected FOR THIS START.
	ChosenScope string
	// SavedPreference is the account's stored preference as it was read at
	// that moment. Separate from ChosenScope, and often different: LT-014
	// already records that the preference is "what was chosen last time",
	// while every run records the scope it actually used.
	SavedPreference string
	// PersonaRef is the account revision id in force at the start.
	PersonaRef string
	// AutoPrecheck is the brand switch (LT-015) as it was read. Recorded, never
	// acted on: triggering a precheck is EP-06.
	AutoPrecheck bool
	// UsesNeutralExpression is 021's marker as it was read. Recorded, never a
	// gate: 021 settled that it marks and does not block.
	UsesNeutralExpression bool
}

// StoredSnapshot is what goes into the row: the sixteen aligned fields plus the
// two extensions.
//
// The extensions are declared rather than squeezed into one of the sixteen.
// Each of those sixteen already means something else, and borrowing one would
// make "aligned with diagnostics.Snapshot" a half-truth.
type StoredSnapshot struct {
	diagnostics.Snapshot
	// AutoPrecheck and UsesNeutralExpression are topic-planning EXTENSIONS.
	// They are not part of diagnostics.Snapshot and must not be read as such.
	AutoPrecheck          bool `json:"auto_precheck"`
	UsesNeutralExpression bool `json:"uses_neutral_expression"`
}

// AssembleSnapshot builds the sixteen aligned fields.
//
// Nine of them have no source in this phase. They are written empty and each
// has its own negative case: filling one in has to be a deliberate change to
// this function, not a quiet default that a later reader takes for real.
func AssembleSnapshot(inputs SnapshotInputs) diagnostics.Snapshot {
	return diagnostics.Snapshot{
		// No version source exists yet for any of these four.
		ConfigVersion: "",
		SOPVersion:    "",
		SkillVersion:  "",
		RuleVersion:   "",

		PersonaRef: inputs.PersonaRef,

		Executor: ExecutorDisabled,
		// There is no executor to have a version.
		ExecutorVersion: "",

		// Two fields, two meanings. Collapsing them would be invisible in every
		// case where the creator did not change the scope for this start.
		Scope:      inputs.ChosenScope,
		Preference: inputs.SavedPreference,

		// Empty, not nil: these are marshalled into the row, and a nil slice
		// reads back as JSON null while an empty one reads back as []. A
		// consumer should not have to know which of the two it received.
		Required: []string{},          // EP-04d / W-03
		Excluded: []string{},          // EP-04d / W-03
		Grants:   []string{},          // LT-016 ships CanRead only; nothing stores a Grant
		Hashes:   map[string]string{}, // no local file input in this phase

		// No model call, so no temperature. cost_limit and time_limit on the
		// brief are free text (022 does not validate them), and turning free
		// text into an integer needs a controlled set, which would be a change
		// to 022's contract.
		Temperature: 0,
		Budget:      0,
		Timeout:     0,
	}
}

// AssembleStored adds the two extension keys.
func AssembleStored(inputs SnapshotInputs) StoredSnapshot {
	return StoredSnapshot{
		Snapshot:              AssembleSnapshot(inputs),
		AutoPrecheck:          inputs.AutoPrecheck,
		UsesNeutralExpression: inputs.UsesNeutralExpression,
	}
}
