//go:build dbtest

package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func inboxRequest(method, path, workspaceID string) *http.Request {
	return testutil.WithHeaders(
		testutil.JSONRequest(method, path, nil),
		"X-User-ID", testUserID,
		"X-Workspace-ID", workspaceID,
	)
}

func inboxWorkspaceHandler(handler http.HandlerFunc) http.HandlerFunc {
	return middleware.RequireWorkspaceMember(testHandler.Queries)(handler).ServeHTTP
}

func TestListInboxProjectsCurrentIssueStatusAndPriority(t *testing.T) {
	workspaceID := dbfx.Workspace(t, "Inbox filter projections", "inbox-filter-"+uuid.NewString())
	dbfx.Member(t, workspaceID, testUserID, "owner")
	issueID := dbfx.Issue(t, "Filtered issue", testutil.Cols{
		"workspace_id": workspaceID,
		"status":       "in_review",
		"priority":     "high",
	})
	dbfx.Insert(t, "inbox_item", testutil.Cols{
		"workspace_id":   workspaceID,
		"recipient_type": "member",
		"recipient_id":   testUserID,
		"type":           "status_changed",
		"severity":       "info",
		"issue_id":       issueID,
		"title":          "Projected issue",
	})

	var items []InboxItemResponse
	testutil.Call(t, inboxWorkspaceHandler(testHandler.ListInbox),
		inboxRequest(http.MethodGet, "/api/inbox", workspaceID)).
		Want(http.StatusOK).
		JSON(&items)

	if len(items) != 1 {
		t.Fatalf("inbox items = %d, want 1: %+v", len(items), items)
	}
	if items[0].IssueStatus == nil || *items[0].IssueStatus != "in_review" {
		t.Errorf("issue_status = %v, want in_review", items[0].IssueStatus)
	}
	if items[0].IssuePriority == nil || *items[0].IssuePriority != "high" {
		t.Errorf("issue_priority = %v, want high", items[0].IssuePriority)
	}
}

func TestListArchivedInboxLimitsIssueGroupsNotRows(t *testing.T) {
	workspaceID := dbfx.Workspace(t, "Archived inbox groups", "archived-groups-"+uuid.NewString())
	dbfx.Member(t, workspaceID, testUserID, "owner")
	noisyIssueID := dbfx.Issue(t, "Noisy archived issue", testutil.Cols{"workspace_id": workspaceID})
	olderIssueID := dbfx.Issue(t, "Older archived issue", testutil.Cols{"workspace_id": workspaceID})

	base := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < 200; i++ {
		cols := testutil.Cols{
			"workspace_id":   workspaceID,
			"recipient_type": "member",
			"recipient_id":   testUserID,
			"type":           "status_changed",
			"severity":       "info",
			"issue_id":       noisyIssueID,
			"title":          fmt.Sprintf("noisy-%03d", i),
			"archived":       true,
			"created_at":     base.Add(-time.Duration(i) * time.Millisecond),
		}
		if i == 199 {
			// The bounded response keeps this row as the group's comment anchor,
			// even though the newest status row is the one the UI renders.
			cols["details"] = testutil.Raw(`'{"comment_id":"comment-1"}'::jsonb`)
		}
		dbfx.Insert(t, "inbox_item", cols)
	}
	dbfx.Insert(t, "inbox_item", testutil.Cols{
		"workspace_id":   workspaceID,
		"recipient_type": "member",
		"recipient_id":   testUserID,
		"type":           "new_comment",
		"severity":       "info",
		"issue_id":       olderIssueID,
		"title":          "older-group",
		"archived":       true,
		"created_at":     base.Add(-time.Hour),
	})

	var items []InboxItemResponse
	testutil.Call(t, inboxWorkspaceHandler(testHandler.ListArchivedInbox),
		inboxRequest(http.MethodGet, "/api/inbox/archived", workspaceID)).
		Want(http.StatusOK).
		JSON(&items)

	var noisyRows int
	var sawNoisyNewest, sawCommentAnchor, sawOlderGroup bool
	for _, item := range items {
		switch {
		case item.IssueID != nil && *item.IssueID == noisyIssueID:
			noisyRows++
			sawNoisyNewest = sawNoisyNewest || item.Title == "noisy-000"
			sawCommentAnchor = sawCommentAnchor || strings.Contains(string(item.Details), `"comment_id":"comment-1"`)
		case item.IssueID != nil && *item.IssueID == olderIssueID:
			sawOlderGroup = true
		}
	}
	if noisyRows != 2 || !sawNoisyNewest || !sawCommentAnchor {
		t.Fatalf("noisy group rows = %d, newest=%v anchor=%v; items=%+v",
			noisyRows, sawNoisyNewest, sawCommentAnchor, items)
	}
	if !sawOlderGroup {
		t.Fatal("raw-row limit let one issue hide another archived issue group")
	}
}

func TestArchiveAllReadInboxUsesNewestIssueRow(t *testing.T) {
	workspaceID := dbfx.Workspace(t, "Archive read groups", "archive-read-"+uuid.NewString())
	dbfx.Member(t, workspaceID, testUserID, "owner")
	readIssueID := dbfx.Issue(t, "Newest row is read", testutil.Cols{"workspace_id": workspaceID})
	unreadIssueID := dbfx.Issue(t, "Newest row is unread", testutil.Cols{"workspace_id": workspaceID})

	insert := func(issueID, title string, read bool, createdAt testutil.Raw) {
		t.Helper()
		dbfx.Insert(t, "inbox_item", testutil.Cols{
			"workspace_id":   workspaceID,
			"recipient_type": "member",
			"recipient_id":   testUserID,
			"type":           "status_changed",
			"severity":       "info",
			"issue_id":       issueID,
			"title":          title,
			"read":           read,
			"archived":       false,
			"created_at":     createdAt,
		})
	}
	insert(readIssueID, "older unread", false, "now() - interval '2 minutes'")
	insert(readIssueID, "newest read", true, "now() - interval '1 minute'")
	insert(unreadIssueID, "older read", true, "now() - interval '2 minutes'")
	insert(unreadIssueID, "newest unread", false, "now() - interval '1 minute'")

	testutil.Call(t, inboxWorkspaceHandler(testHandler.ArchiveAllReadInbox),
		inboxRequest(http.MethodPost, "/api/inbox/archive-all-read", workspaceID)).
		Want(http.StatusOK)

	if got := dbfx.Count(t,
		"SELECT count(*) FROM inbox_item WHERE issue_id = $1 AND archived = true", readIssueID); got != 2 {
		t.Fatalf("archived rows in read issue = %d, want the whole two-row group", got)
	}
	if got := dbfx.Count(t,
		"SELECT count(*) FROM inbox_item WHERE issue_id = $1 AND archived = true", unreadIssueID); got != 0 {
		t.Fatalf("archived rows in unread issue = %d, want the whole group untouched", got)
	}
}

func TestArchiveCompletedInboxExpandsCustomTerminalStatuses(t *testing.T) {
	workspaceID := dbfx.Workspace(t, "Archive custom completed", "archive-custom-completed-"+uuid.NewString())
	dbfx.Member(t, workspaceID, testUserID, "owner")
	dbfx.Insert(t, "issue_status", testutil.Cols{
		"workspace_id": workspaceID,
		"key":          "verified_complete",
		"name":         "Verified complete",
		"category":     "done",
		"color":        "#22c55e",
		"is_system":    false,
		"position":     1,
	})
	completedIssueID := dbfx.Issue(t, "Custom completed issue", testutil.Cols{
		"workspace_id": workspaceID,
		"status":       "verified_complete",
	})
	openIssueID := dbfx.Issue(t, "Open issue", testutil.Cols{
		"workspace_id": workspaceID,
		"status":       "todo",
	})
	for _, issueID := range []string{completedIssueID, openIssueID} {
		dbfx.Insert(t, "inbox_item", testutil.Cols{
			"workspace_id":   workspaceID,
			"recipient_type": "member",
			"recipient_id":   testUserID,
			"type":           "status_changed",
			"severity":       "info",
			"issue_id":       issueID,
			"title":          "Status changed",
			"archived":       false,
		})
	}

	testutil.Call(t, inboxWorkspaceHandler(testHandler.ArchiveCompletedInbox),
		inboxRequest(http.MethodPost, "/api/inbox/archive-completed", workspaceID)).
		Want(http.StatusOK)

	if got := dbfx.Count(t,
		"SELECT count(*) FROM inbox_item WHERE issue_id = $1 AND archived = true", completedIssueID); got != 1 {
		t.Fatalf("archived rows for custom completed issue = %d, want 1", got)
	}
	if got := dbfx.Count(t,
		"SELECT count(*) FROM inbox_item WHERE issue_id = $1 AND archived = true", openIssueID); got != 0 {
		t.Fatalf("archived rows for open issue = %d, want 0", got)
	}
}

// Both inbox lists ship the preview, the stored row keeps the whole comment,
// and a notification that needs its full body still gets it.
func TestInboxListsShipCommentPreviewNotFullComment(t *testing.T) {
	workspaceID := dbfx.Workspace(t, "Inbox body preview", "inbox-preview-"+uuid.NewString())
	dbfx.Member(t, workspaceID, testUserID, "owner")
	activeIssue := dbfx.Issue(t, "Active comment issue", testutil.Cols{"workspace_id": workspaceID})
	archivedIssue := dbfx.Issue(t, "Archived comment issue", testutil.Cols{"workspace_id": workspaceID})
	fullComment := strings.Repeat("A long agent reply. ", 300)
	issueLessBody := strings.Repeat("An issue-less notice. ", 300)

	insert := func(cols testutil.Cols) {
		base := testutil.Cols{
			"workspace_id":   workspaceID,
			"recipient_type": "member",
			"recipient_id":   testUserID,
			"severity":       "info",
			"title":          "Notification",
		}
		for k, v := range cols {
			base[k] = v
		}
		dbfx.Insert(t, "inbox_item", base)
	}
	insert(testutil.Cols{"type": "new_comment", "issue_id": activeIssue, "body": fullComment})
	insert(testutil.Cols{"type": "new_comment", "issue_id": archivedIssue, "body": fullComment, "archived": true})
	insert(testutil.Cols{"type": "autopilot_paused", "body": issueLessBody})

	wantPreview := string([]rune(fullComment)[:inboxListBodyPreviewLimit-1]) + "…"
	bodyOf := func(items []InboxItemResponse, issueID string) string {
		t.Helper()
		for _, item := range items {
			if item.IssueID != nil && *item.IssueID == issueID {
				if item.Body == nil {
					t.Fatalf("item for issue %s has no body", issueID)
				}
				return *item.Body
			}
		}
		t.Fatalf("no item for issue %s in %d items", issueID, len(items))
		return ""
	}

	var active []InboxItemResponse
	testutil.Call(t, inboxWorkspaceHandler(testHandler.ListInbox),
		inboxRequest(http.MethodGet, "/api/inbox", workspaceID)).
		Want(http.StatusOK).
		JSON(&active)
	if got := bodyOf(active, activeIssue); got != wantPreview {
		t.Errorf("main list body = %d characters, want the %d-character preview",
			utf8.RuneCountInString(got), inboxListBodyPreviewLimit)
	}
	var issueLess *string
	for _, item := range active {
		if item.IssueID == nil {
			issueLess = item.Body
		}
	}
	if issueLess == nil || *issueLess != issueLessBody {
		t.Errorf("issue-less notification lost its full body (its detail pane renders it)")
	}

	var archived []InboxItemResponse
	testutil.Call(t, inboxWorkspaceHandler(testHandler.ListArchivedInbox),
		inboxRequest(http.MethodGet, "/api/inbox/archived", workspaceID)).
		Want(http.StatusOK).
		JSON(&archived)
	if got := bodyOf(archived, archivedIssue); got != wantPreview {
		t.Errorf("archived list body = %d characters, want the %d-character preview",
			utf8.RuneCountInString(got), inboxListBodyPreviewLimit)
	}

	// Only the response is shortened; the stored comment is whole.
	if got := dbfx.Count(t,
		`SELECT count(*) FROM inbox_item WHERE workspace_id = $1 AND type = 'new_comment' AND body = $2`,
		workspaceID, fullComment); got != 2 {
		t.Errorf("stored full comments = %d, want 2", got)
	}
}
