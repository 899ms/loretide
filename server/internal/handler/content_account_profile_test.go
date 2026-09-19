package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Contract: specs/021-account-expression-profile/contracts/expression-profile.md

func setExpressionProfile(t *testing.T, wsID, accountID string, profile ipprofile.ExpressionProfile) map[string]any {
	t.Helper()
	payload, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	req := withURLParam(testutil.WithHeaders(
		testutil.JSONRequest(http.MethodPost, "/api/content-accounts/"+accountID+"/profile", string(payload)),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID), "id", accountID)
	var body map[string]any
	testutil.Call(t, testHandler.SetAccountExpressionProfile, req).
		Want(http.StatusCreated).JSON(&body)
	return body
}

func profileReadinessRequest(wsID, accountID string) *http.Request {
	return withURLParam(testutil.WithHeaders(
		testutil.JSONRequest(http.MethodGet, "/api/content-accounts/"+accountID+"/profile", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID), "id", accountID)
}

func confirmedProfile() ipprofile.ExpressionProfile {
	return ipprofile.ExpressionProfile{
		Audience:        ipprofile.TextField{Value: "designers", Status: ipprofile.FieldConfirmed},
		ContentPillars:  ipprofile.TextField{Value: "tools", Status: ipprofile.FieldConfirmed},
		PrimaryChannels: ipprofile.ListField{Values: []string{"zhihu"}, Status: ipprofile.FieldConfirmed},
		WeeklyHours:     ipprofile.HoursField{Value: 4, Status: ipprofile.FieldConfirmed},
	}
}

func TestProfileAndPersonaConfirmationsCarryTheOtherHalf(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "profile-carry", "owner")
	accountID := createAccount(t, ws, "zhihu", "Profile carry")["account_id"].(string)

	setPrompt(t, ws, accountID, "persona one")
	profileRevision := setExpressionProfile(t, ws, accountID, confirmedProfile())
	if profileRevision["persona_prompt"] != "persona one" {
		t.Fatalf("profile write carried persona %v", profileRevision["persona_prompt"])
	}

	promptRevision := setPrompt(t, ws, accountID, "persona two")
	profile, ok := promptRevision["profile"].(map[string]any)
	if !ok {
		t.Fatalf("prompt write has no profile: %v", promptRevision)
	}
	audience := profile["audience"].(map[string]any)
	if audience["value"] != "designers" || audience["status"] != "confirmed" {
		t.Fatalf("prompt write did not carry profile: %v", profile)
	}
	if profileRevision["revision"].(float64) != 2 || promptRevision["revision"].(float64) != 3 {
		t.Fatalf("confirmation revisions = %v and %v, want 2 and 3",
			profileRevision["revision"], promptRevision["revision"])
	}

	var old map[string]any
	testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, ws, accountID, profileRevision["revision_id"].(string))).
		Want(http.StatusOK).JSON(&old)
	if old["persona_prompt"] != "persona one" {
		t.Fatalf("old revision changed: %v", old)
	}
}

func TestProfileRejectsInvalidShapeWithoutWriting(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "profile-invalid", "owner")
	accountID := createAccount(t, ws, "zhihu", "Profile invalid")["account_id"].(string)

	var before int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account_revision WHERE account_id = $1`, accountID).Scan(&before)
	req := withURLParam(testutil.WithHeaders(
		testutil.JSONRequest(http.MethodPost, "/api/content-accounts/"+accountID+"/profile",
			`{"primary_channels":{"values":["myspace"],"status":"confirmed"}}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws), "id", accountID)
	testutil.Call(t, testHandler.SetAccountExpressionProfile, req).Want(http.StatusBadRequest)

	var after int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account_revision WHERE account_id = $1`, accountID).Scan(&after)
	if after != before {
		t.Fatalf("invalid input changed revision count from %d to %d", before, after)
	}
}

func TestProfileReadinessReturnsCurrentDerivedDecision(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "profile-readiness", "owner")
	accountID := createAccount(t, ws, "zhihu", "Profile readiness")["account_id"].(string)
	setExpressionProfile(t, ws, accountID, confirmedProfile())

	var body struct {
		Profile               ipprofile.ExpressionProfile `json:"profile"`
		Readiness             ipprofile.Readiness         `json:"readiness"`
		UsesNeutralExpression bool                        `json:"uses_neutral_expression"`
	}
	testutil.Call(t, testHandler.GetAccountExpressionProfile,
		profileReadinessRequest(ws, accountID)).Want(http.StatusOK).JSON(&body)
	if !body.Readiness.CanStart || len(body.Readiness.Missing) != 0 {
		t.Fatalf("readiness = %+v, want startable", body.Readiness)
	}
	if !body.UsesNeutralExpression {
		t.Fatal("profile without a confirmed sample was not marked neutral")
	}
	if body.Profile.Audience.Value != "designers" {
		t.Fatalf("read profile audience = %+v", body.Profile.Audience)
	}
}

func TestOldRevisionWithoutProfileReadsAsEmptyPendingProfile(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "profile-old-revision", "owner")
	accountID := createAccount(t, ws, "zhihu", "Old revision")["account_id"].(string)
	revisionID := "pre-482-profile-revision"
	_, err := testPool.Exec(t.Context(), `
		INSERT INTO content_account_revision
			(revision_id, account_id, workspace_id, revision, persona_prompt)
		VALUES ($1, $2, $3, 1, 'legacy persona')`, revisionID, accountID, ws)
	if err != nil {
		t.Fatalf("insert legacy-shaped revision: %v", err)
	}

	var body map[string]any
	testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, ws, accountID, revisionID)).Want(http.StatusOK).JSON(&body)
	profile, ok := body["profile"].(map[string]any)
	if !ok {
		t.Fatalf("legacy revision profile = %T %v", body["profile"], body["profile"])
	}
	audience := profile["audience"].(map[string]any)
	if audience["value"] != "" || audience["status"] != "pending" {
		t.Fatalf("legacy audience = %v, want empty pending", audience)
	}
}

func TestAnotherBrandsProfileIsIndistinguishableFromAMissingAccount(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	mine := accountWorkspace(t, "profile-mine", "owner")
	theirs := accountWorkspace(t, "profile-theirs", "")
	accountID := createAccount(t, mine, "zhihu", "Mine")["account_id"].(string)
	setExpressionProfile(t, mine, accountID, confirmedProfile())

	hidden := testutil.Call(t, testHandler.GetAccountExpressionProfile,
		profileReadinessRequest(theirs, accountID)).Want(http.StatusNotFound).Body.String()
	missing := testutil.Call(t, testHandler.GetAccountExpressionProfile,
		profileReadinessRequest(theirs, "no-such-account")).Want(http.StatusNotFound).Body.String()
	if hidden != missing {
		t.Fatalf("cross-brand and missing profile responses differ:\n%s\n%s", hidden, missing)
	}
}
