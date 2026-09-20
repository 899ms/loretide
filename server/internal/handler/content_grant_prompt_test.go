//go:build dbtest

package handler

import (
	"net/http"
	"testing"
	"time"

	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// A persona prompt is configuration, not a credential (D11-V01).
//
// This case lives in the handler package rather than beside CanRead because
// workspace-core's dependencies are diagnostics only — it cannot import
// ip-profile, and a test written there would have to invent an account instead
// of using one. Here both are reachable, so the prompts below are written
// through the real LT-012 endpoint onto real accounts, and the refusal is a
// refusal of the actual thing rather than of a stand-in.
//
// What it catches: any version of CanRead that reads, compares, or derives a
// key from expression settings. Today it cannot — ReadRequest carries no prompt
// and workspace-core cannot reach one — and that is the point: this case is how
// a future change that plumbs one in gets noticed.
func TestTwoAccountsWithIdenticalPersonaPromptsStillCannotReadEachOther(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "grant-identical-prompts", "owner")
	a := createAccount(t, ws, "zhihu", "A 号")["account_id"].(string)
	b := createAccount(t, ws, "weibo", "B 号")["account_id"].(string)

	// Byte-for-byte identical, written through the real endpoint.
	const shared = "同一段人设提示词，逐字节相同。"
	setPrompt(t, ws, a, shared)
	setPrompt(t, ws, b, shared)

	if promptOf(t, ws, a) != promptOf(t, ws, b) {
		t.Fatal("the two prompts differ, so this case would prove nothing")
	}

	// A task acting for account A, holding a grant for account A's material,
	// reaching for account B's material.
	task := workspacecore.Principal{Kind: workspacecore.PrincipalTask, ID: "task-for-a"}
	decision := workspacecore.CanRead(t.Context(), nil, workspacecore.ReadRequest{
		Principal: task,
		Workspace: ws,
		Resource: workspacecore.ResourceRef{
			Workspace: ws, Account: b, Kind: workspacecore.KindMaterial, ID: "b-material",
		},
		Now: time.Now().UTC(),
		Grants: []workspacecore.Grant{{
			Principal: task, Workspace: ws, Account: a,
			Kinds: []workspacecore.ResourceKind{workspacecore.KindMaterial},
		}},
	})

	if decision.Allowed {
		t.Error("identical persona prompts widened access between two accounts; " +
			"a prompt is expression configuration, not a credential")
	}
	if decision.Reason != workspacecore.GrantReasonAccount {
		t.Errorf("reason = %q, want %q", decision.Reason, workspacecore.GrantReasonAccount)
	}
}

// The control half: the same task reading its OWN account's material is
// allowed. Without it, the case above would also pass if CanRead simply refused
// everything.
func TestTheSameGrantStillWorksForItsOwnAccount(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "grant-own-account", "owner")
	a := createAccount(t, ws, "zhihu", "A 号")["account_id"].(string)
	setPrompt(t, ws, a, "A 的人设")

	task := workspacecore.Principal{Kind: workspacecore.PrincipalTask, ID: "task-for-a"}
	decision := workspacecore.CanRead(t.Context(), nil, workspacecore.ReadRequest{
		Principal: task,
		Workspace: ws,
		Resource: workspacecore.ResourceRef{
			Workspace: ws, Account: a, Kind: workspacecore.KindMaterial, ID: "a-material",
		},
		Now: time.Now().UTC(),
		Grants: []workspacecore.Grant{{
			Principal: task, Workspace: ws, Account: a,
			Kinds: []workspacecore.ResourceKind{workspacecore.KindMaterial},
		}},
	})

	if !decision.Allowed {
		t.Errorf("the task cannot read its own account's material: %q", decision.Reason)
	}
}

// promptOf reads an account's current persona prompt through the API.
func promptOf(t *testing.T, wsID, accountID string) string {
	t.Helper()
	var body map[string]any
	req := withURLParam(testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-accounts/"+accountID+"/persona", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID), "id", accountID)
	testutil.Call(t, testHandler.GetAccountPersonaPrompt, req).Want(http.StatusOK).JSON(&body)
	prompt, _ := body["persona_prompt"].(string)
	return prompt
}
