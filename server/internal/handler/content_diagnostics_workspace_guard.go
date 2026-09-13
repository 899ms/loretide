package handler

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type contentDiagnosticsWorkspaceWriteGuard struct{ queries *db.Queries }

func newContentDiagnosticsWorkspaceWriteGuard(queries *db.Queries) diagnostics.WorkspaceWriteGuard {
	return contentDiagnosticsWorkspaceWriteGuard{queries: queries}
}

func (g contentDiagnosticsWorkspaceWriteGuard) LockForContentDiagnosticWrite(ctx context.Context, tx pgx.Tx, workspaceID string) error {
	workspaceUUID, err := util.ParseUUID(workspaceID)
	if err != nil {
		return diagnostics.ErrDenied
	}
	if _, err = g.queries.WithTx(tx).LockWorkspaceForContentDiagnosticWrite(ctx, workspaceUUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return diagnostics.ErrDenied
		}
		return diagnostics.ErrUnavailable
	}
	return nil
}

// NewContentDiagnosticsStore composes the diagnostics store with workspace-core's
// delete/write protocol. Production callers cannot construct this path without
// the guard; isolated diagnostics fixtures inject their own explicit guard.
func (h *Handler) NewContentDiagnosticsStore(pool *pgxpool.Pool) *diagnostics.Store {
	return diagnostics.NewStore(pool, newContentDiagnosticsWorkspaceWriteGuard(h.Queries))
}
