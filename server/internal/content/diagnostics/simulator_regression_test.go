package diagnostics

import (
	"context"
	"errors"
	"testing"
	"time"
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
// - Verifies failing component, actual result code, attempt count, and event sequence
func TestSimulatorRegressionScenarioContracts(t *testing.T) {
	scope := Scope{Workspace: "ws-test", Actor: "actor-test"}

	testCases := []struct {
		scenario          string
		expectedCode      string
		expectedStatus    string
		expectedFailingAt string // step component where failure occurred
		minEvents         int
		expectedAttempt   int
	}{
		{
			scenario:          "normal",
			expectedCode:      "",
			expectedStatus:    "completed",
			expectedFailingAt: "",
			minEvents:         9, // all 9 steps complete
			expectedAttempt:   1,
		},
		{
			scenario:          "slow",
			expectedCode:      "",
			expectedStatus:    "completed",
			expectedFailingAt: "",
			minEvents:         9,
			expectedAttempt:   1,
		},
		{
			scenario:          "timeout",
			expectedCode:      "TIMEOUT",
			expectedStatus:    "failed",
			expectedFailingAt: "executor",
			minEvents:         6, // 00-web .. 05-executor (breaks after executor)
			expectedAttempt:   1,
		},
		{
			scenario:          "cancel",
			expectedCode:      "CANCELLED",
			expectedStatus:    "failed",
			expectedFailingAt: "executor",
			minEvents:         6, // breaks after executor
			expectedAttempt:   1,
		},
		{
			scenario:          "reconnect",
			expectedCode:      "NETWORK_UNAVAILABLE",
			expectedStatus:    "failed",
			expectedFailingAt: "daemon",
			minEvents:         9, // does not break, finishes remaining steps
			expectedAttempt:   2, // attempt incremented on reconnect
		},
		{
			scenario:          "duplicate",
			expectedCode:      "DUPLICATE",
			expectedStatus:    "failed",
			expectedFailingAt: "result",
			minEvents:         9, // does not break
			expectedAttempt:   1,
		},
		{
			scenario:          "late",
			expectedCode:      "LATE_RESULT",
			expectedStatus:    "failed",
			expectedFailingAt: "result",
			minEvents:         8, // breaks after result
			expectedAttempt:   1,
		},
		{
			scenario:          "file_missing",
			expectedCode:      "FILE_MISSING",
			expectedStatus:    "failed",
			expectedFailingAt: "tool",
			minEvents:         7, // breaks after tool
			expectedAttempt:   1,
		},
		{
			scenario:          "file_changed",
			expectedCode:      "FILE_CHANGED",
			expectedStatus:    "failed",
			expectedFailingAt: "tool",
			minEvents:         7, // breaks after tool
			expectedAttempt:   1,
		},
		{
			scenario:          "denied",
			expectedCode:      "AUTHORIZATION_DENIED",
			expectedStatus:    "failed",
			expectedFailingAt: "tool",
			minEvents:         7, // breaks after tool
			expectedAttempt:   1,
		},
		{
			scenario:          "model_auth",
			expectedCode:      "MODEL_AUTH",
			expectedStatus:    "failed",
			expectedFailingAt: "executor",
			minEvents:         6, // breaks after executor
			expectedAttempt:   1,
		},
		{
			scenario:          "model_quota",
			expectedCode:      "MODEL_QUOTA",
			expectedStatus:    "failed",
			expectedFailingAt: "executor",
			minEvents:         6, // breaks after executor
			expectedAttempt:   1,
		},
		{
			scenario:          "schema",
			expectedCode:      "OUTPUT_SCHEMA",
			expectedStatus:    "failed",
			expectedFailingAt: "result",
			minEvents:         8, // breaks after result
			expectedAttempt:   1,
		},
		{
			scenario:          "search",
			expectedCode:      "SEARCH_FAILED",
			expectedStatus:    "failed",
			expectedFailingAt: "tool",
			minEvents:         7, // breaks after tool
			expectedAttempt:   1,
		},
		{
			scenario:          "clock_skew",
			expectedCode:      "CLOCK_SKEW",
			expectedStatus:    "failed",
			expectedFailingAt: "daemon",
			minEvents:         9, // does not break
			expectedAttempt:   1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.scenario, func(t *testing.T) {
			run, err := Simulate(context.Background(), scope, "", tc.scenario, 42, "build-test", true)
			if err != nil {
				t.Fatalf("Simulate failed unexpectedly: %v", err)
			}

			// Assert independently without referencing tc.scenario in Scenarios
			if run.Actual != tc.expectedCode {
				t.Errorf("expected Actual code %q, got %q", tc.expectedCode, run.Actual)
			}
			if run.Status != tc.expectedStatus {
				t.Errorf("expected Status %q, got %q", tc.expectedStatus, run.Status)
			}
			if len(run.Events) < tc.minEvents {
				t.Errorf("expected at least %d events, got %d", tc.minEvents, len(run.Events))
			}

			// If a failure was expected, inspect the event where it occurred
			if tc.expectedFailingAt != "" {
				var failingEvent *Event
				for i := range run.Events {
					if run.Events[i].Component == tc.expectedFailingAt && run.Events[i].Code == tc.expectedCode {
						failingEvent = &run.Events[i]
						break
					}
				}
				if failingEvent == nil {
					t.Fatalf("failed to find event with component %q and code %q", tc.expectedFailingAt, tc.expectedCode)
				}
				if failingEvent.Outcome != "failed" && failingEvent.Outcome != "cancelled" && failingEvent.Outcome != "ignored" {
					t.Errorf("unexpected outcome for failing event: %q", failingEvent.Outcome)
				}
			}

			// Check attempt count
			lastEvent := run.Events[len(run.Events)-1]
			if lastEvent.Attempt != tc.expectedAttempt {
				t.Errorf("expected attempt %d, got %d", tc.expectedAttempt, lastEvent.Attempt)
			}
		})
	}
}

// TestSimulatorRegressionDeterministicTimeAndIdentifiers verifies:
// - Same seed produces identical virtual event duration and timeline progression
// - Random IDs (ID, Trace, Span, Operation) are unique per run and not verbatim identical across runs
// - All events within a single run share the same Operation ID and Trace correlation
func TestSimulatorRegressionDeterministicTimeAndIdentifiers(t *testing.T) {
	scope := Scope{Workspace: "ws-det", Actor: "actor-det"}
	const testSeed = int64(987654321)

	run1, err := Simulate(context.Background(), scope, "", "normal", testSeed, "build-1", true)
	if err != nil {
		t.Fatalf("run1 failed: %v", err)
	}

	// Sleep slightly to ensure real wall-clock time differs
	time.Sleep(2 * time.Millisecond)

	run2, err := Simulate(context.Background(), scope, "", "normal", testSeed, "build-1", true)
	if err != nil {
		t.Fatalf("run2 failed: %v", err)
	}

	// 1. Unique run identifiers
	if run1.ID == run2.ID {
		t.Errorf("run1.ID and run2.ID should be unique, but got identical %q", run1.ID)
	}
	if run1.Created.Equal(run2.Created) {
		t.Errorf("real Created timestamp should not be verbatim identical")
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

		// Random per-event IDs must differ between runs
		if e1.ID == e2.ID {
			t.Errorf("event[%d].ID should be unique between runs, got identical %q", i, e1.ID)
		}
	}

	// 3. Operation and Trace association within a single run
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
