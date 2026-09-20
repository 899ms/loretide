package reviewdelivery

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Submitting a version for review, and disposing of it (SOP 8).

// SubmitRequest is what the caller chose for this submission.
type SubmitRequest struct {
	ArtifactID string  `json:"artifact_id"`
	VersionID  string  `json:"version_id"`
	AccountID  string  `json:"account_id"`
	Channel    Channel `json:"channel"`
	// StartSnapshotID is optional context: the start this work came from, when
	// there was one. "" is a real state.
	StartSnapshotID string `json:"start_snapshot_id"`
}

func decodeSnapshot(payload []byte, target *DeliverySnapshot) error {
	if len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, target)
}

// Submit freezes one version for review.
//
// The same document may be submitted many times: each submission is its own
// request, and a second one is NOT a conflict. SOP 8 requires exactly this -
// "新的交付组合需要重新审核" - so this path must never grow a guard that makes
// it look idempotent.
//
// Only channel drafts may be submitted. SOP 8's object of review is "具体渠道、
// 具体文档版本及附件、具体交付配置的快照": without a channel there is nothing to
// review against. The kind is checked INSIDE the fence, because a check made
// outside the transaction reads a value that may already have changed.
func (s *Store) Submit(ctx context.Context, workspaceID, actor string, request SubmitRequest) (ReviewRequest, error) {
	if s == nil {
		return ReviewRequest{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || request.ArtifactID == "" ||
		request.VersionID == "" || request.AccountID == "" {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "submit-review", ErrInvalid)
		return ReviewRequest{}, invalidField("version_id")
	}
	if err := ValidateChannel(string(request.Channel)); err != nil {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "submit-review", err)
		return ReviewRequest{}, err
	}
	if s.Artifacts == nil {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "submit-review", ErrStorage)
		return ReviewRequest{}, ErrStorage
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "submit-review", err)
		return ReviewRequest{}, err
	}
	defer tx.Rollback(ctx)

	// Inside the fence: the document has to still exist, still be in this
	// workspace, and still be a channel draft at the moment the row is written.
	workID, kind, err := s.Artifacts.ResolveVersion(ctx, workspaceID, request.ArtifactID, request.VersionID)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "submit-review", ErrNotFound)
		return ReviewRequest{}, ErrNotFound
	}
	if kind != channelDraftKind {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "submit-review", ErrInvalid)
		// Named, not hidden: the document is the caller's own, so saying which
		// field is wrong reveals nothing about anyone else.
		return ReviewRequest{}, invalidField("kind")
	}

	created := ReviewRequest{
		ReviewRequestID: s.newID(),
		WorkspaceID:     workspaceID,
		WorkID:          workID,
		ArtifactID:      request.ArtifactID,
		VersionID:       request.VersionID,
		AccountID:       request.AccountID,
		Channel:         request.Channel,
		Snapshot: NewDeliverySnapshot(request.Channel, workID, request.ArtifactID,
			request.VersionID, request.AccountID, request.StartSnapshotID),
		Status:      InitialReviewStatus,
		RequestedBy: actor,
	}
	payload, err := json.Marshal(created.Snapshot)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.ReviewRequestID, "submit-review", err)
		return ReviewRequest{}, ErrStorage
	}
	ctx, err = s.audit(ctx, tx, workspaceID, actor, created.ReviewRequestID, "submit-review")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.ReviewRequestID, "submit-review", err)
		return ReviewRequest{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO content_review_request
		(review_request_id, workspace_id, work_id, artifact_id, version_id,
		 account_id, channel, snapshot, status, requested_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING requested_at, created_at, updated_at`,
		created.ReviewRequestID, workspaceID, workID, request.ArtifactID,
		request.VersionID, request.AccountID, string(request.Channel), payload,
		string(InitialReviewStatus), actor).
		Scan(&created.RequestedAt, &created.CreatedAt, &created.UpdatedAt); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.ReviewRequestID, "submit-review", err)
		return ReviewRequest{}, ErrStorage
	}
	if err = s.recordTransition(ctx, tx, workspaceID, actor, SubjectReviewRequest,
		created.ReviewRequestID, "", string(InitialReviewStatus), ""); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.ReviewRequestID, "submit-review", err)
		return ReviewRequest{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, created.ReviewRequestID, "submit-review", err)
		return ReviewRequest{}, ErrStorage
	}
	return created, nil
}

// channelDraftKind is work-editor's value for a channel cut. Compared as a
// string rather than imported: review-delivery's declared dependency on
// work-editor is a storage-level one, and the adapter is what reads the column.
// A test compares this constant against work-editor's Go source.
const channelDraftKind = "channel_draft"

// Decide records a disposition.
//
// The decision is only ever made by a person. SOP 8: "AI 的自检报告作为审核参考，
// 不能执行人工通过动作". Nothing in this package calls a model, and a guard test
// keeps that true; the executor being disabled (constitution IX) is the other
// half.
//
// The snapshot column is not named here. An approval belongs to the combination
// it was granted for, and SOP 8 is explicit that "系统不得把'已通过'自动套在新版本
// 上" - which holds structurally, because approved is written on a row bound to
// one version_id and there is nowhere else to read it from.
func (s *Store) Decide(ctx context.Context, workspaceID, actor, reviewID string, to ReviewStatus, note string) (ReviewRequest, error) {
	if s == nil {
		return ReviewRequest{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || reviewID == "" {
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", ErrInvalid)
		return ReviewRequest{}, ErrInvalid
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", err)
		return ReviewRequest{}, err
	}
	defer tx.Rollback(ctx)

	// Locked before the transition is judged: two people disposing of the same
	// request at once must not both see pending.
	var current ReviewStatus
	err = tx.QueryRow(ctx, `SELECT status FROM content_review_request
		WHERE workspace_id=$1 AND review_request_id=$2 FOR UPDATE`,
		workspaceID, reviewID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", ErrNotFound)
		return ReviewRequest{}, ErrNotFound
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", err)
		return ReviewRequest{}, ErrStorage
	}
	if err = ValidateReviewDecision(current, to, note); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", err)
		return ReviewRequest{}, err
	}
	ctx, err = s.audit(ctx, tx, workspaceID, actor, reviewID, "decide-review")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", err)
		return ReviewRequest{}, err
	}
	request, err := scanReview(tx.QueryRow(ctx, `UPDATE content_review_request
		SET status=$3, decided_by=$4, decided_at=now(), decision_note=$5, updated_at=now()
		WHERE workspace_id=$1 AND review_request_id=$2
		RETURNING review_request_id, workspace_id, work_id, artifact_id, version_id,
			account_id, channel, snapshot, status, requested_by, requested_at,
			decided_by, decided_at, decision_note, created_at, updated_at`,
		workspaceID, reviewID, string(to), actor, note))
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", err)
		return ReviewRequest{}, ErrStorage
	}
	if err = s.recordTransition(ctx, tx, workspaceID, actor, SubjectReviewRequest,
		reviewID, string(current), string(to), note); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", err)
		return ReviewRequest{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, reviewID, "decide-review", err)
		return ReviewRequest{}, ErrStorage
	}
	return request, nil
}

// ListReviews returns the workspace's requests, optionally narrowed to one
// document or one status, newest first.
func (s *Store) ListReviews(ctx context.Context, workspaceID, actor, artifactID, status string) ([]ReviewRequest, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-reviews", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-reviews", ErrInvalid)
		return nil, ErrInvalid
	}
	if status != "" {
		if err := ValidateReviewStatus(status); err != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-reviews", err)
			return nil, err
		}
	}
	rows, err := s.DB.Query(ctx, reviewSelect+
		` WHERE workspace_id=$1 AND ($2='' OR artifact_id=$2) AND ($3='' OR status=$3)
		  ORDER BY created_at DESC, review_request_id`, workspaceID, artifactID, status)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-reviews", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	requests := []ReviewRequest{}
	for rows.Next() {
		request, scanErr := scanReview(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-reviews", scanErr)
			return nil, ErrStorage
		}
		requests = append(requests, request)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-reviews", rows.Err())
		return nil, ErrStorage
	}
	return requests, nil
}

func (s *Store) GetReview(ctx context.Context, workspaceID, actor, reviewID string) (ReviewRequest, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, reviewID, "get-review", ErrStorage)
		}
		return ReviewRequest{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || reviewID == "" {
		s.reportFailure(ctx, workspaceID, actor, reviewID, "get-review", ErrInvalid)
		return ReviewRequest{}, ErrInvalid
	}
	request, err := scanReview(s.DB.QueryRow(ctx, reviewSelect+
		` WHERE workspace_id=$1 AND review_request_id=$2`, workspaceID, reviewID))
	if errors.Is(err, pgx.ErrNoRows) {
		s.reportFailure(ctx, workspaceID, actor, reviewID, "get-review", ErrNotFound)
		return ReviewRequest{}, ErrNotFound
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, reviewID, "get-review", err)
		return ReviewRequest{}, ErrStorage
	}
	return request, nil
}

// LatestReviewStatusFor reports how the given version stands.
//
// It answers per VERSION, not per document, which is what makes SOP 8's "系统不
// 得把'已通过'自动套在新版本上" observable: after a version is approved, the next
// version reads back as having no request at all.
func (s *Store) LatestReviewStatusFor(ctx context.Context, workspaceID, actor, artifactID, versionID string) (ReviewStatus, bool, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "latest-review", ErrStorage)
		}
		return "", false, ErrStorage
	}
	if workspaceID == "" || actor == "" || artifactID == "" || versionID == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "latest-review", ErrInvalid)
		return "", false, ErrInvalid
	}
	var status ReviewStatus
	err := s.DB.QueryRow(ctx, `SELECT status FROM content_review_request
		WHERE workspace_id=$1 AND artifact_id=$2 AND version_id=$3
		ORDER BY created_at DESC, review_request_id LIMIT 1`,
		workspaceID, artifactID, versionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "latest-review", err)
		return "", false, ErrStorage
	}
	return status, true, nil
}
