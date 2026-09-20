//go:build dbtest

package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestAgentConversationStartersRoundTrip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}

	var created AgentResponse
	testutil.Call(t, testHandler.CreateAgent, newRequest(http.MethodPost, "/api/agents", map[string]any{
		"name":       fmt.Sprintf("conversation-starters-%d", time.Now().UnixNano()),
		"runtime_id": handlerTestRuntimeID(t),
		"conversation_starters": []map[string]string{{
			"label":  "  Review a PR  ",
			"prompt": "  Review the most relevant open pull request.  ",
		}},
	})).Want(http.StatusCreated).JSON(&created)
	dbfx.Cleanup(t, `DELETE FROM agent WHERE id = $1`, created.ID)
	if len(created.ConversationStarters) != 1 ||
		created.ConversationStarters[0].Label != "Review a PR" ||
		created.ConversationStarters[0].Prompt != "Review the most relevant open pull request." {
		t.Fatalf("created conversation_starters = %#v", created.ConversationStarters)
	}

	var preserved AgentResponse
	testutil.Call(t, testHandler.UpdateAgent, withURLParam(
		newRequest(http.MethodPut, "/api/agents/"+created.ID, map[string]any{
			"description": "conversation starters unchanged",
		}),
		"id",
		created.ID,
	)).Want(http.StatusOK).JSON(&preserved)
	if len(preserved.ConversationStarters) != 1 {
		t.Fatalf("omitted update conversation_starters = %#v, want preserved prompt", preserved.ConversationStarters)
	}

	var clearedAgent AgentResponse
	testutil.Call(t, testHandler.UpdateAgent, withURLParam(
		newRequest(http.MethodPut, "/api/agents/"+created.ID, map[string]any{
			"conversation_starters": []AgentConversationStarter{},
		}),
		"id",
		created.ID,
	)).Want(http.StatusOK).JSON(&clearedAgent)
	if len(clearedAgent.ConversationStarters) != 0 {
		t.Fatalf("cleared conversation_starters = %#v, want empty", clearedAgent.ConversationStarters)
	}
}
