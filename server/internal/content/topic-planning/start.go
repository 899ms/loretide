package topicplanning

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
)

// Starting work on a brief revision (EP-04b).
//
// A start fixes the configuration of the moment into an append-only snapshot,
// so that changing the account, the preference, the brief or the brand switch
// afterwards cannot rewrite what a run was started with.
//
// One brief revision may be started many times. Starting the same revision
// again with a different material scope is an ordinary thing to want, and it
// produces a second snapshot rather than a conflict - so this path is NOT
// idempotent, and must not grow a guard that makes it look like it is.
//
// Contract: specs/023-ep04b-start-snapshot/contracts/start-snapshot.md

// ErrNotReady is an account that has not met SOP 3.1's minimum condition to
// start. Answered as 400 with the missing field names, deliberately NOT as the
// 404 that hides other people's resources: the gap is in the caller's own
// account, and naming it reveals nothing.
var ErrNotReady = errors.New("account has not met the minimum condition to start")

// ReadinessError carries which fields are still missing.
type ReadinessError struct{ Missing []string }

func (e ReadinessError) Error() string { return ErrNotReady.Error() }
func (e ReadinessError) Unwrap() error { return ErrNotReady }

// StartRequest is what the caller chose for this start.
//
// AutoPrecheck comes from the handler rather than being read here: the brand
// switch is workspace configuration owned by the workspace adapter, and this
// module has no reason to learn how to read a workspace row.
type StartRequest struct {
	AccountID   string `json:"account_id"`
	SourceScope string `json:"source_scope"`
	ProjectID   string `json:"project_id"`

	// AutoPrecheck is not part of the request body - the handler fills it from
	// the brand's settings. A caller that could set it would be writing its own
	// history.
	AutoPrecheck bool `json:"-"`
}

// StartSnapshot is one recorded start.
type StartSnapshot struct {
	SnapshotID      string         `json:"snapshot_id"`
	WorkspaceID     string         `json:"workspace_id"`
	TopicCardID     string         `json:"topic_card_id"`
	BriefRevisionID string         `json:"brief_revision_id"`
	AccountID       string         `json:"account_id"`
	ProjectID       string         `json:"project_id"`
	ActorID         string         `json:"actor_id"`
	Snapshot        StoredSnapshot `json:"snapshot"`
	CreatedAt       time.Time      `json:"created_at"`
}

const snapshotSelect = `SELECT snapshot_id, workspace_id, topic_card_id,
	brief_revision_id, account_id, project_id, actor_id, snapshot, created_at
	FROM content_start_snapshot`

// Start records one start of one brief revision.
//
// Order matters and follows the contract: the workspace, then the card, then
// the revision, then the account all answer with the same refusal, so a caller
// cannot learn from the shape of a rejection which of the four exists. Only
// once all four are its own does the readiness gap become visible.
func (s *Store) Start(ctx context.Context, workspaceID, actor, topicCardID, revisionID string, request StartRequest) (StartSnapshot, error) {
	if s == nil {
		return StartSnapshot{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || topicCardID == "" || revisionID == "" || request.AccountID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", ErrInvalid)
		return StartSnapshot{}, ErrInvalid
	}
	if err := ipprofile.ValidateScope(request.SourceScope); err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", ErrInvalid)
		return StartSnapshot{}, ErrInvalid
	}
	if s.Accounts == nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", ErrStorage)
		return StartSnapshot{}, ErrStorage
	}

	// The account is read before the fence is taken so that a caller asking
	// about someone else's account never reaches a write transaction at all.
	account, err := s.Accounts.Get(ctx, workspaceID, request.AccountID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", ErrNotFound)
		return StartSnapshot{}, ErrNotFound
	}
	revision, err := s.Accounts.CurrentPersonaRevision(ctx, workspaceID, request.AccountID)
	if err != nil && !errors.Is(err, ipprofile.ErrNotFound) {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", err)
		return StartSnapshot{}, ErrStorage
	}

	// Checked here and NOT only in the browser. The page's own readiness call
	// decides whether a button is clickable; it is not an authorization, and
	// the account can stop being ready between the two.
	readiness := ipprofile.ProfileReadiness(revision.Profile)
	if !readiness.CanStart {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", ErrInvalid)
		return StartSnapshot{}, ReadinessError{Missing: readiness.Missing}
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", err)
		return StartSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	ctx, err = s.audit(ctx, tx, workspaceID, actor, topicCardID, "start")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", err)
		return StartSnapshot{}, err
	}

	// The revision is re-read inside the transaction, scoped to the workspace
	// AND the card: a revision id that belongs to another card, or another
	// brand, must be refused exactly as a missing one.
	brief, err := scanBrief(tx.QueryRow(ctx, briefSelect+
		` WHERE workspace_id=$1 AND topic_card_id=$2 AND brief_revision_id=$3`,
		workspaceID, topicCardID, revisionID))
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", ErrNotFound)
		return StartSnapshot{}, ErrNotFound
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", err)
		return StartSnapshot{}, ErrStorage
	}

	// The brief's own source_scope is SOP 5.3 prose and is NOT the controlled
	// preference. It rides the revision and takes no part here; reading it as
	// the scope would need a controlled set 022 deliberately does not have.
	_ = brief.SourceScope

	stored := AssembleStored(SnapshotInputs{
		ChosenScope:           request.SourceScope,
		SavedPreference:       ipprofile.ScopeOf(account.Settings),
		PersonaRef:            revision.RevisionID,
		AutoPrecheck:          request.AutoPrecheck,
		UsesNeutralExpression: ipprofile.UsesNeutralExpression(revision.Profile),
	})
	payload, err := json.Marshal(stored)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", err)
		return StartSnapshot{}, ErrStorage
	}

	snapshot := StartSnapshot{
		SnapshotID:      s.newID(),
		WorkspaceID:     workspaceID,
		TopicCardID:     topicCardID,
		BriefRevisionID: revisionID,
		AccountID:       request.AccountID,
		ProjectID:       request.ProjectID,
		ActorID:         actor,
		Snapshot:        stored,
	}
	if err = tx.QueryRow(ctx, `INSERT INTO content_start_snapshot
		(snapshot_id, workspace_id, topic_card_id, brief_revision_id, account_id,
		 project_id, actor_id, snapshot)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at`,
		snapshot.SnapshotID, workspaceID, topicCardID, revisionID, request.AccountID,
		request.ProjectID, actor, payload).Scan(&snapshot.CreatedAt); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", err)
		return StartSnapshot{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", err)
		return StartSnapshot{}, ErrStorage
	}
	return snapshot, nil
}

// ListSnapshots returns every start of one brief revision, newest first.
func (s *Store) ListSnapshots(ctx context.Context, workspaceID, actor, topicCardID, revisionID string) ([]StartSnapshot, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-snapshots", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || topicCardID == "" || revisionID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-snapshots", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, snapshotSelect+
		` WHERE workspace_id=$1 AND topic_card_id=$2 AND brief_revision_id=$3
		  ORDER BY created_at DESC, snapshot_id`,
		workspaceID, topicCardID, revisionID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-snapshots", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	snapshots := []StartSnapshot{}
	for rows.Next() {
		snapshot, scanErr := scanSnapshot(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-snapshots", scanErr)
			return nil, ErrStorage
		}
		snapshots = append(snapshots, snapshot)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-snapshots", rows.Err())
		return nil, ErrStorage
	}
	return snapshots, nil
}

// GetSnapshot returns one start by its stable key.
//
// Scoped to the workspace AND the card: a snapshot id from another brand is
// refused exactly as a missing one, so an id cannot be used to probe.
func (s *Store) GetSnapshot(ctx context.Context, workspaceID, actor, topicCardID, snapshotID string) (StartSnapshot, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "get-snapshot", ErrStorage)
		}
		return StartSnapshot{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || topicCardID == "" || snapshotID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "get-snapshot", ErrInvalid)
		return StartSnapshot{}, ErrInvalid
	}
	snapshot, err := scanSnapshot(s.DB.QueryRow(ctx, snapshotSelect+
		` WHERE workspace_id=$1 AND topic_card_id=$2 AND snapshot_id=$3`,
		workspaceID, topicCardID, snapshotID))
	if errors.Is(err, pgx.ErrNoRows) {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "get-snapshot", ErrNotFound)
		return StartSnapshot{}, ErrNotFound
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "get-snapshot", err)
		return StartSnapshot{}, ErrStorage
	}
	return snapshot, nil
}

func scanSnapshot(row scanner) (StartSnapshot, error) {
	var snapshot StartSnapshot
	var payload []byte
	if err := row.Scan(&snapshot.SnapshotID, &snapshot.WorkspaceID, &snapshot.TopicCardID,
		&snapshot.BriefRevisionID, &snapshot.AccountID, &snapshot.ProjectID,
		&snapshot.ActorID, &payload, &snapshot.CreatedAt); err != nil {
		return StartSnapshot{}, err
	}
	if err := json.Unmarshal(payload, &snapshot.Snapshot); err != nil {
		return StartSnapshot{}, err
	}
	return snapshot, nil
}
