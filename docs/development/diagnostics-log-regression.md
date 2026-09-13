# Diagnostics Log Sanitization & Bounded Buffer Regression Tests

This document describes the non-UI regression tests for event sanitization, structured logging leakage prevention, and bounded log buffer concurrency guarantees in `server/internal/content/diagnostics/log_regression_test.go`.

These tests fulfill the regression requirements for Issue #13 (DIAG-LOG-TEST-01).

## Scope & Boundaries

- **File under test**: `server/internal/content/diagnostics/log.go` (`Sanitize`, `LogBuffer`, `SlogHandler`).
- **Test file**: `server/internal/content/diagnostics/log_regression_test.go`
- **Exclusions**: Does not alter production logger setup, does not import or touch external log collectors, and maintains strict separation from database persistence layers.

## Covered Behaviors and Assertions

### 1. Table-Driven Sanitization Rules (`TestLogRegressionSanitizeRules`)
- **Unknown Error Codes and Enums**: Unknown `Code` normalizes to `"INTERNAL"` and maps `Message` to `"Internal error"`. Unknown values for `Component`, `Severity`, `ActorKind`, `Outcome`, `Action`, and `ObjectType` normalize to `"unknown"`.
- **Malformed Identifiers**: Invalid non-hex trace/span/parent/operation/run/id values (or non-16/32 char lengths) are cleared to `""`. Valid hex IDs are preserved.
- **Path and URL Redaction**: Steps, Builds, or Versions containing slash, colon, URL, or filesystem path characters outside the safe token character set (`^[a-zA-Z0-9_.:-]{0,100}$`) are redacted to `"[redacted]"`.
- **Business Identity vs. Technical Messages**: Workspace, Account, Actor, and ObjectID identity fields are preserved for multi-tenant routing, while untrusted, potentially malicious technical message bodies are systematically replaced by allowlisted safe descriptions matching the code.

### 2. Slog Structured Logging Leakage Prevention (`TestLogRegressionSlogHandlerLeakingPrevention`)
- Verifies that synthetic secrets placed in the log message, arbitrary attributes (`password`, `sql_query`, `user_profile`), `WithAttrs`, or `WithGroup` are discarded and never appear in serialized JSON output.
- Only allowlisted finite enum fields (`error_code`, `component`, `trace_id`) are mapped onto the `Event`.

### 3. Boundary Buffer Operations (`TestLogRegressionBufferCapacities`)
- Capacity values <= 0 safely default to minimum capacity of 1.
- Exact FIFO eviction and atomic `Dropped` incrementing under overflow.
- `Events()` returns an independent slice copy to prevent caller mutation of internal buffer state.

### 4. Concurrency Safety Under Race Detector (`TestLogRegressionConcurrentAppendAndEvents`)
- Multi-goroutine concurrent execution of `Append` and `Events` passes under `-race`.
- Exact event balance accounting: `len(finalEvents) + Dropped == totalAppends`.

## Running Tests

From `server/`:

```bash
# Direct regression run
go test -v ./internal/content/diagnostics -run TestLogRegression -count=1

# Race detector
go test -race -v ./internal/content/diagnostics -run TestLogRegression -count=1
```
