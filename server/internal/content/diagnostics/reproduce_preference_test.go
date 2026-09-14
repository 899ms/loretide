package diagnostics

import (
	"context"
	"testing"
)

// Reproducing a past run must not write anything back onto it, and in
// particular must not overwrite the saved preference it recorded (D13-V10).
//
// simulator_test.go already checks that Simulate leaves Preference at "all" -
// but "all" is exactly what the fixture puts there, so that assertion cannot
// tell "nothing was written" apart from "something was written and happened to
// match". Every run below therefore uses a preference no fixture produces.
const nonFixturePreference = "strict-sources-only"

func reproduceService(t *testing.T) *Service {
	t.Helper()
	return &Service{Store: testStore(t), Build: "test", Enabled: true}
}

// Commits a run whose snapshot carries a preference the fixture never writes,
// so a later comparison is meaningful.
func originalRunWithPreference(t *testing.T, s *Service, ctx context.Context, scope Scope) Run {
	t.Helper()
	run, err := Simulate(ctx, scope, "", "timeout", 3, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	run.Snapshot.Preference = nonFixturePreference
	run.Snapshot.Scope = "selected"
	Evaluate(&run)
	if err = s.Store.CommitRun(ctx, scope, run, false); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.GetRun(ctx, scope, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Snapshot.Preference != nonFixturePreference {
		t.Fatalf("setup did not persist the preference: got %q", stored.Snapshot.Preference)
	}
	return stored
}

func TestReproduceDoesNotWriteBackTheSavedPreference(t *testing.T) {
	s := reproduceService(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u"}
	original := originalRunWithPreference(t, s, ctx, scope)

	if _, err := s.Run(ctx, scope, "", "timeout", 9, original.ID); err != nil {
		t.Fatal(err)
	}

	after, err := s.Store.GetRun(ctx, scope, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Snapshot.Preference != nonFixturePreference {
		t.Errorf("reproducing rewrote the original's saved preference: %q -> %q",
			nonFixturePreference, after.Snapshot.Preference)
	}
	if after.Snapshot.Scope != "selected" {
		t.Errorf("reproducing rewrote the original's source scope: %q", after.Snapshot.Scope)
	}
}

// The preference is one field of the recorded inputs; none of them may move.
func TestReproduceLeavesEveryOriginalSnapshotInputUnchanged(t *testing.T) {
	s := reproduceService(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u"}
	original := originalRunWithPreference(t, s, ctx, scope)
	before := CloneSnapshot(original.Snapshot)

	if _, err := s.Run(ctx, scope, "", "normal", 11, original.ID); err != nil {
		t.Fatal(err)
	}

	after, err := s.Store.GetRun(ctx, scope, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Snapshot.Preference != before.Preference ||
		after.Snapshot.Scope != before.Scope ||
		after.Snapshot.ConfigVersion != before.ConfigVersion ||
		after.Snapshot.PersonaRef != before.PersonaRef ||
		after.Snapshot.Executor != before.Executor ||
		after.Snapshot.ExecutorVersion != before.ExecutorVersion ||
		after.Snapshot.Temperature != before.Temperature ||
		after.Snapshot.Budget != before.Budget ||
		after.Snapshot.Timeout != before.Timeout {
		t.Errorf("reproducing changed the original's recorded inputs:\nbefore %+v\nafter  %+v",
			before, after.Snapshot)
	}
	if len(after.Snapshot.Grants) != len(before.Grants) ||
		len(after.Snapshot.Required) != len(before.Required) ||
		len(after.Snapshot.Hashes) != len(before.Hashes) {
		t.Error("reproducing changed the original's recorded source lists")
	}
	// Status and verdict are part of the original's record too.
	if after.Status != original.Status || after.Regression != original.Regression {
		t.Errorf("reproducing changed the original's outcome: status %q->%q regression %q->%q",
			original.Status, after.Status, original.Regression, after.Regression)
	}
}

// The reproduction is a new run that links back; it does not become the original
// and does not inherit its preference by reference.
func TestReproduceProducesANewRunLinkedToTheOriginal(t *testing.T) {
	s := reproduceService(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u"}
	original := originalRunWithPreference(t, s, ctx, scope)

	repro, err := s.Run(ctx, scope, "", "timeout", 21, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repro.ID == original.ID {
		t.Fatal("reproducing reused the original run id")
	}
	if repro.Original != original.ID {
		t.Errorf("reproduction links to %q, want the original %q", repro.Original, original.ID)
	}
	// Documented limitation, asserted so a change to it is deliberate: the
	// reproduction carries the fixture snapshot rather than restoring the
	// original's inputs (simulator.go:14). See specs/008 Assumptions - closing
	// that is a separate product decision, not this feature's scope.
	if repro.Snapshot.Preference == nonFixturePreference {
		t.Error("reproduction unexpectedly restored the original's preference; " +
			"if that is now intended, specs/008 Assumptions and the acceptance " +
			"mapping follow-up entry must be updated together with this test")
	}
}
