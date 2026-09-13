package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLogRegressionSanitizeRules tests table-driven sanitization of Event fields:
// - Unknown error codes/enums normalized to safe defaults ("INTERNAL", "unknown")
// - Invalid non-hex trace/span/run/operation/id identifiers stripped to empty
// - Paths and URLs in Step/Build/Version replaced by "[redacted]"
// - Safe message derived from error code map, distinguishing business identity fields from tech messages
func TestLogRegressionSanitizeRules(t *testing.T) {
	testCases := []struct {
		name     string
		input    Event
		validate func(t *testing.T, out Event)
	}{
		{
			name: "unknown error code and enums normalized to safe defaults",
			input: Event{
				Code:       "TOTALLY_UNKNOWN_CODE_XYZ",
				Component:  "rogue_service",
				Severity:   "catastrophic",
				ActorKind:  "alien",
				Outcome:    "exploded",
				Action:     "hack_database",
				ObjectType: "unregistered_object",
			},
			validate: func(t *testing.T, out Event) {
				if out.Code != "INTERNAL" {
					t.Errorf("expected Code to be normalized to INTERNAL, got %q", out.Code)
				}
				if out.Message != "Internal error" {
					t.Errorf("expected Message to be 'Internal error', got %q", out.Message)
				}
				if out.Component != "unknown" {
					t.Errorf("expected Component to be 'unknown', got %q", out.Component)
				}
				if out.Severity != "unknown" {
					t.Errorf("expected Severity to be 'unknown', got %q", out.Severity)
				}
				if out.ActorKind != "unknown" {
					t.Errorf("expected ActorKind to be 'unknown', got %q", out.ActorKind)
				}
				if out.Outcome != "unknown" {
					t.Errorf("expected Outcome to be 'unknown', got %q", out.Outcome)
				}
				if out.Action != "unknown" {
					t.Errorf("expected Action to be 'unknown', got %q", out.Action)
				}
				if out.ObjectType != "unknown" {
					t.Errorf("expected ObjectType to be 'unknown', got %q", out.ObjectType)
				}
				if out.Next != "inspect_trace" || out.Retryable != false {
					t.Errorf("expected default next inspect_trace and retryable false, got next=%s retryable=%v", out.Next, out.Retryable)
				}
			},
		},
		{
			name: "invalid trace and span identifiers stripped",
			input: Event{
				Trace:     "not-a-valid-hex-trace-id!!",
				Span:      "http://attacker.com/span",
				Parent:    "invalid_parent_with_symbols!@#",
				Operation: "12345", // too short, hexID requires 16-32 hex chars
				Run:       "this-is-thirty-two-chars-long-nonhex",
				ID:        "Z123456789abcdef0123456789abcdef", // non-hex character 'Z'
			},
			validate: func(t *testing.T, out Event) {
				if out.Trace != "" {
					t.Errorf("expected Trace to be cleared, got %q", out.Trace)
				}
				if out.Span != "" {
					t.Errorf("expected Span to be cleared, got %q", out.Span)
				}
				if out.Parent != "" {
					t.Errorf("expected Parent to be cleared, got %q", out.Parent)
				}
				if out.Operation != "" {
					t.Errorf("expected Operation to be cleared, got %q", out.Operation)
				}
				if out.Run != "" {
					t.Errorf("expected Run to be cleared, got %q", out.Run)
				}
				if out.ID != "" {
					t.Errorf("expected ID to be cleared, got %q", out.ID)
				}
			},
		},
		{
			name: "valid hex identifiers preserved",
			input: Event{
				Trace:     "0123456789abcdef0123456789abcdef",
				Span:      "0123456789abcdef",
				Parent:    "fedcba9876543210",
				Operation: "aabbccddeeff00112233445566778899",
				Run:       "deadbeefdeadbeefdeadbeefdeadbeef",
				ID:        "11223344556677889900aabbccddeeff",
			},
			validate: func(t *testing.T, out Event) {
				if out.Trace != "0123456789abcdef0123456789abcdef" {
					t.Errorf("expected Trace preserved, got %q", out.Trace)
				}
				if out.Span != "0123456789abcdef" {
					t.Errorf("expected Span preserved, got %q", out.Span)
				}
				if out.Parent != "fedcba9876543210" {
					t.Errorf("expected Parent preserved, got %q", out.Parent)
				}
				if out.Operation != "aabbccddeeff00112233445566778899" {
					t.Errorf("expected Operation preserved, got %q", out.Operation)
				}
				if out.Run != "deadbeefdeadbeefdeadbeefdeadbeef" {
					t.Errorf("expected Run preserved, got %q", out.Run)
				}
				if out.ID != "11223344556677889900aabbccddeeff" {
					t.Errorf("expected ID preserved, got %q", out.ID)
				}
			},
		},
		{
			name: "paths and URLs in Step, Build, Version redacted while safe tokens kept",
			input: Event{
				Step:    "/usr/local/bin/run.sh",
				Build:   "https://ci.internal.net/build/12345?token=secret",
				Version: "C:\\Users\\admin\\app.exe",
			},
			validate: func(t *testing.T, out Event) {
				if out.Step != "[redacted]" {
					t.Errorf("expected Step path to be [redacted], got %q", out.Step)
				}
				if out.Build != "[redacted]" {
					t.Errorf("expected Build URL to be [redacted], got %q", out.Build)
				}
				if out.Version != "[redacted]" {
					t.Errorf("expected Version path to be [redacted], got %q", out.Version)
				}
			},
		},
		{
			name: "safe token strings in Step, Build, Version preserved",
			input: Event{
				Step:    "01-executor_step.v2",
				Build:   "git:5716a4f-v1.0.0",
				Version: "v1.2.3-alpha:test",
			},
			validate: func(t *testing.T, out Event) {
				if out.Step != "01-executor_step.v2" {
					t.Errorf("expected Step preserved, got %q", out.Step)
				}
				if out.Build != "git:5716a4f-v1.0.0" {
					t.Errorf("expected Build preserved, got %q", out.Build)
				}
				if out.Version != "v1.2.3-alpha:test" {
					t.Errorf("expected Version preserved, got %q", out.Version)
				}
			},
		},
		{
			name: "business identity fields preserved while freeform technical message is overwritten",
			input: Event{
				Workspace: "workspace-enterprise-99",
				Account:   "acc-user-42",
				Actor:     "usr_1001",
				ObjectID:  "obj_doc_555",
				Message:   "MALICIOUS_SQL_INJECTION; DROP TABLE users; --",
				Code:      "NETWORK_UNAVAILABLE",
			},
			validate: func(t *testing.T, out Event) {
				// Business identity preserved
				if out.Workspace != "workspace-enterprise-99" {
					t.Errorf("expected Workspace preserved, got %q", out.Workspace)
				}
				if out.Account != "acc-user-42" {
					t.Errorf("expected Account preserved, got %q", out.Account)
				}
				if out.Actor != "usr_1001" {
					t.Errorf("expected Actor preserved, got %q", out.Actor)
				}
				if out.ObjectID != "obj_doc_555" {
					t.Errorf("expected ObjectID preserved, got %q", out.ObjectID)
				}
				// Freeform technical message MUST be overwritten by mapped code message
				if out.Message != "Connection interrupted" {
					t.Errorf("expected Message to be mapped to 'Connection interrupted', got %q", out.Message)
				}
				if out.Next != "retry_simulation" || out.Retryable != true {
					t.Errorf("expected retry_simulation and retryable true, got next=%s retryable=%v", out.Next, out.Retryable)
				}
			},
		},
		{
			name: "code-to-next mappings for security and failure scenarios",
			input: Event{
				Code: "AUTHORIZATION_DENIED",
			},
			validate: func(t *testing.T, out Event) {
				if out.Next != "check_authorization" || out.Retryable != false {
					t.Errorf("expected check_authorization and non-retryable, got next=%s retryable=%v", out.Next, out.Retryable)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sanitized := Sanitize(tc.input)
			tc.validate(t, sanitized)
		})
	}
}

// TestLogRegressionSlogHandlerLeakingPrevention verifies that slog attributes, messages,
// WithAttrs, and WithGroup cannot leak sensitive data into the LogBuffer.
func TestLogRegressionSlogHandlerLeakingPrevention(t *testing.T) {
	buf := NewLogBuffer(10)
	handler := SlogHandler{Buffer: buf}

	// Test WithAttrs and WithGroup chaining
	wrappedHandler := handler.WithAttrs([]slog.Attr{
		slog.String("secret_api_key", "SECRET-KEY-11223344"),
		slog.String("bearer_token", "Bearer eyJhbGciOi..."),
	}).WithGroup("sensitive_group")

	logger := slog.New(wrappedHandler)

	// Log with sensitive message and deeply nested attributes
	logger.ErrorContext(
		context.Background(),
		"Sensitive prompt text containing PRIVATE_USER_DATA and password=SUPER_SECRET",
		slog.String("password", "SUPER_SECRET"),
		slog.String("sql_query", "SELECT * FROM secret_credentials"),
		slog.Any("user_profile", map[string]string{
			"email":    "user@private-domain.internal",
			"passport": "PASS_998877",
		}),
		// Legitimate allowed attributes
		slog.String("error_code", "MODEL_QUOTA"),
		slog.String("component", "executor"),
		slog.String("trace_id", "0123456789abcdef0123456789abcdef"),
	)

	events := buf.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event in buffer, got %d", len(events))
	}

	wire, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("failed to marshal events: %v", err)
	}
	serialized := string(wire)

	// Check sensitive markers never leak
	leakCheckList := []string{
		"SECRET-KEY",
		"Bearer",
		"PRIVATE_USER_DATA",
		"SUPER_SECRET",
		"secret_credentials",
		"user@private-domain.internal",
		"PASS_998877",
		"sensitive_group",
	}

	for _, token := range leakCheckList {
		if strings.Contains(serialized, token) {
			t.Errorf("found leaked sensitive token %q in serialized log events: %s", token, serialized)
		}
	}

	// Verify allowed fields were correctly transferred and sanitized
	ev := events[0]
	if ev.Code != "MODEL_QUOTA" {
		t.Errorf("expected Code to be MODEL_QUOTA, got %q", ev.Code)
	}
	if ev.Message != "Model quota exhausted" {
		t.Errorf("expected Message to be 'Model quota exhausted', got %q", ev.Message)
	}
	if ev.Component != "executor" {
		t.Errorf("expected Component to be 'executor', got %q", ev.Component)
	}
	if ev.Trace != "0123456789abcdef0123456789abcdef" {
		t.Errorf("expected Trace to be preserved, got %q", ev.Trace)
	}
	if ev.Severity != "error" {
		t.Errorf("expected Severity to be 'error', got %q", ev.Severity)
	}
	if ev.Next != "check_local_client" {
		t.Errorf("expected Next to be 'check_local_client', got %q", ev.Next)
	}
}

// TestLogRegressionBufferCapacities verifies boundary behaviors:
// - capacity <= 0 defaults to 1
// - capacity 1 drops earlier entries correctly
// - FIFO eviction order and Dropped atomic counter
// - Events() returns an independent copy of the slice
func TestLogRegressionBufferCapacities(t *testing.T) {
	t.Run("zero or negative capacity defaults to 1", func(t *testing.T) {
		for _, capVal := range []int{0, -1, -999} {
			buf := NewLogBuffer(capVal)
			buf.Append(Event{Code: "TIMEOUT"})
			buf.Append(Event{Code: "CANCELLED"})

			evs := buf.Events()
			if len(evs) != 1 {
				t.Errorf("expected capacity 1 for input %d, got %d events", capVal, len(evs))
			}
			if buf.Dropped.Load() != 1 {
				t.Errorf("expected 1 dropped for input %d, got %d", capVal, buf.Dropped.Load())
			}
			if evs[0].Code != "CANCELLED" {
				t.Errorf("expected retained event to be CANCELLED, got %s", evs[0].Code)
			}
		}
	})

	t.Run("capacity 1 FIFO eviction and dropped count", func(t *testing.T) {
		buf := NewLogBuffer(1)
		buf.Append(Event{Step: "step-1", Code: "TIMEOUT"})
		if len(buf.Events()) != 1 || buf.Dropped.Load() != 0 {
			t.Fatalf("unexpected state after 1st append")
		}

		buf.Append(Event{Step: "step-2", Code: "CANCELLED"})
		if len(buf.Events()) != 1 || buf.Dropped.Load() != 1 {
			t.Fatalf("unexpected state after 2nd append: dropped=%d", buf.Dropped.Load())
		}
		if buf.Events()[0].Step != "step-2" {
			t.Fatalf("expected step-2, got %s", buf.Events()[0].Step)
		}

		buf.Append(Event{Step: "step-3", Code: "NETWORK_UNAVAILABLE"})
		if len(buf.Events()) != 1 || buf.Dropped.Load() != 2 {
			t.Fatalf("unexpected state after 3rd append: dropped=%d", buf.Dropped.Load())
		}
		if buf.Events()[0].Step != "step-3" {
			t.Fatalf("expected step-3, got %s", buf.Events()[0].Step)
		}
	})

	t.Run("bounded FIFO eviction order on larger capacity", func(t *testing.T) {
		buf := NewLogBuffer(3)
		for i := 1; i <= 5; i++ {
			buf.Append(Event{Step: fmt.Sprintf("step-%d", i), Code: "TIMEOUT"})
		}

		if buf.Dropped.Load() != 2 {
			t.Errorf("expected 2 dropped events, got %d", buf.Dropped.Load())
		}

		events := buf.Events()
		if len(events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(events))
		}
		expectedSteps := []string{"step-3", "step-4", "step-5"}
		for i, exp := range expectedSteps {
			if events[i].Step != exp {
				t.Errorf("event[%d] step mismatch: expected %s, got %s", i, exp, events[i].Step)
			}
		}
	})

	t.Run("Events returns independent copy", func(t *testing.T) {
		buf := NewLogBuffer(3)
		buf.Append(Event{Step: "initial-step", Code: "TIMEOUT"})

		evs1 := buf.Events()
		if len(evs1) != 1 {
			t.Fatalf("expected 1 event")
		}

		// Mutate caller's slice and struct
		evs1[0].Step = "mutated-by-caller"
		evs1 = append(evs1, Event{Step: "appended-by-caller"})

		// Buffer internal state must not be affected
		evs2 := buf.Events()
		if len(evs2) != 1 {
			t.Fatalf("internal buffer length modified by caller")
		}
		if evs2[0].Step != "initial-step" {
			t.Fatalf("internal buffer event content modified by caller: %s", evs2[0].Step)
		}
	})
}

// TestLogRegressionConcurrentAppendAndEvents verifies thread-safety under race detector:
// - Multiple concurrent goroutines calling Append and Events
// - Buffer capacity bound is strictly maintained
// - Total Appends == Remaining in buffer + Dropped count
// - No race conditions detected
func TestLogRegressionConcurrentAppendAndEvents(t *testing.T) {
	const (
		capacity    = 25
		goroutines  = 10
		opsPerG     = 100
		totalEvents = goroutines * opsPerG
	)

	buf := NewLogBuffer(capacity)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(routineID int) {
			defer wg.Done()
			for i := 0; i < opsPerG; i++ {
				// Alternately call Append and Events
				e := Event{
					Step:      fmt.Sprintf("g%d-op%d", routineID, i),
					Code:      "TIMEOUT",
					Component: "queue",
					Severity:  "info",
					Occurred:  time.Now().UTC(),
				}
				buf.Append(e)

				if i%5 == 0 {
					readEvs := buf.Events()
					if len(readEvs) > capacity {
						t.Errorf("readEvs length %d exceeds capacity %d", len(readEvs), capacity)
					}
				}
			}
		}(g)
	}

	wg.Wait()

	finalEvents := buf.Events()
	dropped := buf.Dropped.Load()

	if len(finalEvents) != capacity {
		t.Fatalf("expected exactly %d events in final buffer, got %d", capacity, len(finalEvents))
	}

	if int64(len(finalEvents))+dropped != int64(totalEvents) {
		t.Fatalf("event accounting mismatch: len(finalEvents)=%d + dropped=%d != totalEvents=%d",
			len(finalEvents), dropped, totalEvents)
	}
}
