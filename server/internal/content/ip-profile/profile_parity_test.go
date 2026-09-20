package ipprofile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The Go half of the readiness parity contract.
//
// packages/core carries its own copy of this decision so EP-04's start screen
// can answer "can this account start yet" without a round trip. Two copies of
// a rule is how the two drift, so both run against one committed matrix:
// specs/021-account-expression-profile/contracts/readiness-parity.json.
//
// It compares DECISIONS rather than names. A rule changed on one side without
// the other turns one of these two suites red, and the file itself is small
// enough to review as a table.

type parityCase struct {
	Name     string            `json:"name"`
	Profile  ExpressionProfile `json:"profile"`
	CanStart bool              `json:"can_start"`
	Missing  []string          `json:"missing"`
	Neutral  bool              `json:"neutral"`
}

type parityDocument struct {
	Cases []parityCase `json:"cases"`
}

func loadParityMatrix(t *testing.T) parityDocument {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..",
		"specs", "021-account-expression-profile", "contracts", "readiness-parity.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the parity matrix: %v", err)
	}
	var doc parityDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse the parity matrix: %v", err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("the parity matrix is empty, so it proves nothing")
	}
	return doc
}

func TestReadinessMatchesTheParityMatrix(t *testing.T) {
	for _, tc := range loadParityMatrix(t).Cases {
		t.Run(tc.Name, func(t *testing.T) {
			readiness := ProfileReadiness(tc.Profile)

			if readiness.CanStart != tc.CanStart {
				t.Errorf("can_start = %v, want %v (missing %v)",
					readiness.CanStart, tc.CanStart, readiness.Missing)
			}
			if len(readiness.Missing) != len(tc.Missing) {
				t.Fatalf("missing = %v, want %v", readiness.Missing, tc.Missing)
			}
			// Order is part of the contract: a page renders the list in order.
			for i, want := range tc.Missing {
				if readiness.Missing[i] != want {
					t.Errorf("missing[%d] = %q, want %q", i, readiness.Missing[i], want)
				}
			}
		})
	}
}

func TestNeutralExpressionMatchesTheParityMatrix(t *testing.T) {
	for _, tc := range loadParityMatrix(t).Cases {
		t.Run(tc.Name, func(t *testing.T) {
			if got := UsesNeutralExpression(tc.Profile); got != tc.Neutral {
				t.Errorf("uses neutral expression = %v, want %v", got, tc.Neutral)
			}
		})
	}
}

// The matrix has to exercise both outcomes of each decision, or a rule could
// be inverted on both sides at once and still agree with it.
func TestTheParityMatrixCoversBothOutcomes(t *testing.T) {
	cases := loadParityMatrix(t).Cases

	var canStart, cannotStart, neutral, notNeutral int
	missingSeen := map[string]bool{}
	for _, tc := range cases {
		if tc.CanStart {
			canStart++
		} else {
			cannotStart++
		}
		if tc.Neutral {
			neutral++
		} else {
			notNeutral++
		}
		for _, name := range tc.Missing {
			missingSeen[name] = true
		}
	}

	if canStart == 0 || cannotStart == 0 {
		t.Errorf("the matrix has %d startable and %d not-startable cases; it needs both",
			canStart, cannotStart)
	}
	if neutral == 0 || notNeutral == 0 {
		t.Errorf("the matrix has %d neutral and %d non-neutral cases; it needs both",
			neutral, notNeutral)
	}
	for _, name := range []string{MissingAudience, MissingPillars, MissingChannels, MissingWeeklyHours} {
		if !missingSeen[name] {
			t.Errorf("no case in the matrix reports %q as missing", name)
		}
	}
}
