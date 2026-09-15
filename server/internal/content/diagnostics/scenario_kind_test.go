package diagnostics

import "testing"

// The 16 business fault scenarios as they stood before the self-check shapes
// were added. This list is the fixed point of INV-1: a shape must never grow
// this set, and a fault must never leave it silently.
//
// docs/13 section 7 enumerates 15 of these; clock_skew is the superset entry
// (checked by the main task on 2026-09-14 at c9cc63eca). INV-2 below keeps the
// shapes disjoint from that enumeration, so the mapping stays checkable by
// machine rather than by a sentence in a document.
var faultScenariosBeforeShapes = []string{
	"normal", "slow", "timeout", "cancel", "reconnect", "duplicate", "late",
	"file_missing", "file_changed", "denied", "database", "model_auth",
	"model_quota", "schema", "search", "clock_skew",
}

// docs/13 section 7's enumeration: the 16 above minus clock_skew.
func docs13Section7() map[string]bool {
	out := map[string]bool{}
	for _, id := range faultScenariosBeforeShapes {
		if id == "clock_skew" {
			continue
		}
		out[id] = true
	}
	return out
}

func TestEveryScenarioDeclaresAKind(t *testing.T) {
	for _, s := range Scenarios {
		if s.Kind != "fault" && s.Kind != "shape" {
			t.Errorf("scenario %s: kind %q is neither %q nor %q. Classification has to be a "+
				"fact in the struct, not a convention in a document, or the docs/13 section 7 "+
				"mapping stops being checkable.", s.ID, s.Kind, "fault", "shape")
		}
	}
}

// INV-1: adding shapes must not change which scenarios are business faults.
func TestFaultScenariosAreExactlyTheOriginalSixteen(t *testing.T) {
	got := map[string]bool{}
	for _, s := range Scenarios {
		if s.Kind == "fault" {
			got[s.ID] = true
		}
	}
	for _, id := range faultScenariosBeforeShapes {
		if !got[id] {
			t.Errorf("%s is no longer a fault scenario; docs/13 section 7 coverage would drop", id)
		}
		delete(got, id)
	}
	for id := range got {
		t.Errorf("%s was added as a fault scenario. A self-check data shape must be kind %q, "+
			"otherwise it lands inside the docs/13 section 7 comparison it is not part of.", id, "shape")
	}
}

// INV-2: the shapes are disjoint from docs/13 section 7.
func TestShapeScenariosAreDisjointFromDocs13Section7(t *testing.T) {
	section7 := docs13Section7()
	shapes := 0
	for _, s := range Scenarios {
		if s.Kind != "shape" {
			continue
		}
		shapes++
		if section7[s.ID] {
			t.Errorf("shape %s collides with a docs/13 section 7 fault id", s.ID)
		}
	}
	if shapes == 0 {
		t.Error("no shape scenarios are registered, so nothing this feature exists for is reachable")
	}
}

// INV-4: kind is the rule; the shape_ prefix is only readability. Nothing may
// decide classification by parsing the id - today the two agree, and the day
// someone renames a shape they would stop agreeing silently.
func TestClassificationIsNotDerivedFromTheIDPrefix(t *testing.T) {
	for _, s := range Scenarios {
		if s.Kind == "shape" && !hasShapePrefix(s.ID) {
			t.Errorf("shape %s does not carry the shape_ prefix; the convention is for readers, "+
				"so keep it, but note the test that matters is the Kind field", s.ID)
		}
		if s.Kind == "fault" && hasShapePrefix(s.ID) {
			t.Errorf("fault %s carries the shape_ prefix and would read as a shape", s.ID)
		}
	}
}

func hasShapePrefix(id string) bool {
	const p = "shape_"
	return len(id) > len(p) && id[:len(p)] == p
}
