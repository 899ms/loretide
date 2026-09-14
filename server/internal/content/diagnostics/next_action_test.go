package diagnostics

import "testing"

// The next action a reader is told to take. Sanitize derives it from the error
// code (log.go:99-100). Four of the five branches already had assertions; what
// was missing is that the mapping is TOTAL over the codes the product actually
// produces, that check_registered_file is covered at all, and that the action
// and the retryable flag agree.
//
// Contract: specs/008-diag-linkage-and-invariants/contracts/next-action.md
//
// The closed set lives here rather than in log.go on purpose: "these five and
// nothing else" is the claim this test makes. Reading it from production code
// would compare the implementation against itself and pass however wrong it got.
var knownNextActions = map[string]bool{
	"inspect_trace":         true,
	"retry_simulation":      true,
	"check_authorization":   true,
	"check_registered_file": true,
	"check_local_client":    true,
}

// The four codes log.go treats as worth retrying. Kept apart from the action
// map so A4 can assert the two agree rather than restating one from the other.
var retryableCodes = map[string]bool{
	"NETWORK_UNAVAILABLE":  true,
	"SEARCH_FAILED":        true,
	"TIMEOUT":              true,
	"DATABASE_UNAVAILABLE": true,
}

func nextFor(code string) Event {
	return Sanitize(Event{Code: code, Component: "diagnostics", Severity: "info"})
}

// A1. Every code the simulator can actually produce must have a DECIDED action.
//
// Checking only "the result is in the known set" does not work: an unmapped code
// falls through to inspect_trace, which is in the set, so such a test passes for
// exactly the case it exists to catch. (It did - the mutation that adds an
// unmapped scenario code left the first draft of this test green.) So the
// expectation is written out per code, and a scenario whose code appears in
// neither table fails until someone decides which action it deserves.
var expectedNextByCode = map[string]string{
	"NETWORK_UNAVAILABLE":  "retry_simulation",
	"SEARCH_FAILED":        "retry_simulation",
	"TIMEOUT":              "retry_simulation",
	"DATABASE_UNAVAILABLE": "retry_simulation",
	"AUTHORIZATION_DENIED": "check_authorization",
	"FILE_MISSING":         "check_registered_file",
	"FILE_CHANGED":         "check_registered_file",
	"MODEL_AUTH":           "check_local_client",
	"MODEL_QUOTA":          "check_local_client",
}

// Codes that deliberately take the default. Listed rather than assumed, so that
// adding a scenario does not quietly join them.
var intentionallyDefaulted = map[string]bool{
	"":              true, // normal and slow: no fault, nothing to act on
	"CANCELLED":     true,
	"DUPLICATE":     true,
	"LATE_RESULT":   true,
	"OUTPUT_SCHEMA": true,
	"CLOCK_SKEW":    true,
}

func TestNextActionIsTotalOverEveryScenarioCode(t *testing.T) {
	if len(Scenarios) == 0 {
		t.Fatal("no scenarios to check")
	}
	for _, scenario := range Scenarios {
		code := scenario.Expected
		out := nextFor(code)
		if out.Next == "" {
			t.Errorf("scenario %s (code %q): next action is empty", scenario.ID, code)
			continue
		}
		if !knownNextActions[out.Next] {
			t.Errorf("scenario %s (code %q): next action %q is outside the known set",
				scenario.ID, code, out.Next)
			continue
		}
		want, decided := expectedNextByCode[code]
		if decided {
			if out.Next != want {
				t.Errorf("scenario %s (code %q): got next=%q, want %q",
					scenario.ID, code, out.Next, want)
			}
			continue
		}
		if !intentionallyDefaulted[code] {
			t.Errorf("scenario %s introduces code %q with no decided next action; "+
				"it silently takes the default %q. Add it to expectedNextByCode or "+
				"to intentionallyDefaulted.", scenario.ID, code, out.Next)
			continue
		}
		if out.Next != "inspect_trace" {
			t.Errorf("code %q is listed as intentionally defaulted but maps to %q",
				code, out.Next)
		}
	}
}

// A2. The only branch that had no assertion before this feature.
func TestNextActionForMissingAndChangedFilesPointsAtTheRegistration(t *testing.T) {
	for _, code := range []string{"FILE_MISSING", "FILE_CHANGED"} {
		out := nextFor(code)
		if out.Next != "check_registered_file" {
			t.Errorf("code %s: got next=%q, want check_registered_file", code, out.Next)
		}
		if out.Retryable {
			t.Errorf("code %s: a missing or changed file is not retryable as-is", code)
		}
	}
}

// A3 and A5. An unmapped code and an empty code must both still tell the reader
// something; falling through to an empty string would leave a blank cell.
func TestNextActionFallsBackToInspectTraceRatherThanNothing(t *testing.T) {
	for _, code := range []string{"", "NOT_A_REAL_CODE", "lowercase_code", "TIMEOUT_"} {
		out := nextFor(code)
		if out.Next != "inspect_trace" {
			t.Errorf("code %q: got next=%q, want the default inspect_trace", code, out.Next)
		}
		if out.Retryable {
			t.Errorf("code %q: the default must not claim retryable", code)
		}
	}
}

// A4. The action and the flag are two halves of one instruction. "Retry this"
// with retryable=false, or an unretryable action with retryable=true, would
// have the panel and the caller disagree about what to do next.
func TestNextActionAndRetryableAgreeOnEveryCode(t *testing.T) {
	codes := []string{
		"", "NOT_A_REAL_CODE",
		"NETWORK_UNAVAILABLE", "SEARCH_FAILED", "TIMEOUT", "DATABASE_UNAVAILABLE",
		"AUTHORIZATION_DENIED", "FILE_MISSING", "FILE_CHANGED",
		"MODEL_AUTH", "MODEL_QUOTA", "CANCELLED", "DUPLICATE", "LATE_RESULT",
		"OUTPUT_SCHEMA", "CLOCK_SKEW",
	}
	for _, code := range codes {
		out := nextFor(code)
		if !knownNextActions[out.Next] {
			t.Errorf("code %q: next action %q is outside the known set", code, out.Next)
		}
		wantRetryable := retryableCodes[code]
		if out.Retryable != wantRetryable {
			t.Errorf("code %q: retryable=%v, want %v", code, out.Retryable, wantRetryable)
		}
		// Retryable and "retry_simulation" must be the same decision.
		if out.Retryable != (out.Next == "retry_simulation") {
			t.Errorf("code %q: retryable=%v but next=%q - the flag and the action disagree",
				code, out.Retryable, out.Next)
		}
	}
}
