package handler

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestBatchedHeartbeatScheduler_RaceToOfflineSelfHeals confirms that if the
// sweeper flips a row offline between Schedule and FlushNow, receipt
// reconciliation restores it without a new heartbeat lookup.
// recordingRecoveryNotifier captures RuntimeRecoveryNotifier calls so tests
// can assert a lifecycle refresh fires for the already-known workspace.
type recordingRecoveryNotifier struct {
	workspaceIDs []string
	contexts     []context.Context
}

func (r *recordingRecoveryNotifier) NotifyRuntimeRecovered(ctx context.Context, workspaceID string) {
	r.workspaceIDs = append(r.workspaceIDs, workspaceID)
	r.contexts = append(r.contexts, ctx)
}

type recoveryContextKey struct{}

// silenceUnusedPgUUID ensures the package compiles even if no other test
// happens to reference pgtype after future edits trim imports.
var _ = pgtype.UUID{}
