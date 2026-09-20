package reviewdelivery

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Delivery tasks (SOP 9.1, 9.2, 9.3).
//
// Nothing in this file schedules anything. scheduled_at is stored and read;
// SOP 9.1's "到期后产生站内待办" is IsDue, a comparison made when the list is
// read. A search test asserts no other code reads that column.

// CreateTaskRequest is a new handover to-do.
type CreateTaskRequest struct {
	ArtifactID string  `json:"artifact_id"`
	Channel    Channel `json:"channel"`
	// ReviewRequestID may be empty: drafting a task while the review is still
	// pending is ordinary. From ready onwards it is required AND has to be
	// approved.
	ReviewRequestID string `json:"review_request_id"`
}

// CreateTask opens a delivery task in draft.
func (s *Store) CreateTask(ctx context.Context, workspaceID, actor string, request CreateTaskRequest) (DeliveryTask, error) {
	if s == nil {
		return DeliveryTask{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || request.ArtifactID == "" {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "create-task", ErrInvalid)
		return DeliveryTask{}, invalidField("artifact_id")
	}
	if err := ValidateChannel(string(request.Channel)); err != nil {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "create-task", err)
		return DeliveryTask{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "create-task", err)
		return DeliveryTask{}, err
	}
	defer tx.Rollback(ctx)

	workID := ""
	if request.ReviewRequestID != "" {
		// A referenced request has to be this workspace's. Its STATUS is not
		// checked here: draft is exactly the stage where it may still be
		// pending.
		var reviewArtifact string
		err = tx.QueryRow(ctx, `SELECT work_id, artifact_id FROM content_review_request
			WHERE workspace_id=$1 AND review_request_id=$2`,
			workspaceID, request.ReviewRequestID).Scan(&workID, &reviewArtifact)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "create-task", ErrNotFound)
			return DeliveryTask{}, ErrNotFound
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "create-task", err)
			return DeliveryTask{}, ErrStorage
		}
		if reviewArtifact != request.ArtifactID {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "create-task", ErrInvalid)
			return DeliveryTask{}, invalidField("review_request_id")
		}
	} else if s.Artifacts != nil {
		// No request to take the work id from, so the document itself answers.
		// Any version of it will do - the task points at the document, and
		// which version is delivered comes from the approved request.
		resolved, _, resolveErr := s.Artifacts.ResolveVersion(ctx, workspaceID, request.ArtifactID, "")
		if resolveErr != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "create-task", ErrNotFound)
			return DeliveryTask{}, ErrNotFound
		}
		workID = resolved
	}

	created := DeliveryTask{
		DeliveryTaskID: s.newID(), WorkspaceID: workspaceID, WorkID: workID,
		ArtifactID: request.ArtifactID, ReviewRequestID: request.ReviewRequestID,
		Channel: request.Channel, Status: InitialDeliveryStatus,
	}
	ctx, err = s.audit(ctx, tx, workspaceID, actor, created.DeliveryTaskID, "create-task")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.DeliveryTaskID, "create-task", err)
		return DeliveryTask{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO content_delivery_task
		(delivery_task_id, workspace_id, work_id, artifact_id, review_request_id,
		 channel, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at, updated_at`,
		created.DeliveryTaskID, workspaceID, workID, request.ArtifactID,
		request.ReviewRequestID, string(request.Channel), string(InitialDeliveryStatus)).
		Scan(&created.CreatedAt, &created.UpdatedAt); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.DeliveryTaskID, "create-task", err)
		return DeliveryTask{}, ErrStorage
	}
	if err = s.recordTransition(ctx, tx, workspaceID, actor, SubjectDeliveryTask,
		created.DeliveryTaskID, "", string(InitialDeliveryStatus), ""); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.DeliveryTaskID, "create-task", err)
		return DeliveryTask{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, created.DeliveryTaskID, "create-task", err)
		return DeliveryTask{}, ErrStorage
	}
	return created, nil
}

// Advance moves a task to another status.
//
// Decision order, first failure wins: the controlled values and the conditional
// requirements (ValidateDeliveryAdvance), then the approved-review precondition,
// then the transition itself. Advancing to handed_off writes NO publication
// record - SOP 9.1 says handing over is not publishing, and a test asserts the
// count stays zero.
func (s *Store) Advance(ctx context.Context, workspaceID, actor, taskID string, advance DeliveryAdvance) (DeliveryTask, error) {
	if s == nil {
		return DeliveryTask{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || taskID == "" {
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", ErrInvalid)
		return DeliveryTask{}, ErrInvalid
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", err)
		return DeliveryTask{}, err
	}
	defer tx.Rollback(ctx)

	var current DeliveryStatus
	var reviewRequestID string
	err = tx.QueryRow(ctx, `SELECT status, review_request_id FROM content_delivery_task
		WHERE workspace_id=$1 AND delivery_task_id=$2 FOR UPDATE`,
		workspaceID, taskID).Scan(&current, &reviewRequestID)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", ErrNotFound)
		return DeliveryTask{}, ErrNotFound
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", err)
		return DeliveryTask{}, ErrStorage
	}
	if err = ValidateDeliveryAdvance(current, advance); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", err)
		return DeliveryTask{}, err
	}
	// Cancelling never needs an approval behind it: abandoning a piece is
	// allowed at every stage, and requiring a review to do it would strand
	// tasks whose review was itself cancelled.
	if RequiresApprovedReview(advance.To) && advance.To != DeliveryCancelled {
		if err = s.requireApproved(ctx, tx, workspaceID, reviewRequestID); err != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", err)
			return DeliveryTask{}, err
		}
	}
	ctx, err = s.audit(ctx, tx, workspaceID, actor, taskID, "advance-task")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", err)
		return DeliveryTask{}, err
	}
	task, err := s.writeStatus(ctx, tx, workspaceID, taskID, advance)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", err)
		return DeliveryTask{}, ErrStorage
	}
	if err = s.recordTransition(ctx, tx, workspaceID, actor, SubjectDeliveryTask,
		taskID, string(current), string(advance.To), advance.Reason); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", err)
		return DeliveryTask{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, taskID, "advance-task", err)
		return DeliveryTask{}, ErrStorage
	}
	return task, nil
}

// requireApproved is the precondition from ruling Q2: past draft, the task's
// referenced review must exist and be approved.
//
// It answers ErrInvalid naming the field rather than 404: the request is the
// caller's own, so saying "that one is not approved" reveals nothing.
func (s *Store) requireApproved(ctx context.Context, tx pgx.Tx, workspaceID, reviewRequestID string) error {
	if reviewRequestID == "" {
		return invalidField("review_request_id")
	}
	var status ReviewStatus
	err := tx.QueryRow(ctx, `SELECT status FROM content_review_request
		WHERE workspace_id=$1 AND review_request_id=$2`, workspaceID, reviewRequestID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return ErrStorage
	}
	if status != ReviewApproved {
		return invalidField("review_request_id")
	}
	return nil
}

// writeStatus is the one UPDATE of content_delivery_task.
//
// handoff_method and scheduled_at are only written when the target status is
// the one that carries them, so moving to held does not silently blank a
// planned time the person may still want when the task comes back to ready.
func (s *Store) writeStatus(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, advance DeliveryAdvance) (DeliveryTask, error) {
	scheduledAt := advance.ScheduledAt
	method := string(advance.HandoffMethod)
	return scanDelivery(tx.QueryRow(ctx, `UPDATE content_delivery_task
		SET status=$3,
		    scheduled_at=CASE WHEN $3='scheduled' THEN $4::timestamptz ELSE scheduled_at END,
		    handoff_method=CASE WHEN $3='handed_off' THEN $5::text ELSE handoff_method END,
		    updated_at=now()
		WHERE workspace_id=$1 AND delivery_task_id=$2
		RETURNING delivery_task_id, workspace_id, work_id, artifact_id,
			review_request_id, channel, status, scheduled_at, handoff_method,
			created_at, updated_at`,
		workspaceID, taskID, string(advance.To), scheduledAt, method))
}

// HoldForChangedTarget is SOP 9.3: the task's approved snapshot no longer
// describes what is about to be delivered, so the task goes back to held until
// a new snapshot is approved.
//
// The caller passes the current target; ShouldHold decides and names which of
// the four changed. Returning the task unchanged (and false) when nothing moved
// is deliberate - this is called on every target edit, and a no-op must not
// write a transition row saying something happened.
func (s *Store) HoldForChangedTarget(ctx context.Context, workspaceID, actor, taskID string, current DeliveryTarget) (DeliveryTask, bool, error) {
	if s == nil {
		return DeliveryTask{}, false, ErrStorage
	}
	if workspaceID == "" || actor == "" || taskID == "" {
		s.reportFailure(ctx, workspaceID, actor, taskID, "hold-task", ErrInvalid)
		return DeliveryTask{}, false, ErrInvalid
	}
	task, err := s.GetTask(ctx, workspaceID, actor, taskID)
	if err != nil {
		return DeliveryTask{}, false, err
	}
	if !HoldableStatus(task.Status) || task.ReviewRequestID == "" {
		return task, false, nil
	}
	request, err := s.GetReview(ctx, workspaceID, actor, task.ReviewRequestID)
	if err != nil {
		return DeliveryTask{}, false, err
	}
	hold, reason := ShouldHold(TargetOf(request.Snapshot), current)
	if !hold {
		return task, false, nil
	}
	held, err := s.Advance(ctx, workspaceID, actor, taskID, DeliveryAdvance{
		To: DeliveryHeld, Reason: reason,
	})
	if err != nil {
		return DeliveryTask{}, false, err
	}
	return held, true, nil
}

// KeepDeliveringApprovedSnapshot is SOP 9.3's other exit: the person looks at
// the version number and the preview and says "deliver the old approved one
// anyway".
//
// It is a deliberate action with its own recorded reason, never a default.
func (s *Store) KeepDeliveringApprovedSnapshot(ctx context.Context, workspaceID, actor, taskID, note string) (DeliveryTask, error) {
	reason := "keep delivering the approved snapshot"
	if note != "" {
		reason = reason + ": " + note
	}
	return s.Advance(ctx, workspaceID, actor, taskID, DeliveryAdvance{
		To: DeliveryReady, Reason: reason,
	})
}

func (s *Store) GetTask(ctx context.Context, workspaceID, actor, taskID string) (DeliveryTask, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, taskID, "get-task", ErrStorage)
		}
		return DeliveryTask{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || taskID == "" {
		s.reportFailure(ctx, workspaceID, actor, taskID, "get-task", ErrInvalid)
		return DeliveryTask{}, ErrInvalid
	}
	task, err := scanDelivery(s.DB.QueryRow(ctx, deliverySelect+
		` WHERE workspace_id=$1 AND delivery_task_id=$2`, workspaceID, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		s.reportFailure(ctx, workspaceID, actor, taskID, "get-task", ErrNotFound)
		return DeliveryTask{}, ErrNotFound
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, taskID, "get-task", err)
		return DeliveryTask{}, ErrStorage
	}
	return s.decorate(ctx, workspaceID, task)
}

// ListTasks returns the workspace's tasks, newest first, each decorated with
// the two derived flags.
func (s *Store) ListTasks(ctx context.Context, workspaceID, actor, artifactID, status string) ([]DeliveryTask, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-tasks", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-tasks", ErrInvalid)
		return nil, ErrInvalid
	}
	if status != "" {
		if err := ValidateDeliveryStatus(status); err != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-tasks", err)
			return nil, err
		}
	}
	rows, err := s.DB.Query(ctx, deliverySelect+
		` WHERE workspace_id=$1 AND ($2='' OR artifact_id=$2) AND ($3='' OR status=$3)
		  ORDER BY created_at DESC, delivery_task_id`, workspaceID, artifactID, status)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-tasks", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	tasks := []DeliveryTask{}
	for rows.Next() {
		task, scanErr := scanDelivery(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-tasks", scanErr)
			return nil, ErrStorage
		}
		tasks = append(tasks, task)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-tasks", rows.Err())
		return nil, ErrStorage
	}
	for i := range tasks {
		decorated, decorateErr := s.decorate(ctx, workspaceID, tasks[i])
		if decorateErr != nil {
			return nil, decorateErr
		}
		tasks[i] = decorated
	}
	return tasks, nil
}

// decorate fills the two derived flags. They are computed here and stored
// nowhere: a stored "due" would be wrong one minute after it was written, and a
// stored "pending registration" would be the system claiming to know the
// platform's state, which SOP 9.2 forbids in so many words.
func (s *Store) decorate(ctx context.Context, workspaceID string, task DeliveryTask) (DeliveryTask, error) {
	task.Due = IsDue(task.Status, task.ScheduledAt, s.now())
	if task.Status != DeliveryHandedOff {
		task.PendingRegistration = false
		return task, nil
	}
	var recorded int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM content_publication_record
		WHERE workspace_id=$1 AND artifact_id=$2`, workspaceID, task.ArtifactID).Scan(&recorded); err != nil {
		return DeliveryTask{}, ErrStorage
	}
	task.PendingRegistration = IsPendingRegistration(task.Status, recorded)
	return task, nil
}
