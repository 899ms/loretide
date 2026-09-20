package handler

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"strings"
	"testing"
)

type failChatInputQueryDB struct {
	delegate db.DBTX
}

func (f *failChatInputQueryDB) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return f.delegate.Exec(ctx, query, args...)
}

func (f *failChatInputQueryDB) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	if strings.Contains(query, "-- name: ListChatInputMessages") {
		return nil, errors.New("injected chat input load failure")
	}
	return f.delegate.Query(ctx, query, args...)
}

func (f *failChatInputQueryDB) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return f.delegate.QueryRow(ctx, query, args...)
}

func TestChatSessionResumeFallbackNeeded(t *testing.T) {
	tests := []struct {
		name           string
		priorSessionID string
		priorWorkDir   string
		want           bool
	}{
		{name: "both present", priorSessionID: "session", priorWorkDir: "/work", want: false},
		{name: "session missing", priorWorkDir: "/work", want: true},
		{name: "workdir missing", priorSessionID: "session", want: true},
		{name: "both missing", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatSessionResumeFallbackNeeded(tt.priorSessionID, tt.priorWorkDir); got != tt.want {
				t.Fatalf("chatSessionResumeFallbackNeeded(%q, %q) = %v, want %v", tt.priorSessionID, tt.priorWorkDir, got, tt.want)
			}
		})
	}
}
