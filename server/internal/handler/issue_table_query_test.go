package handler

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type issueTableEnrichmentFailTxStarter struct {
	inner           txStarter
	labelCalls      *int
	tableQueryCalls *int
	facetQueryCalls *int
	rowQuerySQL     *string
	groupQuerySQL   *string
}

func (s issueTableEnrichmentFailTxStarter) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.inner.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &issueTableEnrichmentFailTx{
		Tx:              tx,
		labelCalls:      s.labelCalls,
		tableQueryCalls: s.tableQueryCalls,
		facetQueryCalls: s.facetQueryCalls,
		rowQuerySQL:     s.rowQuerySQL,
		groupQuerySQL:   s.groupQuerySQL,
	}, nil
}

type issueTableEnrichmentFailTx struct {
	pgx.Tx
	labelCalls      *int
	tableQueryCalls *int
	facetQueryCalls *int
	rowQuerySQL     *string
	groupQuerySQL   *string
}

func (tx *issueTableEnrichmentFailTx) recordTableQuery(sql string) {
	if tx.tableQueryCalls != nil {
		if strings.Contains(sql, "page AS MATERIALIZED (") ||
			strings.Contains(sql, "SELECT COUNT(*)::bigint FROM issue i WHERE") {
			*tx.tableQueryCalls = *tx.tableQueryCalls + 1
		}
	}
	if tx.facetQueryCalls != nil && strings.Contains(sql, "GROUP BY GROUPING SETS") {
		*tx.facetQueryCalls = *tx.facetQueryCalls + 1
	}
	if tx.rowQuerySQL != nil && strings.Contains(sql, "page AS MATERIALIZED (") {
		*tx.rowQuerySQL = sql
	}
	if tx.groupQuerySQL != nil && strings.Contains(sql, "WITH grouped AS (") {
		*tx.groupQuerySQL = sql
	}
}

func (tx *issueTableEnrichmentFailTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	tx.recordTableQuery(sql)
	if strings.Contains(sql, "ListLabelsForIssues") {
		*tx.labelCalls = *tx.labelCalls + 1
		// A real PostgreSQL statement error poisons the transaction until
		// rollback. Before enrichment moved after Commit, this turned the
		// otherwise successful row window into a 500.
		_, err := tx.Tx.Exec(ctx, "SELECT * FROM issue_table_missing_enrichment_relation")
		return nil, err
	}
	return tx.Tx.Query(ctx, sql, args...)
}

func (tx *issueTableEnrichmentFailTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	tx.recordTableQuery(sql)
	return tx.Tx.QueryRow(ctx, sql, args...)
}

func TestCanonicalIssueTableFingerprintNormalizesSetLikeArrays(t *testing.T) {
	left := issueTableQuerySpec{
		Scope: issueTableScope{Kind: "workspace", AssigneeTypes: []string{"agent", "member", "agent"}},
		Filters: issueTableFiltersRequest{
			Statuses:   []string{"todo", "backlog", "todo"},
			ProjectIDs: []string{"b", "a"},
		},
		Sort: issueTableSortRequest{Field: "title", Direction: "asc"},
	}
	right := issueTableQuerySpec{
		Scope: issueTableScope{Kind: "workspace", AssigneeTypes: []string{"member", "agent"}},
		Filters: issueTableFiltersRequest{
			Statuses:   []string{"backlog", "todo"},
			ProjectIDs: []string{"a", "b"},
		},
		Sort: issueTableSortRequest{Field: "title", Direction: "asc"},
	}
	leftFingerprint, err := canonicalIssueTableFingerprint("workspace-1", left)
	if err != nil {
		t.Fatal(err)
	}
	rightFingerprint, err := canonicalIssueTableFingerprint("workspace-1", right)
	if err != nil {
		t.Fatal(err)
	}
	if leftFingerprint != rightFingerprint {
		t.Fatalf("equivalent table queries produced different fingerprints: %s != %s", leftFingerprint, rightFingerprint)
	}
}

func TestCanonicalIssueTableFingerprintBindsWorkspace(t *testing.T) {
	spec := issueTableQuerySpec{
		Scope: issueTableScope{Kind: "workspace"},
		Sort:  issueTableSortRequest{Field: "position", Direction: "asc"},
	}
	left, err := canonicalIssueTableFingerprint("workspace-1", spec)
	if err != nil {
		t.Fatal(err)
	}
	right, err := canonicalIssueTableFingerprint("workspace-2", spec)
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("equivalent queries in different workspaces produced the same fingerprint")
	}
}

func TestIssueTableCursorRejectsAnotherQuery(t *testing.T) {
	groupKey := "status:todo"
	cursor := issueTableCursor{
		Version:          1,
		QueryFingerprint: "sha256:old",
		GroupKey:         &groupKey,
	}
	w := httptest.NewRecorder()
	if issueTableCursorMatches(w, &cursor, "sha256:new", &groupKey, nil) {
		t.Fatal("cursor from another query unexpectedly matched")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestIssueTablePositionCursorIncludesIndexableLowerBound(t *testing.T) {
	cursorValue := "90000"
	cursor := issueTableCursor{
		SortValue:    &cursorValue,
		RowCreatedAt: "2026-01-01T00:00:00Z",
		RowID:        "00000000-0000-4000-8000-000000000001",
	}
	args := make([]any, 0, 3)
	predicate, ok := (resolvedIssueTableSort{
		expression: "i.position",
		direction:  "asc",
		castType:   "double precision",
	}).cursorPredicate(httptest.NewRecorder(), &cursor, func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	})
	if !ok {
		t.Fatal("valid position cursor was rejected")
	}
	if !strings.Contains(predicate, "i.position >= $3::double precision") {
		t.Fatalf("position cursor is missing its indexable lower bound: %s", predicate)
	}
}

func TestIssueTableGroupIdentityBindsIncludeEmpty(t *testing.T) {
	withoutEmpty := issueTableGroupIdentity(issueTableGroupSpec{
		Kind:       "property",
		PropertyID: "00000000-0000-4000-8000-000000000001",
	})
	withEmpty := issueTableGroupIdentity(issueTableGroupSpec{
		Kind:         "property",
		PropertyID:   "00000000-0000-4000-8000-000000000001",
		IncludeEmpty: true,
	})
	if withoutEmpty == withEmpty {
		t.Fatalf("include-empty property cursors share an identity: %q", withEmpty)
	}
}

func TestIssueTableCompoundCellKeyResolvesPrimaryAndStatus(t *testing.T) {
	primary := resolvedIssueTableGroup{kind: "parent"}
	compound := resolvedIssueTableGroup{kind: "compound", primary: &primary}
	key := compoundCellGroupKey(
		"parent:00000000-0000-4000-8000-000000000001",
		"todo",
		false,
	)
	args := make([]any, 0, 2)
	predicate, ok := compound.predicate(
		httptest.NewRecorder(),
		key,
		func(value any) string {
			args = append(args, value)
			return fmt.Sprintf("$%d", len(args))
		},
	)
	if !ok {
		t.Fatal("valid compound cell key was rejected")
	}
	if !strings.Contains(predicate, "i.parent_issue_id = $1::uuid") ||
		!strings.Contains(predicate, "i.status = $2::text") ||
		len(args) != 2 || args[1] != "todo" {
		t.Fatalf("compound cell predicate lost a dimension: %s args=%#v", predicate, args)
	}
}
