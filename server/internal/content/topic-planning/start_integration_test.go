package topicplanning

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
)

// Starting a brief revision, against real PostgreSQL.
//
// Contract: specs/023-ep04b-start-snapshot/contracts/start-snapshot.md
//
// Runs only with LORETIDE_TOPIC_TEST_DATABASE_URL set; newTopicFixture skips
// otherwise, and a skip is not a pass.

const (
	startWorkspace = "ws-start"
	startAccount   = "acct-start"
)

func readyProfile() ipprofile.ExpressionProfile {
	profile := ipprofile.ExpressionProfile{}
	profile.Audience = ipprofile.TextField{Value: "new managers", Status: ipprofile.FieldConfirmed}
	profile.ContentPillars = ipprofile.TextField{Value: "one-on-ones", Status: ipprofile.FieldConfirmed}
	profile.PrimaryChannels = ipprofile.ListField{Values: []string{"xiaohongshu"}, Status: ipprofile.FieldConfirmed}
	profile.WeeklyHours = ipprofile.HoursField{Value: 6, Status: ipprofile.FieldConfirmed}
	return profile
}

// seedAccount writes an account with a stored scope preference and one persona
// revision carrying the given profile, and returns the revision id.
func seedAccount(t *testing.T, fx topicFixture, savedScope string, profile ipprofile.ExpressionProfile) string {
	t.Helper()
	settings, err := json.Marshal(map[string]any{ipprofile.ScopeSettingsKey: savedScope})
	if err != nil {
		t.Fatal(err)
	}
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	fx.db.Exec(t, `INSERT INTO content_account
		(account_id, workspace_id, platform, display_name, settings)
		VALUES ($1,$2,'xiaohongshu','start account',$3)
		ON CONFLICT DO NOTHING`, startAccount, startWorkspace, settings)
	next := fx.db.Count(t, `SELECT COALESCE(max(revision),0)+1 FROM content_account_revision
		WHERE workspace_id=$1 AND account_id=$2`, startWorkspace, startAccount)
	revisionID := "persona-" + diagnostics.NewID()
	fx.db.Exec(t, `INSERT INTO content_account_revision
		(revision_id, account_id, workspace_id, revision, persona_prompt, profile)
		VALUES ($1,$2,$3,$4,'',$5)`,
		revisionID, startAccount, startWorkspace, next, profileJSON)
	return revisionID
}

// seedStartableBrief creates a card, starts it (EP-04a) and returns the card id
// and the frozen brief revision id.
func seedStartableBrief(t *testing.T, fx topicFixture) (string, string) {
	t.Helper()
	ctx := t.Context()
	card, err := fx.store.Create(ctx, "actor-start", completeCard(startWorkspace))
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	result, err := fx.store.Act(ctx, startWorkspace, "actor-start", card.TopicCardID,
		ActionRequest{Action: ActionStart, Brief: completeBrief()})
	if err != nil {
		t.Fatalf("freeze brief: %v", err)
	}
	if result.Brief == nil {
		t.Fatal("the start action produced no brief revision")
	}
	return card.TopicCardID, result.Brief.BriefRevisionID
}

func startRequest(scope string) StartRequest {
	return StartRequest{AccountID: startAccount, SourceScope: scope, AutoPrecheck: true}
}

func TestAStartRecordsTheConfigurationOfTheMoment(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	revisionID := seedAccount(t, fx, string(ipprofile.ScopeLocal), readyProfile())
	cardID, briefID := seedStartableBrief(t, fx)

	// The scope chosen for this start differs from the stored preference, so a
	// single collapsed field cannot pass.
	snapshot, err := fx.store.Start(ctx, startWorkspace, "actor-start", cardID, briefID,
		startRequest(string(ipprofile.ScopeWeb)))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if snapshot.SnapshotID == "" {
		t.Error("the snapshot has no stable key for a future run to pin")
	}
	if snapshot.Snapshot.Scope != string(ipprofile.ScopeWeb) {
		t.Errorf("source_scope = %q, want web", snapshot.Snapshot.Scope)
	}
	if snapshot.Snapshot.Preference != string(ipprofile.ScopeLocal) {
		t.Errorf("saved_preference = %q, want local", snapshot.Snapshot.Preference)
	}
	if snapshot.Snapshot.PersonaRef != revisionID {
		t.Errorf("persona_ref = %q, want %q", snapshot.Snapshot.PersonaRef, revisionID)
	}
	if !snapshot.Snapshot.AutoPrecheck {
		t.Error("auto_precheck was not recorded")
	}
	// No confirmed style sample in readyProfile, so the account writes neutral.
	if !snapshot.Snapshot.UsesNeutralExpression {
		t.Error("uses_neutral_expression was not recorded")
	}
	if snapshot.ProjectID != "" {
		t.Errorf("project_id = %q, want empty for a start with no project", snapshot.ProjectID)
	}
}

func TestAStartIsRefusedUntilTheAccountIsReadyAndSaysWhatIsMissing(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	profile := readyProfile()
	// Typed but not confirmed: SOP 3.1's "pending" does not satisfy the
	// condition, which is exactly the case a value-only check would pass.
	profile.ContentPillars = ipprofile.TextField{Value: "one-on-ones", Status: ipprofile.FieldPending}
	profile.WeeklyHours = ipprofile.HoursField{Value: 0, Status: ipprofile.FieldConfirmed}
	seedAccount(t, fx, string(ipprofile.ScopeAll), profile)
	cardID, briefID := seedStartableBrief(t, fx)

	_, err := fx.store.Start(ctx, startWorkspace, "actor-start", cardID, briefID,
		startRequest(string(ipprofile.ScopeAll)))
	var readiness ReadinessError
	if !errorsAs(err, &readiness) {
		t.Fatalf("start with an unready account = %v, want a readiness error", err)
	}
	want := []string{ipprofile.MissingPillars, ipprofile.MissingWeeklyHours}
	if !reflect.DeepEqual(readiness.Missing, want) {
		t.Errorf("missing = %v, want %v", readiness.Missing, want)
	}
	if fx.db.Count(t, `SELECT count(*) FROM content_start_snapshot WHERE workspace_id=$1`, startWorkspace) != 0 {
		t.Error("a refused start still wrote a snapshot")
	}
}

func TestOneBriefRevisionCanBeStartedManyTimes(t *testing.T) {
	// The behaviour the new table buys (spec FR-014a). It is the one most
	// easily lost to somebody "just adding an idempotency guard", and nothing
	// else in the suite would notice.
	fx := newTopicFixture(t)
	ctx := t.Context()
	seedAccount(t, fx, string(ipprofile.ScopeAll), readyProfile())
	cardID, briefID := seedStartableBrief(t, fx)

	first, err := fx.store.Start(ctx, startWorkspace, "actor-start", cardID, briefID,
		startRequest(string(ipprofile.ScopeLocal)))
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	second, err := fx.store.Start(ctx, startWorkspace, "actor-start", cardID, briefID,
		startRequest(string(ipprofile.ScopeWeb)))
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if first.SnapshotID == second.SnapshotID {
		t.Fatal("the second start reused the first snapshot id")
	}

	reread, err := fx.store.GetSnapshot(ctx, startWorkspace, "actor-start", cardID, first.SnapshotID)
	if err != nil {
		t.Fatalf("re-read first snapshot: %v", err)
	}
	if reread.Snapshot.Scope != string(ipprofile.ScopeLocal) {
		t.Errorf("the first snapshot now reads source_scope=%q; the second start rewrote it",
			reread.Snapshot.Scope)
	}

	listed, err := fx.store.ListSnapshots(ctx, startWorkspace, "actor-start", cardID, briefID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d snapshots, want 2", len(listed))
	}
}

// Four sources, four cases. "Changing the configuration does not change the
// snapshot" sounds like one assertion; it is four different write paths, and
// testing one leaves three that can break unnoticed.
func TestNothingChangedAfterwardsReachesAnExistingSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, fx topicFixture, cardID string)
	}{
		{"the account's expression profile changes", func(t *testing.T, fx topicFixture, _ string) {
			seedAccount(t, fx, string(ipprofile.ScopeAll), readyProfile())
		}},
		{"the account's scope preference changes", func(t *testing.T, fx topicFixture, _ string) {
			settings, err := json.Marshal(map[string]any{ipprofile.ScopeSettingsKey: string(ipprofile.ScopeWeb)})
			if err != nil {
				t.Fatal(err)
			}
			fx.db.Exec(t, `UPDATE content_account SET settings=$3
				WHERE workspace_id=$1 AND account_id=$2`, startWorkspace, startAccount, settings)
		}},
		{"another brief revision is appended", func(t *testing.T, fx topicFixture, cardID string) {
			if _, err := fx.store.AppendBrief(t.Context(), startWorkspace, "actor-start", cardID,
				completeBrief()); err != nil {
				t.Fatal(err)
			}
		}},
		{"the brand precheck switch changes", func(t *testing.T, fx topicFixture, _ string) {
			// The switch lives on the workspace row, which this module never
			// reads: the handler passes its value in. Changing it therefore
			// cannot reach a stored snapshot by construction, and this case
			// pins that a later refactor does not start reading it live.
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newTopicFixture(t)
			ctx := t.Context()
			seedAccount(t, fx, string(ipprofile.ScopeLocal), readyProfile())
			cardID, briefID := seedStartableBrief(t, fx)
			before, err := fx.store.Start(ctx, startWorkspace, "actor-start", cardID, briefID,
				startRequest(string(ipprofile.ScopeWeb)))
			if err != nil {
				t.Fatalf("start: %v", err)
			}

			tc.change(t, fx, cardID)

			after, err := fx.store.GetSnapshot(ctx, startWorkspace, "actor-start", cardID, before.SnapshotID)
			if err != nil {
				t.Fatalf("re-read: %v", err)
			}
			if !reflect.DeepEqual(before.Snapshot, after.Snapshot) {
				t.Errorf("the snapshot changed:\n before %+v\n after  %+v", before.Snapshot, after.Snapshot)
			}
		})
	}
}

func TestASnapshotFromAnotherWorkspaceIsRefusedAsMissing(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	seedAccount(t, fx, string(ipprofile.ScopeAll), readyProfile())
	cardID, briefID := seedStartableBrief(t, fx)
	snapshot, err := fx.store.Start(ctx, startWorkspace, "actor-start", cardID, briefID,
		startRequest(string(ipprofile.ScopeAll)))
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err = fx.store.GetSnapshot(ctx, "ws-other", "actor-start", cardID, snapshot.SnapshotID); err == nil {
		t.Fatal("a snapshot id read from another workspace succeeded")
	}
	if _, err = fx.store.GetSnapshot(ctx, startWorkspace, "actor-start", cardID, "no-such-snapshot"); err == nil {
		t.Fatal("an unknown snapshot id succeeded")
	}
}

func TestABriefRevisionFromAnotherCardCannotBeStarted(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	seedAccount(t, fx, string(ipprofile.ScopeAll), readyProfile())
	firstCard, firstBrief := seedStartableBrief(t, fx)
	secondCard, _ := seedStartableBrief(t, fx)

	if _, err := fx.store.Start(ctx, startWorkspace, "actor-start", secondCard, firstBrief,
		startRequest(string(ipprofile.ScopeAll))); err == nil {
		t.Fatal("a brief revision belonging to another card was accepted")
	}
	if fx.db.Count(t, `SELECT count(*) FROM content_start_snapshot WHERE topic_card_id=$1`, secondCard) != 0 {
		t.Error("the refused start still wrote a snapshot")
	}
	_ = firstCard
}

// The same shape as TestBriefStoreHasNoUpdateOrDeletePath (022's A6). Snapshots
// are append-only, and the only way to keep that true is for no second write
// path to exist.
func TestStartSnapshotStoreHasNoUpdateOrDeletePath(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	dir := filepath.Dir(current)
	var combined strings.Builder
	for _, name := range []string{"start.go", "store.go"} {
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		combined.Write(source)
	}
	upper := strings.ToUpper(combined.String())
	for _, forbidden := range []string{
		"UPDATE CONTENT_START_SNAPSHOT", "DELETE FROM CONTENT_START_SNAPSHOT",
	} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("append-only store contains forbidden path %q", forbidden)
		}
	}
	if !strings.Contains(upper, "INSERT INTO CONTENT_START_SNAPSHOT") {
		t.Fatal("append-only store has no insert path")
	}
}

func errorsAs(err error, target *ReadinessError) bool {
	for err != nil {
		if readiness, ok := err.(ReadinessError); ok {
			*target = readiness
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}
