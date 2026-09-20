package dbtest

import (
	"strings"
	"testing"
)

func TestTwoScopesFromOneRunDoNotCollide(t *testing.T) {
	cfg := Config{RunID: "12345_1", Suite: SuiteHandler}

	first := NewSuiteScope(cfg)
	second := NewSuiteScope(cfg)

	// Same run identity, different names: a rerun of one CI attempt, or two
	// packages sharing a database, must not reuse a fixture name.
	if first.Email("root") == second.Email("root") {
		t.Error("two scopes from the same run produced the same email")
	}
	if first.Slug("ws") == second.Slug("ws") {
		t.Error("two scopes from the same run produced the same slug")
	}
}

func TestScopeNamesAreSafeToStore(t *testing.T) {
	scope := NewSuiteScope(Config{RunID: "12345_1", Suite: SuiteCmdServer})

	email := scope.Email("integration-test")
	if !strings.HasSuffix(email, "@tests.invalid") {
		t.Errorf("email %q is not in the reserved test domain", email)
	}
	if strings.ContainsAny(email, " '\"\\;\t") {
		t.Errorf("email %q contains a character that has no business in a fixture name", email)
	}

	slug := scope.Slug("integration-tests")
	for _, r := range slug {
		if !(r == '-' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			t.Errorf("slug %q contains %q, which is outside lowercase, digits and dashes", slug, r)
		}
	}
}

func TestScopeStringNamesTheRunNotATarget(t *testing.T) {
	scope := NewSuiteScope(Config{RunID: "12345_1", Suite: SuiteHandler})

	described := scope.String()
	if !strings.Contains(described, "12345_1") || !strings.Contains(described, SuiteHandler) {
		t.Errorf("String() omitted the run identity: %s", described)
	}
}
