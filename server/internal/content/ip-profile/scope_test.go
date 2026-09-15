package ipprofile

import (
	"errors"
	"testing"
)

// A1. An account nobody has touched reads as "all". Not "", not absent: the
// start screen (EP-04) renders a radio group from this value, and an empty one
// leaves all three unselected on first open.
func TestAnAccountThatNeverChoseReadsAsAll(t *testing.T) {
	for name, settings := range map[string]map[string]any{
		"no settings at all": nil,
		"empty settings":     {},
		"other keys only":    {"something.else": "kept"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ScopeOf(settings); got != DefaultScope {
				t.Errorf("ScopeOf(%v) = %q, want %q", settings, got, DefaultScope)
			}
		})
	}
}

func TestAStoredScopeIsReadBack(t *testing.T) {
	for _, scope := range Scopes {
		settings := map[string]any{ScopeSettingsKey: string(scope)}
		if got := ScopeOf(settings); got != string(scope) {
			t.Errorf("ScopeOf(%q) = %q", scope, got)
		}
	}
}

// A12. A value nothing recognises reads as the default rather than being handed
// on. Reading is not a repair operation either - see TestReadingDoesNotChange
// below - so a bad value stays in the row until someone writes a good one.
func TestAnUnrecognisedStoredScopeReadsAsAll(t *testing.T) {
	for name, stored := range map[string]any{
		"unknown word": "everything",
		"wrong case":   "Local",
		"empty string": "",
		"a number":     42,
		"a list":       []any{"local"},
		"null":         nil,
	} {
		t.Run(name, func(t *testing.T) {
			if got := ScopeOf(map[string]any{ScopeSettingsKey: stored}); got != DefaultScope {
				t.Errorf("ScopeOf(%v) = %q, want %q", stored, got, DefaultScope)
			}
		})
	}
}

// A2. Filling the default is something that happens to a RESPONSE. If reading
// wrote the default back, an account that never chose would become
// indistinguishable from one that chose "all" on purpose, and the first read
// would silently make that decision on the person's behalf.
func TestReadingDoesNotChangeTheStoredSettings(t *testing.T) {
	settings := map[string]any{"something.else": "kept"}

	if got := ScopeOf(settings); got != DefaultScope {
		t.Fatalf("ScopeOf = %q", got)
	}
	if _, present := settings[ScopeSettingsKey]; present {
		t.Error("reading wrote the default back into the settings it was given")
	}
	if len(settings) != 1 {
		t.Errorf("reading changed the settings map: %v", settings)
	}
}

func TestOnlyTheThreeControlledValuesAreAccepted(t *testing.T) {
	for _, scope := range Scopes {
		if err := ValidateScope(string(scope)); err != nil {
			t.Errorf("ValidateScope(%q) = %v, want nil", scope, err)
		}
	}
	for _, value := range []string{"", " ", "All", "LOCAL", "everything", "local,web", "local "} {
		if err := ValidateScope(value); !errors.Is(err, ErrScope) {
			t.Errorf("ValidateScope(%q) = %v, want ErrScope", value, err)
		}
	}
}

// A4. The settings blob is replaced wholesale by the update query, so merging
// is not an optimisation - writing just this key would delete every other
// setting the account has.
func TestWritingTheScopeKeepsEveryOtherSetting(t *testing.T) {
	existing := map[string]any{"something.else": "kept", "a.number": 7}

	merged := withScope(existing, "local")

	if merged[ScopeSettingsKey] != "local" {
		t.Errorf("scope = %v", merged[ScopeSettingsKey])
	}
	if merged["something.else"] != "kept" || merged["a.number"] != 7 {
		t.Errorf("other settings did not survive: %v", merged)
	}
	if _, present := existing[ScopeSettingsKey]; present {
		t.Error("merging wrote into the caller's map instead of returning a new one")
	}
}

func TestWritingTheScopeWorksFromNothing(t *testing.T) {
	merged := withScope(nil, "web")

	if len(merged) != 1 || merged[ScopeSettingsKey] != "web" {
		t.Errorf("withScope(nil) = %v", merged)
	}
}

// Overwriting is the whole point: this is "the last choice", not a history.
func TestWritingTheScopeReplacesAPreviousChoice(t *testing.T) {
	merged := withScope(map[string]any{ScopeSettingsKey: "local"}, "web")

	if merged[ScopeSettingsKey] != "web" {
		t.Errorf("scope = %v, want web", merged[ScopeSettingsKey])
	}
}
