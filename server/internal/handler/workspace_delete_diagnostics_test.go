package handler

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestDeleteWorkspace_PurgesContentDiagnosticsAtomically(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	createWorkspace := func(slug string) string {
		var id string
		if err := testPool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id`, slug, slug).Scan(&id); err != nil {
			t.Fatalf("create workspace %s: %v", slug, err)
		}
		return id
	}
	targetID := createWorkspace("handler-content-delete-target-" + suffix)
	neighbourID := createWorkspace("handler-content-delete-neighbour-" + suffix)
	t.Cleanup(func() {
		for _, id := range []string{targetID, neighbourID} {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM content_technical_log WHERE workspace_id = $1`, id)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM content_operation_audit WHERE workspace_id = $1`, id)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM content_diagnostic_run WHERE workspace_id = $1`, id)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, id)
		}
	})
	if _, err := testPool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, targetID, testUserID); err != nil {
		t.Fatalf("add target owner: %v", err)
	}
	for _, id := range []string{targetID, neighbourID} {
		for _, entry := range []struct {
			table string
			id    string
			key   string
		}{
			{"content_diagnostic_run", "run-" + id, "run_id"},
			{"content_operation_audit", "audit-" + id, "event_id"},
			{"content_technical_log", "log-" + id, "event_id"},
		} {
			if _, err := testPool.Exec(ctx, `INSERT INTO `+entry.table+` (`+entry.key+`, workspace_id, payload) VALUES ($1, $2, '{}'::jsonb)`, entry.id, id); err != nil {
				t.Fatalf("seed %s for %s: %v", entry.table, id, err)
			}
		}
	}

	functionName := "handler_test_fail_content_workspace_delete_" + suffix
	triggerName := "handler_test_fail_content_workspace_delete_trigger_" + suffix
	if _, err := testPool.Exec(ctx, `CREATE FUNCTION `+functionName+` () RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.id::text = TG_ARGV[0] THEN RAISE EXCEPTION 'forced rollback'; END IF; RETURN OLD; END $$`); err != nil {
		t.Fatalf("create rollback trigger function: %v", err)
	}
	// Register this before attempting the trigger: if trigger creation fails, the
	// unique function cannot pollute a later test run. Cleanup is LIFO, so this
	// runs before the workspace fixture cleanup registered above.
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DROP FUNCTION IF EXISTS `+functionName+`()`)
	})
	if _, err := testPool.Exec(ctx, `CREATE TRIGGER `+triggerName+` BEFORE DELETE ON workspace FOR EACH ROW EXECUTE FUNCTION `+functionName+`('`+targetID+`')`); err != nil {
		t.Fatalf("create rollback trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+triggerName+` ON workspace`)
	})

	request := newRequest(http.MethodDelete, "/api/workspaces/"+targetID, nil)
	request = withURLParam(request, "id", targetID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusInternalServerError)
	for _, table := range []string{"content_diagnostic_run", "content_operation_audit", "content_technical_log"} {
		var count int
		if err := testPool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, targetID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("rollback %s count=%d err=%v, want 1", table, count, err)
		}
	}
	if _, err := testPool.Exec(ctx, `DROP TRIGGER `+triggerName+` ON workspace; DROP FUNCTION `+functionName+`() `); err != nil {
		t.Fatalf("drop rollback trigger: %v", err)
	}
	for _, object := range []struct{ kind, name string }{{"trigger", triggerName}, {"function", functionName}} {
		var count int
		query := `SELECT count(*) FROM pg_trigger WHERE tgname = $1`
		if object.kind == "function" {
			query = `SELECT count(*) FROM pg_proc WHERE proname = $1`
		}
		if err := testPool.QueryRow(ctx, query, object.name).Scan(&count); err != nil || count != 0 {
			t.Fatalf("cleanup left %s %s count=%d err=%v", object.kind, object.name, count, err)
		}
	}

	request = newRequest(http.MethodDelete, "/api/workspaces/"+targetID, nil)
	request = withURLParam(request, "id", targetID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)
	for _, table := range []string{"content_diagnostic_run", "content_operation_audit", "content_technical_log"} {
		for workspaceID, want := range map[string]int{targetID: 0, neighbourID: 1} {
			var count int
			if err := testPool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, workspaceID).Scan(&count); err != nil || count != want {
				t.Fatalf("%s workspace %s count=%d err=%v, want %d", table, workspaceID, count, err, want)
			}
		}
	}
}
