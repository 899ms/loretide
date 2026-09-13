package diagnostics

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// TestSimulatorRegressionReceiverContracts validates the standalone behavior of Receiver:
// - Monotonically increasing sequence receives successfully (returns "")
// - Duplicate sequence numbers return "DUPLICATE"
// - Out-of-order sequence numbers (seq <= last seen) return "LATE_RESULT"
// - Receiver after cancellation returns "LATE_RESULT" even for higher sequences
func TestSimulatorRegressionReceiverContracts(t *testing.T) {
	t.Run("normal strictly increasing sequences accepted", func(t *testing.T) {
		r := Receiver{}
		for seq := 1; seq <= 5; seq++ {
			if res := r.Receive(seq); res != "" {
				t.Fatalf("expected empty string for normal sequence %d, got %q", seq, res)
			}
		}
	})

	t.Run("duplicate sequence rejected as DUPLICATE", func(t *testing.T) {
		r := Receiver{}
		if res := r.Receive(1); res != "" {
			t.Fatalf("seq 1 failed: %q", res)
		}
		if res := r.Receive(2); res != "" {
			t.Fatalf("seq 2 failed: %q", res)
		}
		if res := r.Receive(2); res != "DUPLICATE" {
			t.Fatalf("expected DUPLICATE for duplicate seq 2, got %q", res)
		}
	})

	t.Run("out-of-order sequence rejected as LATE_RESULT", func(t *testing.T) {
		r := Receiver{}
		if res := r.Receive(1); res != "" {
			t.Fatalf("seq 1 failed: %q", res)
		}
		if res := r.Receive(4); res != "" {
			t.Fatalf("seq 4 failed: %q", res)
		}
		// Seq 2 is smaller than last (4) and not yet seen -> LATE_RESULT
		if res := r.Receive(2); res != "LATE_RESULT" {
			t.Fatalf("expected LATE_RESULT for out-of-order seq 2, got %q", res)
		}
	})

	t.Run("cancelled receiver rejects all further results as LATE_RESULT", func(t *testing.T) {
		r := Receiver{Cancelled: true}
		for seq := 1; seq <= 3; seq++ {
			if res := r.Receive(seq); res != "LATE_RESULT" {
				t.Fatalf("expected LATE_RESULT for cancelled receiver on seq %d, got %q", seq, res)
			}
		}
	})
}

// TestSimulatorRegressionScenarioContracts verifies individual scenario failure contracts
// without relying on s.Expected from Scenarios:
// - Verifies exact event component sequences, event counts, Sequence numbers, and termination points
// - Checks exact failing component and specific outcome ("failed", "cancelled", or "ignored")
// - Verifies reconnect attempt transition point (attempt 1 before daemon, attempt 2 at daemon and onwards)
func TestSimulatorRegressionScenarioContracts(t *testing.T) {
	scope := Scope{Workspace: "ws-test", Actor: "actor-test"}

	allComponents := []string{"web", "api", "database", "queue", "daemon", "executor", "tool", "result", "database"}

	testCases := []struct {
		scenario          string
		expectedCode      string
		expectedStatus    string
		expectedComponents []string
		failingIndex      int // index in expectedComponents where failure occurs (-1 for success)
		failingOutcome    string
		verifyAttempts    func(t *testing.T, events []Event)
	}{
		{
			scenario:           "normal",
			expectedCode:       "",
			expectedStatus:     "completed",
			expectedComponents: allComponents,
			failingIndex:       -1,
			failingOutcome:     "",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "slow",
			expectedCode:       "",
			expectedStatus:     "completed",
			expectedComponents: allComponents,
			failingIndex:       -1,
			failingOutcome:     "",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "timeout",
			expectedCode:       "TIMEOUT",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor"}, // breaks after executor (index 5)
			failingIndex:       5,
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "cancel",
			expectedCode:       "CANCELLED",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor"}, // breaks after executor (index 5)
			failingIndex:       5,
			failingOutcome:     "cancelled",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "reconnect",
			expectedCode:       "NETWORK_UNAVAILABLE",
			expectedStatus:     "failed",
			expectedComponents: allComponents, // does not break on NETWORK_UNAVAILABLE
			failingIndex:       4,             // daemon at index 4
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				// Steps before daemon (indices 0..3: web, api, database, queue) must have attempt 1
				for i := 0; i <= 3; i++ {
					if events[i].Attempt != 1 {
						t.Errorf("event[%d] (%s) attempt expected 1 before reconnect, got %d", i, events[i].Component, events[i].Attempt)
					}
				}
				// Reconnect occurs in daemon (index 4); from daemon onwards (indices 4..8), attempt must be 2
				for i := 4; i < len(events); i++ {
					if events[i].Attempt != 2 {
						t.Errorf("event[%d] (%s) attempt expected 2 at/after reconnect, got %d", i, events[i].Component, events[i].Attempt)
					}
				}
			},
		},
		{
			scenario:           "duplicate",
			expectedCode:       "DUPLICATE",
			expectedStatus:     "failed",
			expectedComponents: allComponents, // does not break on DUPLICATE
			failingIndex:       7,             // result at index 7
			failingOutcome:     "ignored",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "late",
			expectedCode:       "LATE_RESULT",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor", "tool", "result"}, // breaks after result (index 7)
			failingIndex:       7,
			failingOutcome:     "ignored",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "file_missing",
			expectedCode:       "FILE_MISSING",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor", "tool"}, // breaks after tool (index 6)
			failingIndex:       6,
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "file_changed",
			expectedCode:       "FILE_CHANGED",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor", "tool"}, // breaks after tool (index 6)
			failingIndex:       6,
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "denied",
			expectedCode:       "AUTHORIZATION_DENIED",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor", "tool"}, // breaks after tool (index 6)
			failingIndex:       6,
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "model_auth",
			expectedCode:       "MODEL_AUTH",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor"}, // breaks after executor (index 5)
			failingIndex:       5,
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "model_quota",
			expectedCode:       "MODEL_QUOTA",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor"}, // breaks after executor (index 5)
			failingIndex:       5,
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "schema",
			expectedCode:       "OUTPUT_SCHEMA",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor", "tool", "result"}, // breaks after result (index 7)
			failingIndex:       7,
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "search",
			expectedCode:       "SEARCH_FAILED",
			expectedStatus:     "failed",
			expectedComponents: []string{"web", "api", "database", "queue", "daemon", "executor", "tool"}, // breaks after tool (index 6)
			failingIndex:       6,
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
		{
			scenario:           "clock_skew",
			expectedCode:       "CLOCK_SKEW",
			expectedStatus:     "failed",
			expectedComponents: allComponents, // does not break on CLOCK_SKEW
			failingIndex:       4,             // daemon at index 4
			failingOutcome:     "failed",
			verifyAttempts: func(t *testing.T, events []Event) {
				for i, e := range events {
					if e.Attempt != 1 {
						t.Errorf("event[%d] attempt should be 1, got %d", i, e.Attempt)
					}
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.scenario, func(t *testing.T) {
			run, err := Simulate(context.Background(), scope, "", tc.scenario, 42, "build-test", true)
			if err != nil {
				t.Fatalf("Simulate failed unexpectedly: %v", err)
			}

			// Assert code and status independently
			if run.Actual != tc.expectedCode {
				t.Errorf("expected Actual code %q, got %q", tc.expectedCode, run.Actual)
			}
			if run.Status != tc.expectedStatus {
				t.Errorf("expected Status %q, got %q", tc.expectedStatus, run.Status)
			}

			// Validate exact event count and prevent empty-slice panic
			if len(run.Events) != len(tc.expectedComponents) {
				t.Fatalf("exact event count mismatch: expected %d, got %d", len(tc.expectedComponents), len(run.Events))
			}

			// Extract components and verify sequence numbers (strictly 1..N)
			actualComponents := make([]string, len(run.Events))
			for i, ev := range run.Events {
				actualComponents[i] = ev.Component
				expectedSeq := int64(i + 1)
				if ev.Sequence != expectedSeq {
					t.Errorf("event[%d] sequence expected %d, got %d", i, expectedSeq, ev.Sequence)
				}
			}

			// Validate exact component sequence and termination point
			if !reflect.DeepEqual(actualComponents, tc.expectedComponents) {
				t.Fatalf("component sequence mismatch:\nexpected: %v\ngot:      %v", tc.expectedComponents, actualComponents)
			}

			// Validate outcomes and failing event specifics
			for i, ev := range run.Events {
				if i == tc.failingIndex {
					if ev.Code != tc.expectedCode {
						t.Errorf("failing event[%d] code mismatch: expected %q, got %q", i, tc.expectedCode, ev.Code)
					}
					if ev.Outcome != tc.failingOutcome {
						t.Errorf("failing event[%d] outcome mismatch: expected %q, got %q", i, tc.failingOutcome, ev.Outcome)
					}
				} else {
					if ev.Outcome != "success" {
						t.Errorf("non-failing event[%d] (%s) expected outcome 'success', got %q", i, ev.Component, ev.Outcome)
					}
				}
			}

			// Verify attempt transitions and counts
			if tc.verifyAttempts != nil {
				tc.verifyAttempts(t, run.Events)
			}
		})
	}
}

// TestSimulatorRegressionDeterministicTimeAndIdentifiers verifies:
// - Same seed produces identical virtual event durations and virtual timeline progression
// - Random IDs (ID, Trace, Span, Operation) are unique across distinct runs
// - All events within a single run share the same Operation ID and Trace correlation
func TestSimulatorRegressionDeterministicTimeAndIdentifiers(t *testing.T) {
	scope := Scope{Workspace: "ws-det", Actor: "actor-det"}
	const testSeed = int64(987654321)

	run1, err := Simulate(context.Background(), scope, "", "normal", testSeed, "build-1", true)
	if err != nil {
		t.Fatalf("run1 failed: %v", err)
	}

	run2, err := Simulate(context.Background(), scope, "", "normal", testSeed, "build-1", true)
	if err != nil {
		t.Fatalf("run2 failed: %v", err)
	}

	// 1. Run ID uniqueness across distinct runs
	if run1.ID == run2.ID {
		t.Errorf("run1.ID and run2.ID should be unique, but got identical %q", run1.ID)
	}

	// 2. Deterministic virtual time across identical seeds
	if len(run1.Events) != len(run2.Events) {
		t.Fatalf("events count mismatch: %d vs %d", len(run1.Events), len(run2.Events))
	}
	for i := range run1.Events {
		e1 := run1.Events[i]
		e2 := run2.Events[i]

		if e1.Duration != e2.Duration {
			t.Errorf("event[%d] duration mismatch: %d vs %d", i, e1.Duration, e2.Duration)
		}
		if !e1.Occurred.Equal(e2.Occurred) {
			t.Errorf("event[%d] virtual Occurred time mismatch: %v vs %v", i, e1.Occurred, e2.Occurred)
		}

		// Random per-event IDs must differ across distinct runs
		if e1.ID == e2.ID {
			t.Errorf("event[%d].ID should be unique between runs, got identical %q", i, e1.ID)
		}
		if e1.Span == e2.Span {
			t.Errorf("event[%d].Span should be unique between runs, got identical %q", i, e1.Span)
		}
	}

	// 3. Operation, Trace, and Span correlation
	// Across distinct runs, generated Operation and Trace IDs must differ
	if run1.Events[0].Operation == run2.Events[0].Operation {
		t.Errorf("Operation ID should be unique across distinct runs, got identical %q", run1.Events[0].Operation)
	}
	if run1.Events[0].Trace == run2.Events[0].Trace {
		t.Errorf("Trace ID should be unique across distinct runs, got identical %q", run1.Events[0].Trace)
	}

	// Within a single run, all events share the exact same Operation and Trace
	op1 := run1.Events[0].Operation
	trace1 := run1.Events[0].Trace
	if op1 == "" || trace1 == "" {
		t.Fatalf("expected non-empty operation and trace IDs")
	}
	for i, e := range run1.Events {
		if e.Operation != op1 {
			t.Errorf("event[%d].Operation %q does not match run operation %q", i, e.Operation, op1)
		}
		if e.Trace != trace1 {
			t.Errorf("event[%d].Trace %q does not match run trace %q", i, e.Trace, trace1)
		}
		if e.Run != run1.ID {
			t.Errorf("event[%d].Run %q does not match run1.ID %q", i, e.Run, run1.ID)
		}
	}
}

// TestSimulatorRegressionRejectionAndSecurity verifies:
// - testEnabled=false rejects with ErrDenied
// - unauthorized account rejects with ErrDenied
// - unknown scenario rejects with ErrConflict
// - database scenario requires persistent layer and is excluded from non-database simulation unit tests
func TestSimulatorRegressionRejectionAndSecurity(t *testing.T) {
	validScope := Scope{
		Workspace: "ws-corp",
		Actor:     "auditor-1",
		Accounts:  []string{"acc-allowed"},
	}

	t.Run("testEnabled false rejects with ErrDenied", func(t *testing.T) {
		_, err := Simulate(context.Background(), validScope, "acc-allowed", "normal", 1, "build", false)
		if !errors.Is(err, ErrDenied) {
			t.Fatalf("expected ErrDenied when testEnabled=false, got %v", err)
		}
	})

	t.Run("unauthorized account rejected with ErrDenied", func(t *testing.T) {
		_, err := Simulate(context.Background(), validScope, "acc-unauthorized-hacker", "normal", 1, "build", true)
		if !errors.Is(err, ErrDenied) {
			t.Fatalf("expected ErrDenied for unlisted account, got %v", err)
		}
	})

	t.Run("unknown scenario rejected with ErrConflict", func(t *testing.T) {
		for _, invalidScenario := range []string{"nonexistent", "arbitrary_eval", "sql_injection"} {
			_, err := Simulate(context.Background(), validScope, "acc-allowed", invalidScenario, 1, "build", true)
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("expected ErrConflict for unknown scenario %q, got %v", invalidScenario, err)
			}
		}
	})

	t.Run("database scenario note and exclusion verification", func(t *testing.T) {
		// Verify that database scenario exists in Scenarios definition
		foundDB := false
		for _, s := range Scenarios {
			if s.ID == "database" {
				foundDB = true
				if s.Expected != "DATABASE_UNAVAILABLE" {
					t.Fatalf("expected database expected_code to be DATABASE_UNAVAILABLE, got %s", s.Expected)
				}
			}
		}
		if !foundDB {
			t.Fatal("database scenario not found in Scenarios list")
		}

		// Direct Simulate call creates synthetic data, but full database commit/rollback
		// integration is owned by Issue #2 and store_integration_test.go.
		run, err := Simulate(context.Background(), validScope, "acc-allowed", "database", 1, "build", true)
		if err != nil {
			t.Fatalf("synthetic simulation for database scenario failed: %v", err)
		}
		if run.Scenario != "database" {
			t.Fatalf("expected scenario 'database', got %s", run.Scenario)
		}
	})
}
