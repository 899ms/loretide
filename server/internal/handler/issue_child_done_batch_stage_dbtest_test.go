//go:build dbtest

package handler

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestBatchChildDonePreservesRepresentativeAndParentOrder(t *testing.T) {
	for _, staged := range []bool{false, true} {
		for _, status := range []string{"done", "cancelled", "approved"} {
			t.Run(fmt.Sprintf("staged=%t/%s", staged, status), func(t *testing.T) {
				ws := dbfx.Workspace(t, "Batch stage selection", "batch-stage-selection", testutil.Cols{"issue_prefix": "BST"})
				fx := testutil.New(testPool, ws, testUserID)
				fx.Member(t, ws, testUserID, "owner")
				fx.Insert(t, "issue_status", testutil.Cols{"workspace_id": ws, "key": "approved", "name": "Approved", "category": "done", "color": "#123456"})
				var parents, agents []string
				var children [][]string
				for p := range 2 {
					agent := fx.Agent(t, fmt.Sprintf("Parent agent %d", p), fx.Runtime(t, fmt.Sprintf("Runtime %d", p)))
					parent := fx.Issue(t, "Parent", testutil.Cols{"status": "in_progress", "assignee_type": "agent", "assignee_id": agent})
					parents, agents = append(parents, parent), append(agents, agent)
					fx.Cleanup(t, "DELETE FROM comment WHERE issue_id = $1", parent)
					fx.Cleanup(t, "DELETE FROM agent_task_queue WHERE issue_id = $1", parent)
					var ids []string
					for _, stage := range []int{2, 7, 7} {
						cols := testutil.Cols{"status": "in_progress", "parent_issue_id": parent}
						if staged {
							cols["stage"] = stage
						}
						ids = append(ids, fx.Issue(t, "Child", cols))
					}
					children = append(children, ids)
					if staged {
						fx.Issue(t, "Next stage", testutil.Cols{"status": "backlog", "parent_issue_id": parent, "stage": 20})
						fx.Issue(t, "Unstaged unknown status", testutil.Cols{"status": "missing", "parent_issue_id": parent})
					}
				}
				h := *testHandler
				h.Bus = events.New()
				var notified []string
				h.Bus.Subscribe(protocol.EventCommentCreated, func(e events.Event) {
					payload, ok := e.Payload.(map[string]any)
					if !ok {
						t.Errorf("unexpected comment payload %T", e.Payload)
						return
					}
					comment, ok := payload["comment"].(CommentResponse)
					if !ok {
						t.Errorf("unexpected comment response %T", payload["comment"])
						return
					}
					notified = append(notified, comment.IssueID)
				})
				// Interleave parents, visit higher stages before lower ones, and
				// choose the second child in the highest stage first.
				ids := []string{children[1][2], children[0][0], children[1][0], children[0][2], children[1][1], children[0][1]}
				request := testutil.WithHeaders(testutil.JSONRequest(http.MethodPatch, "/api/issues/batch", map[string]any{
					"issue_ids": ids, "updates": map[string]any{"status": status},
				}), "X-User-ID", testUserID, "X-Workspace-ID", ws)
				var response struct {
					Updated int `json:"updated"`
				}
				testutil.Call(t, h.BatchUpdateIssues, request).Want(http.StatusOK).JSON(&response)
				if response.Updated != len(ids) {
					t.Fatalf("updated=%d, want %d", response.Updated, len(ids))
				}
				if !slices.Equal(notified, []string{parents[1], parents[0]}) {
					t.Fatalf("notification order=%v, want second parent then first", notified)
				}
				for p, parent := range parents {
					if countSystemCommentsOn(t, parent) != 1 || countPendingTasksForAgent(t, parent, agents[p]) != 1 {
						t.Fatal("each parent must receive exactly one comment and one pending run")
					}
					rep := children[p][2]
					if !staged && p == 0 {
						rep = children[p][0] // Unstaged groups retain their first completed child.
					}
					content, _, _, _ := systemCommentOn(t, parent)
					if !strings.Contains(content, "together in a batch update") || !strings.Contains(content, "](mention://issue/"+rep+")") {
						t.Fatalf("lost batch wording or first representative: %s", content)
					}
					if staged && (!strings.Contains(content, "Stage 7 of this issue is complete") || !strings.Contains(content, "Stage 2: 1/1 done; Stage 7: 2/2 done; Stage 20: 0/1 done (next)") || !strings.Contains(content, "Stage 20 is next")) {
						t.Fatalf("inaccurate final-state summary: %s", content)
					}
					if !staged && !strings.Contains(content, "All sub-issues are complete") {
						t.Fatalf("lost unstaged completion: %s", content)
					}
					if got, want := triggerCommentIDForAgentTask(t, parent, agents[p]), systemCommentIDOn(t, parent); got != want {
						t.Fatalf("run trigger=%s, want final comment %s", got, want)
					}
				}
			})
		}
	}
}
