package workeditor

import (
	"strings"
	"testing"
)

// Contract: specs/024-work-editor-manual/contracts/work-and-versions.md
//
// The four controlled sets and the two rules that are easiest to get subtly
// wrong: rune-counted limits, and the fact that restoring is an ACTION and not
// a fourth source.

func TestTheControlledSetsAreExactlyWhatSOP71Gives(t *testing.T) {
	// Not "at least these": exactly these. A set that quietly grows is how a
	// value nobody agreed to ends up stored and then relied on.
	for _, tc := range []struct {
		name string
		got  []string
		want []string
	}{
		{"kind", asStrings(Kinds), []string{"body", "channel_draft"}},
		{"draft_status", asStrings(DraftStatuses), []string{"working", "saved"}},
		{"source", asStrings(Sources), []string{"generated", "edited", "adopted"}},
		{"action", asStrings(Actions), []string{"saved", "restored", "adopted"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.got) != len(tc.want) {
				t.Fatalf("%s has %d values %v, want %d %v", tc.name, len(tc.got), tc.got, len(tc.want), tc.want)
			}
			for i := range tc.want {
				if tc.got[i] != tc.want[i] {
					t.Errorf("%s[%d] = %q, want %q", tc.name, i, tc.got[i], tc.want[i])
				}
			}
		})
	}
}

// "restored" must not be a source. The content that comes back is still what a
// person wrote, so the source stays edited; that it was restored is an action.
func TestRestoredIsAnActionAndNotASource(t *testing.T) {
	if ValidateSource("restored") == nil {
		t.Error("source=restored was accepted; restoring is an action, not a fourth source")
	}
	if ValidateAction("restored") != nil {
		t.Error("action=restored was rejected")
	}
	if ValidateSource("edited") != nil || ValidateAction("saved") != nil {
		t.Error("an ordinary save is (edited, saved) and must validate")
	}
}

func TestEachControlledSetRejectsAValueOutsideIt(t *testing.T) {
	for _, tc := range []struct {
		name     string
		validate func(string) error
		bad      string
	}{
		{"kind", func(v string) error { return ValidateKind(v) }, "outline"},
		{"draft_status", func(v string) error { return ValidateDraftStatus(v) }, "done"},
		{"source", func(v string) error { return ValidateSource(v) }, "imported"},
		{"action", func(v string) error { return ValidateAction(v) }, "published"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.validate(tc.bad); err == nil {
				t.Errorf("%s=%q was accepted", tc.name, tc.bad)
			}
			// Exact match only: no trimming, no case folding. Two spellings of
			// one value in storage means every reader has to know both.
			if err := tc.validate(strings.ToUpper(string(tc.name[0])) + ""); err == nil {
				t.Errorf("%s accepted a value that is not in the set", tc.name)
			}
		})
	}
}

// Runes, not bytes. A byte limit gives a Chinese draft a third of the room an
// English one gets, which is the same reason the persona prompt counts runes.
func TestLengthLimitsAreCountedInRunes(t *testing.T) {
	chinese := strings.Repeat("字", MaxBodyRunes)
	if err := ValidateBody(chinese); err != nil {
		t.Errorf("a body of exactly MaxBodyRunes Chinese characters was rejected: %v", err)
	}
	if len(chinese) <= MaxBodyRunes {
		t.Fatal("the fixture is not multi-byte; this test would pass under a byte limit too")
	}
	if err := ValidateBody(chinese + "字"); err == nil {
		t.Error("a body one rune over the limit was accepted")
	}
}

// SOP 3.1's reading, applied here: an empty answer is a real state, not a
// validation failure. Saving a blank version is legitimate.
func TestAnEmptyBodyIsLegitimate(t *testing.T) {
	if err := ValidateBody(""); err != nil {
		t.Errorf("an empty body was rejected: %v", err)
	}
}

// The recomputation FR-007b's consistency assertions compare against.
func TestDraftStatusIsSavedOnlyWhenTheDraftMatchesTheLatestVersion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		draft  string
		latest *ArtifactVersion
		want   DraftStatus
	}{
		{"no version yet, empty draft", "", nil, DraftSaved},
		{"no version yet, something typed", "hello", nil, DraftWorking},
		{"draft equals the latest version", "hello", &ArtifactVersion{Body: "hello"}, DraftSaved},
		{"draft differs by one character", "hello!", &ArtifactVersion{Body: "hello"}, DraftWorking},
		{"draft differs only by whitespace", "hello ", &ArtifactVersion{Body: "hello"}, DraftWorking},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DraftStatusFor(tc.draft, tc.latest); got != tc.want {
				t.Errorf("DraftStatusFor(%q, %v) = %q, want %q", tc.draft, tc.latest, got, tc.want)
			}
		})
	}
}

func asStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}
