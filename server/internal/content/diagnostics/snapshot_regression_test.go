package diagnostics

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// TestSnapshotRegressionCloneIsolation verifies that CloneSnapshot performs deep copy
// on all slice and map fields (Required, Excluded, Grants, Hashes), preserves scalar,
// version, scope, and preference fields, and handles nil/empty collections properly.
func TestSnapshotRegressionCloneIsolation(t *testing.T) {
	t.Run("deep copy mutation isolation across all collections", func(t *testing.T) {
		orig := Snapshot{
			ConfigVersion:   "cfg-v1.2",
			PersonaRef:      "persona:editor-in-chief",
			SOPVersion:      "sop-v3.0",
			SkillVersion:    "skill-v2.1",
			RuleVersion:     "rule-v1.0",
			Executor:        "simulator",
			ExecutorVersion: "v0.9.4",
			Scope:           "workspace-shared",
			Preference:      "strict-audit",
			Required:        []string{"src-a", "src-b"},
			Excluded:        []string{"src-x"},
			Grants:          []string{"grant-read", "grant-exec"},
			Hashes: map[string]string{
				"file1.go": "sha256:1111",
				"file2.go": "sha256:2222",
			},
			Temperature: 0.7,
			Budget:      1000,
			Timeout:     15000,
		}

		cloned := CloneSnapshot(orig)

		// Verify initial equality of fields
		if cloned.ConfigVersion != orig.ConfigVersion ||
			cloned.PersonaRef != orig.PersonaRef ||
			cloned.SOPVersion != orig.SOPVersion ||
			cloned.SkillVersion != orig.SkillVersion ||
			cloned.RuleVersion != orig.RuleVersion ||
			cloned.Executor != orig.Executor ||
			cloned.ExecutorVersion != orig.ExecutorVersion ||
			cloned.Scope != orig.Scope ||
			cloned.Preference != orig.Preference ||
			cloned.Temperature != orig.Temperature ||
			cloned.Budget != orig.Budget ||
			cloned.Timeout != orig.Timeout {
			t.Fatalf("scalar/version fields mismatch between original and cloned: orig=%+v cloned=%+v", orig, cloned)
		}

		// Mutate original slice and map contents
		orig.Required[0] = "mutated-src-a"
		orig.Required = append(orig.Required, "mutated-extra")
		orig.Excluded[0] = "mutated-src-x"
		orig.Grants[0] = "mutated-grant"
		orig.Hashes["file1.go"] = "sha256:mutated"
		orig.Hashes["file3.new"] = "sha256:3333"

		// Cloned snapshot must NOT be affected by mutations on original
		if cloned.Required[0] != "src-a" || len(cloned.Required) != 2 {
			t.Errorf("cloned.Required was mutated by changes to original: %v", cloned.Required)
		}
		if cloned.Excluded[0] != "src-x" || len(cloned.Excluded) != 1 {
			t.Errorf("cloned.Excluded was mutated by changes to original: %v", cloned.Excluded)
		}
		if cloned.Grants[0] != "grant-read" || len(cloned.Grants) != 2 {
			t.Errorf("cloned.Grants was mutated by changes to original: %v", cloned.Grants)
		}
		if cloned.Hashes["file1.go"] != "sha256:1111" || len(cloned.Hashes) != 2 {
			t.Errorf("cloned.Hashes was mutated by changes to original: %v", cloned.Hashes)
		}
		if _, exists := cloned.Hashes["file3.new"]; exists {
			t.Errorf("cloned.Hashes received newly added key from original")
		}

		// Now mutate cloned slice and map contents
		cloned.Required[1] = "mutated-in-clone"
		cloned.Excluded = append(cloned.Excluded, "mutated-clone-excluded")
		cloned.Grants[1] = "mutated-clone-grant"
		cloned.Hashes["file2.go"] = "sha256:mutated-in-clone"
		delete(cloned.Hashes, "file1.go")

		// Original must NOT be affected by mutations on cloned
		if orig.Required[1] != "src-b" {
			t.Errorf("orig.Required was mutated by changes to clone: %v", orig.Required)
		}
		if len(orig.Excluded) != 1 {
			t.Errorf("orig.Excluded length mutated by clone: %v", orig.Excluded)
		}
		if orig.Grants[1] != "grant-exec" {
			t.Errorf("orig.Grants was mutated by changes to clone: %v", orig.Grants)
		}
		if orig.Hashes["file2.go"] != "sha256:2222" {
			t.Errorf("orig.Hashes was mutated by changes to clone: %v", orig.Hashes)
		}
		if _, exists := orig.Hashes["file1.go"]; !exists {
			t.Errorf("orig.Hashes lost key deleted in clone")
		}
	})

	t.Run("empty and nil collections handling", func(t *testing.T) {
		emptySnapshot := Snapshot{
			ConfigVersion: "v1",
			Required:      []string{},
			Excluded:      []string{},
			Grants:        []string{},
			Hashes:        map[string]string{},
		}
		clonedEmpty := CloneSnapshot(emptySnapshot)
		if clonedEmpty.Required == nil || len(clonedEmpty.Required) != 0 {
			t.Errorf("expected empty non-nil Required slice, got %v", clonedEmpty.Required)
		}
		if clonedEmpty.Excluded == nil || len(clonedEmpty.Excluded) != 0 {
			t.Errorf("expected empty non-nil Excluded slice, got %v", clonedEmpty.Excluded)
		}
		if clonedEmpty.Grants == nil || len(clonedEmpty.Grants) != 0 {
			t.Errorf("expected empty non-nil Grants slice, got %v", clonedEmpty.Grants)
		}
		if clonedEmpty.Hashes == nil || len(clonedEmpty.Hashes) != 0 {
			t.Errorf("expected empty non-nil Hashes map, got %v", clonedEmpty.Hashes)
		}

		nilSnapshot := Snapshot{
			ConfigVersion: "v0",
			Required:      nil,
			Excluded:      nil,
			Grants:        nil,
			Hashes:        nil,
		}
		clonedNil := CloneSnapshot(nilSnapshot)
		if len(clonedNil.Required) != 0 || len(clonedNil.Excluded) != 0 || len(clonedNil.Grants) != 0 || len(clonedNil.Hashes) != 0 {
			t.Errorf("expected empty/nil lengths, got req=%d excl=%d grants=%d hashes=%d",
				len(clonedNil.Required), len(clonedNil.Excluded), len(clonedNil.Grants), len(clonedNil.Hashes))
		}
	})
}

// TestSnapshotRegressionReproductionGaps verifies gap detection across all combinations of:
// no changes, missing files, changed hashes, revoked grants, and simultaneous occurrences.
// Asserts exact gap contents (order-independent) rather than only counts.
func TestSnapshotRegressionReproductionGaps(t *testing.T) {
	testCases := []struct {
		name         string
		snapshot     Snapshot
		current      map[string]string
		grants       map[string]bool
		expectedGaps []string
	}{
		{
			name: "no changes (baseline match)",
			snapshot: Snapshot{
				Grants: []string{"grant-1", "grant-2"},
				Hashes: map[string]string{
					"doc.md":    "hash:111",
					"schema.go": "hash:222",
				},
			},
			current: map[string]string{
				"doc.md":    "hash:111",
				"schema.go": "hash:222",
			},
			grants: map[string]bool{
				"grant-1": true,
				"grant-2": true,
			},
			expectedGaps: []string{},
		},
		{
			name: "single missing file",
			snapshot: Snapshot{
				Grants: []string{"grant-1"},
				Hashes: map[string]string{
					"doc.md":    "hash:111",
					"absent.go": "hash:333",
				},
			},
			current: map[string]string{
				"doc.md": "hash:111",
				// absent.go is missing
			},
			grants: map[string]bool{
				"grant-1": true,
			},
			expectedGaps: []string{
				"FILE_MISSING:absent.go",
			},
		},
		{
			name: "single changed file",
			snapshot: Snapshot{
				Grants: []string{"grant-1"},
				Hashes: map[string]string{
					"doc.md": "hash:111",
				},
			},
			current: map[string]string{
				"doc.md": "hash:999-different",
			},
			grants: map[string]bool{
				"grant-1": true,
			},
			expectedGaps: []string{
				"FILE_CHANGED:doc.md",
			},
		},
		{
			name: "single revoked grant",
			snapshot: Snapshot{
				Grants: []string{"grant-1", "grant-2"},
				Hashes: map[string]string{
					"doc.md": "hash:111",
				},
			},
			current: map[string]string{
				"doc.md": "hash:111",
			},
			grants: map[string]bool{
				"grant-1": true,
				"grant-2": false, // revoked
			},
			expectedGaps: []string{
				"AUTHORIZATION_REVOKED:grant-2",
			},
		},
		{
			name: "simultaneous missing, changed, and revoked gaps",
			snapshot: Snapshot{
				Grants: []string{"auth-read", "auth-write", "auth-admin"},
				Hashes: map[string]string{
					"stable.txt":  "hash:stable",
					"changed.txt": "hash:orig",
					"deleted.txt": "hash:deleted",
				},
			},
			current: map[string]string{
				"stable.txt":  "hash:stable",
				"changed.txt": "hash:modified",
				// deleted.txt missing
			},
			grants: map[string]bool{
				"auth-read":  true,
				"auth-write": false, // revoked
				// auth-admin missing entirely from map (evaluates to false)
			},
			expectedGaps: []string{
				"AUTHORIZATION_REVOKED:auth-write",
				"AUTHORIZATION_REVOKED:auth-admin",
				"FILE_CHANGED:changed.txt",
				"FILE_MISSING:deleted.txt",
			},
		},
		{
			name: "empty snapshot has zero gaps against empty or populated current environment",
			snapshot: Snapshot{
				Grants: []string{},
				Hashes: map[string]string{},
			},
			current: map[string]string{
				"unrelated.go": "hash:any",
			},
			grants: map[string]bool{
				"unrelated-grant": true,
			},
			expectedGaps: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualGaps := ReproductionGaps(tc.snapshot, tc.current, tc.grants)

			// Sort both slices to make comparison order-independent
			sortedActual := append([]string(nil), actualGaps...)
			sortedExpected := append([]string(nil), tc.expectedGaps...)
			sort.Strings(sortedActual)
			sort.Strings(sortedExpected)

			if !reflect.DeepEqual(sortedActual, sortedExpected) {
				t.Fatalf("ReproductionGaps mismatch:\nexpected: %v\ngot:      %v", sortedExpected, sortedActual)
			}
		})
	}
}

// TestSnapshotRegressionInputImmutability verifies that calling ReproductionGaps does NOT mutate
// the input Snapshot, the current hashes map, or the grants map, and that revoked grants
// cannot be resurrected or modified by stale snapshot values.
func TestSnapshotRegressionInputImmutability(t *testing.T) {
	origSnapshot := Snapshot{
		ConfigVersion: "v1",
		Grants:        []string{"grant-alpha", "grant-beta"},
		Hashes: map[string]string{
			"file-a": "hash-a",
			"file-b": "hash-b",
		},
	}
	snapshotCopy := CloneSnapshot(origSnapshot)

	currentHashes := map[string]string{
		"file-a": "hash-a",
		"file-b": "hash-modified",
	}
	initialCurrentHashes := map[string]string{
		"file-a": "hash-a",
		"file-b": "hash-modified",
	}

	grants := map[string]bool{
		"grant-alpha": true,
		"grant-beta":  false, // explicitly revoked
	}
	initialGrants := map[string]bool{
		"grant-alpha": true,
		"grant-beta":  false,
	}

	// First evaluation
	gaps1 := ReproductionGaps(origSnapshot, currentHashes, grants)
	if len(gaps1) != 2 {
		t.Fatalf("expected 2 gaps, got %d (%v)", len(gaps1), gaps1)
	}

	// Verify Snapshot was not mutated
	if !reflect.DeepEqual(origSnapshot, snapshotCopy) {
		t.Fatalf("origSnapshot was modified by ReproductionGaps:\nbefore: %+v\nafter:  %+v", snapshotCopy, origSnapshot)
	}

	// Verify currentHashes was not mutated
	if !reflect.DeepEqual(currentHashes, initialCurrentHashes) {
		t.Fatalf("currentHashes was modified by ReproductionGaps:\nbefore: %v\nafter:  %v", initialCurrentHashes, currentHashes)
	}

	// Verify grants map was not mutated (revocation is persistent)
	if !reflect.DeepEqual(grants, initialGrants) {
		t.Fatalf("grants map was modified by ReproductionGaps:\nbefore: %v\nafter:  %v", initialGrants, grants)
	}
	if grants["grant-beta"] != false {
		t.Fatal("revoked grant-beta was improperly toggled to true")
	}

	// Second evaluation must produce deterministic results
	gaps2 := ReproductionGaps(origSnapshot, currentHashes, grants)
	sort.Strings(gaps1)
	sort.Strings(gaps2)
	if !reflect.DeepEqual(gaps1, gaps2) {
		t.Fatalf("ReproductionGaps is not deterministic across multiple calls: %v vs %v", gaps1, gaps2)
	}
}

// TestSnapshotRegressionJSONRoundtrip verifies serialization roundtrip fidelity for all Snapshot
// fields, and verifies contract compliance regarding unknown future fields in Run envelopes.
func TestSnapshotRegressionJSONRoundtrip(t *testing.T) {
	t.Run("all Snapshot fields preserve fidelity across JSON serialization", func(t *testing.T) {
		expected := Snapshot{
			ConfigVersion:   "cfg-v2.0.0",
			PersonaRef:      "persona:tech-reviewer",
			SOPVersion:      "sop-v1.4",
			SkillVersion:    "skill-v3.0",
			RuleVersion:     "rule-v2.5",
			Executor:        "simulator",
			ExecutorVersion: "v1.0-loretide",
			Scope:           "workspace-content",
			Preference:      "reproducible-diagnostics",
			Required:        []string{"spec.md", "guidelines.txt"},
			Excluded:        []string{"secret.env", "credentials.json"},
			Grants:          []string{"role:diagnostics", "perm:audit"},
			Hashes: map[string]string{
				"spec.md":        "sha256:abc1234",
				"guidelines.txt": "sha256:def5678",
			},
			Temperature: 0.25,
			Budget:      5000,
			Timeout:     60000,
		}

		wire, err := json.Marshal(expected)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		var decoded Snapshot
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		if !reflect.DeepEqual(decoded, expected) {
			t.Fatalf("JSON roundtrip lost fidelity:\nexpected: %+v\ngot:      %+v", expected, decoded)
		}
	})

	t.Run("Run and Snapshot handle unknown future fields gracefully", func(t *testing.T) {
		raw := `{
			"config_version": "v1.0",
			"persona_ref": "ref:1",
			"unknown_future_field": "compatible_value",
			"another_extension": {"nested": 123},
			"required_sources": ["a"],
			"file_hashes": {"a": "sha256:111"}
		}`

		var s Snapshot
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			t.Fatalf("Unmarshal with unknown future fields failed: %v", err)
		}
		if s.ConfigVersion != "v1.0" || len(s.Required) != 1 || s.Hashes["a"] != "sha256:111" {
			t.Fatalf("recognized fields corrupted by presence of unknown fields: %+v", s)
		}
	})
}
