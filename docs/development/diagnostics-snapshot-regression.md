# Snapshot Regression Tests (Loretide Diagnostics)

This document describes the non-UI regression tests for `diagnostics.Snapshot`, `CloneSnapshot`, and `ReproductionGaps` in `server/internal/content/diagnostics/snapshot_regression_test.go`.

These tests cover the audit and reproduction integrity guarantees required by Issue #15 (DIAG-SNAPSHOT-TEST-01).

## Scope & Boundaries

- **File under test**: `server/internal/content/diagnostics/contract.go` (`Snapshot`, `CloneSnapshot`, `ReproductionGaps`, JSON codecs).
- **Test file**: `server/internal/content/diagnostics/snapshot_regression_test.go`
- **Exclusions**: Does not include database transactions, HTTP export endpoints, or real provider / external service execution (which belong to Issue #2).

## Covered Behaviors and Assertions

### 1. Snapshot Deep-Copy Isolation (`TestSnapshotRegressionCloneIsolation`)
- **Bidirectional Mutation Isolation**: Confirms mutating slices (`Required`, `Excluded`, `Grants`) and map (`Hashes`) in the original snapshot does not mutate the cloned snapshot, and vice versa.
- **Fidelity of Metadata & Scalars**: Verifies all version tags (`ConfigVersion`, `SOPVersion`, `SkillVersion`, `RuleVersion`, `ExecutorVersion`), context references (`PersonaRef`, `Scope`, `Preference`), and execution limits (`Temperature`, `Budget`, `Timeout`) are preserved exactly.
- **Empty and Nil Collections**: Verifies that empty non-nil slices/maps and nil collections clone cleanly without panics.

### 2. Reproduction Gap Analysis (`TestSnapshotRegressionReproductionGaps`)
- **Combinations**:
  - Baseline match (zero gaps when hashes and grants match).
  - Single missing file (`FILE_MISSING:<file_id>`).
  - Single changed file (`FILE_CHANGED:<file_id>`).
  - Single revoked grant (`AUTHORIZATION_REVOKED:<grant_id>`).
  - Simultaneous multiple missing, changed, and revoked items.
  - Empty snapshot against empty or populated current environments.
- **Order-Independent Content Assertions**: Compares the exact gap token strings sorted, rather than relying on Go map traversal iteration order or checking only `len(gaps)`.

### 3. Input Immutability (`TestSnapshotRegressionInputImmutability`)
- Confirms that evaluating reproduction gaps does not modify the original snapshot, the current file hashes map, or the current grants map.
- Revocation cannot be undone or masked by stale permissions in the snapshot.

### 4. JSON Serialization Fidelity & Unknown Field Tolerance (`TestSnapshotRegressionJSONRoundtrip`)
- Ensures complete roundtrip fidelity across JSON marshaling and unmarshaling.
- Validates graceful handling of unknown future fields in accordance with diagnostic contract backwards compatibility.

## Running Tests

From `server/`:

```bash
# Standard run
go test -v ./internal/content/diagnostics -run TestSnapshotRegression -count=1

# Race detection
go test -race -v ./internal/content/diagnostics -run TestSnapshotRegression -count=1
```
