package handler

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestRequestHasClientCapability(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "absent"},
		{name: "exact", header: protocol.DaemonCapabilityCoalescedCommentsV1, want: true},
		{name: "comma separated and trimmed", header: "skill-bundles-v1,  coalesced-comments-v1  ", want: true},
		{name: "substring", header: "xcoalesced-comments-v1", want: false},
		{name: "case sensitive", header: "Coalesced-Comments-V1", want: false},
		{name: "unknown", header: "future-comments-v2", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/claim", nil)
			if tc.header != "" {
				req.Header.Set("X-Client-Capabilities", tc.header)
			}
			if got := requestHasClientCapability(req, protocol.DaemonCapabilityCoalescedCommentsV1); got != tc.want {
				t.Fatalf("requestHasClientCapability(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

func TestSelectCommentDelivery_BudgetKeepsTriggerAndStablePrefix(t *testing.T) {
	comments := []CoalescedCommentData{
		{ID: "00000000-0000-0000-0000-000000000001", Content: "oldest"},
		{ID: "00000000-0000-0000-0000-000000000002", Content: "overflow"},
		{ID: "00000000-0000-0000-0000-000000000003", Content: "trigger"},
	}
	triggerID := comments[2].ID
	limit := commentDeliveryBaseSize(false) +
		commentDeliveryEntrySize(comments[2], false) +
		commentDeliveryEntrySize(comments[0], false)

	selected := selectCommentDelivery(comments, triggerID, false, limit)
	got := make([]string, 0, len(selected))
	for _, comment := range selected {
		got = append(got, comment.ID)
	}
	want := []string{comments[0].ID, comments[2].ID}
	if !slices.Equal(got, want) {
		t.Fatalf("selected ids = %v, want stable prefix + trigger %v", got, want)
	}
}

func TestFormatLegacyCommentBundle_PreservesBodiesAndOrder(t *testing.T) {
	comments := []CoalescedCommentData{
		{ID: "00000000-0000-0000-0000-000000000001", ThreadID: "thread-a", AuthorType: "member", AuthorName: "A", Content: "  first body\n", CreatedAt: "2026-07-10T01:00:00Z"},
		{ID: "00000000-0000-0000-0000-000000000002", ThreadID: "thread-b", AuthorType: "agent", AuthorName: "B", Content: "second body", CreatedAt: "2026-07-10T02:00:00Z"},
	}
	bundle := formatLegacyCommentBundle(comments)
	for _, want := range []string{comments[0].ID, comments[1].ID, "thread-a", "thread-b", "member: A", "agent: B", comments[0].Content, comments[1].Content} {
		if !strings.Contains(bundle, want) {
			t.Fatalf("legacy bundle missing %q:\n%s", want, bundle)
		}
	}
	if strings.Index(bundle, comments[0].ID) > strings.Index(bundle, comments[1].ID) {
		t.Fatalf("legacy bundle is not chronological:\n%s", bundle)
	}
}

func TestCommentDeliveryEntrySize_AccountsForLegacyJSONEscaping(t *testing.T) {
	comment := CoalescedCommentData{
		ID:      "00000000-0000-0000-0000-000000000001",
		Content: `quotes " backslashes \\ and html <>&`,
	}
	raw := len(formatLegacyCommentEntry(comment))
	if got := commentDeliveryEntrySize(comment, true); got <= raw {
		t.Fatalf("legacy escaped size = %d, want greater than raw size %d", got, raw)
	}
}

type commentDeliveryFixture struct {
	runtimeID string
	agentID   string
	issueID   string
	taskID    string
	commentID []string
	threadID  []string
	content   []string
}

type failNthBegin struct {
	delegate *pgxpool.Pool
	failAt   int
	calls    int
}

type failDeleteCommentDB struct {
	delegate db.DBTX
}

type zeroDeleteCommentDB struct {
	delegate db.DBTX
}

type deleteCommentResultRow struct {
	changed bool
	err     error
}

func (r deleteCommentResultRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*bool)) = r.changed
	*(dest[1].(*int64)) = 0
	return nil
}

func (f *failDeleteCommentDB) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	if strings.Contains(query, "-- name: DeleteComment") {
		return pgconn.CommandTag{}, errors.New("injected comment deletion failure")
	}
	return f.delegate.Exec(ctx, query, args...)
}

func (f *failDeleteCommentDB) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return f.delegate.Query(ctx, query, args...)
}

func (f *failDeleteCommentDB) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	if strings.Contains(query, "-- name: DeleteComment") {
		return deleteCommentResultRow{err: errors.New("injected comment deletion failure")}
	}
	return f.delegate.QueryRow(ctx, query, args...)
}

func (z *zeroDeleteCommentDB) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	if strings.Contains(query, "-- name: DeleteComment") {
		return pgconn.NewCommandTag("DELETE 0"), nil
	}
	return z.delegate.Exec(ctx, query, args...)
}

func (z *zeroDeleteCommentDB) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return z.delegate.Query(ctx, query, args...)
}

func (z *zeroDeleteCommentDB) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	if strings.Contains(query, "-- name: DeleteComment") {
		return deleteCommentResultRow{changed: false}
	}
	return z.delegate.QueryRow(ctx, query, args...)
}

func (f *failNthBegin) Begin(ctx context.Context) (pgx.Tx, error) {
	f.calls++
	if f.calls == f.failAt {
		return nil, errors.New("injected claim finalization transaction failure")
	}
	return f.delegate.Begin(ctx)
}
