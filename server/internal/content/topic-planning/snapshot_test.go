package topicplanning

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Contract: specs/023-ep04b-start-snapshot/contracts/start-snapshot.md section 4.
//
// AssembleSnapshot is the whole of what EP-04b produces, so the assembly rules
// are tested here rather than through an endpoint. Sixteen fields have sixteen
// sources; reaching them through one HTTP case would mean one failure message
// for all of them.

func completeInputs() SnapshotInputs {
	return SnapshotInputs{
		ChosenScope:           "web",
		SavedPreference:       "local",
		PersonaRef:            "rev-7",
		AutoPrecheck:          true,
		UsesNeutralExpression: false,
	}
}

func TestTheChosenScopeAndTheSavedPreferenceAreSeparateFields(t *testing.T) {
	// The single most likely quiet mistake: writing one value into both. It is
	// invisible in every case where the creator did not change the scope for
	// this start, which is most of them.
	snapshot := AssembleSnapshot(completeInputs())
	if snapshot.Scope != "web" {
		t.Errorf("source_scope = %q, want the scope chosen for this start", snapshot.Scope)
	}
	if snapshot.Preference != "local" {
		t.Errorf("saved_preference = %q, want the account preference as it was read", snapshot.Preference)
	}
	if snapshot.Scope == snapshot.Preference {
		t.Fatal("the two fields hold the same value; they are not one field")
	}
}

func TestTheSnapshotPinsThePersonaRevisionItRead(t *testing.T) {
	snapshot := AssembleSnapshot(completeInputs())
	if snapshot.PersonaRef != "rev-7" {
		t.Errorf("persona_ref = %q, want rev-7", snapshot.PersonaRef)
	}
}

func TestTheExecutorRecordsThatItIsDisabled(t *testing.T) {
	// Constitution IX. The field cannot be blank: blank reads as "nobody
	// filled this in", and this is a decision, not a gap.
	snapshot := AssembleSnapshot(completeInputs())
	if snapshot.Executor != ExecutorDisabled {
		t.Errorf("executor = %q, want %q", snapshot.Executor, ExecutorDisabled)
	}
}

func TestTheTwoExtensionKeysAreRecordedAndAreNotSnapshotFields(t *testing.T) {
	// The brand precheck switch and the neutral-expression marker have no field
	// among the sixteen, and every one of those sixteen already means something
	// else. They travel as declared extensions (FR-015a).
	for _, tc := range []struct {
		name     string
		inputs   SnapshotInputs
		precheck bool
		neutral  bool
	}{
		{"precheck on, styled", completeInputs(), true, false},
		{"precheck off, neutral", SnapshotInputs{
			ChosenScope: "all", SavedPreference: "all", PersonaRef: "r",
			AutoPrecheck: false, UsesNeutralExpression: true,
		}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := AssembleStored(tc.inputs)
			if stored.AutoPrecheck != tc.precheck {
				t.Errorf("auto_precheck = %v, want %v", stored.AutoPrecheck, tc.precheck)
			}
			if stored.UsesNeutralExpression != tc.neutral {
				t.Errorf("uses_neutral_expression = %v, want %v", stored.UsesNeutralExpression, tc.neutral)
			}
		})
	}
}

// The nine fields that have no source today. One case each, not one case for
// all nine: they have different future owners (EP-04d, W-03, EP-08), and a
// single "all empty" assertion would go red without saying which one somebody
// filled in.
func TestNoSourcelessFieldIsInvented(t *testing.T) {
	snapshot := AssembleSnapshot(completeInputs())
	for _, tc := range []struct {
		field string
		got   any
		owner string
	}{
		{"config_version", snapshot.ConfigVersion, "no version source yet"},
		{"sop_version", snapshot.SOPVersion, "no version source yet"},
		{"skill_version", snapshot.SkillVersion, "no version source yet"},
		{"rule_version", snapshot.RuleVersion, "no version source yet"},
		{"executor_version", snapshot.ExecutorVersion, "executors are disabled (constitution IX)"},
		{"required_sources", snapshot.Required, "EP-04d / W-03"},
		{"excluded_sources", snapshot.Excluded, "EP-04d / W-03"},
		{"grants", snapshot.Grants, "LT-016 ships CanRead only; nothing stores a Grant"},
		{"file_hashes", snapshot.Hashes, "no local file input in this phase"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			if !isEmptyValue(tc.got) {
				t.Errorf("%s = %#v, want empty: %s. Filling it in has to be a "+
					"deliberate change to this rule, not a quiet default", tc.field, tc.got, tc.owner)
			}
		})
	}
}

// Empty means empty, not nil: the sixteen fields are marshalled into the row,
// and a nil slice reads back as JSON null while an empty one reads back as [].
// A consumer that optional-chains a null is a consumer that has to know which
// of the two it got.
func TestEmptyCollectionsMarshalAsEmptyNotNull(t *testing.T) {
	body, err := json.Marshal(AssembleSnapshot(completeInputs()))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"required_sources":[]`, `"excluded_sources":[]`, `"grants":[]`, `"file_hashes":{}`,
	} {
		if !containsJSON(string(body), want) {
			t.Errorf("marshalled snapshot lacks %s: %s", want, body)
		}
	}
}

func TestTemperatureBudgetAndTimeoutAreZeroBecauseNothingMapsToThem(t *testing.T) {
	// cost_limit and time_limit on the brief are free text (022 does not
	// validate them), and turning free text into an integer needs a controlled
	// set that would be a change to 022's contract.
	snapshot := AssembleSnapshot(completeInputs())
	if snapshot.Temperature != 0 || snapshot.Budget != 0 || snapshot.Timeout != 0 {
		t.Errorf("temperature/budget/timeout = %v/%d/%d, want 0/0/0",
			snapshot.Temperature, snapshot.Budget, snapshot.Timeout)
	}
}

func isEmptyValue(v any) bool {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return true
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.Len() == 0
	case reflect.Slice, reflect.Map:
		return rv.Len() == 0
	default:
		return rv.IsZero()
	}
}

func containsJSON(body, fragment string) bool {
	for i := 0; i+len(fragment) <= len(body); i++ {
		if body[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
