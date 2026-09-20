package handler

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// enqueueViaRealQuery creates a task through the generated CreateAgentTask query —
// the actual production enqueue path, fence included. Raw INSERTs in tests would
// bypass the fence, which lives in the application SQL rather than in a trigger.
func enqueueViaRealQuery(ctx context.Context, q *db.Queries, agentID, runtimeID, issueID string) error {
	params := db.CreateAgentTaskParams{
		AgentID:   parseUUID(agentID),
		RuntimeID: parseUUID(runtimeID),
		Priority:  0,
	}
	if issueID != "" {
		params.IssueID = parseUUID(issueID)
	}
	_, err := q.CreateAgentTask(ctx, params)
	return err
}

var _ = pgtype.UUID{}
