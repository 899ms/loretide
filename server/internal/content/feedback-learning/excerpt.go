package feedbacklearning

import (
	"context"
	"time"
)

// Recording what people said, and what the operator makes of it (SOP 10.1,
// PRD R-045).
//
// Nothing here redacts anything. 10.1 says the product "支持脱敏摘录" - it
// supports a person redacting, it does not redact for them. An automatic
// redactor that misses one name is worse than none, because it teaches people
// to stop checking.

// Excerpt writes one excerpt.
//
// The quote and the operator's reading go into two columns and stay there.
// Folded into one paragraph, nobody can tell afterwards which sentence is the
// evidence and which is the judgement - and 10.2's review is exactly the act
// of checking the second against the first.
func (s *Store) Excerpt(ctx context.Context, workspaceID, actor string, input ExcerptInput) (FeedbackExcerpt, error) {
	if s == nil {
		return FeedbackExcerpt{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "record-excerpt", ErrInvalid)
		return FeedbackExcerpt{}, ErrInvalid
	}
	if err := ValidateExcerptInput(input); err != nil {
		s.reportFailure(ctx, workspaceID, actor, input.PublicationRecordID, "record-excerpt", err)
		return FeedbackExcerpt{}, err
	}
	if s.Publications == nil {
		s.reportFailure(ctx, workspaceID, actor, "", "record-excerpt", ErrStorage)
		return FeedbackExcerpt{}, ErrStorage
	}
	occurredAt, err := time.Parse(time.RFC3339, input.OccurredAt)
	if err != nil {
		return FeedbackExcerpt{}, FieldError{Field: "occurred_at", Reason: "not an RFC 3339 timestamp"}
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, input.PublicationRecordID, "record-excerpt", err)
		return FeedbackExcerpt{}, err
	}
	defer tx.Rollback(ctx)

	if _, _, _, resolveErr := s.Publications.Resolve(ctx, workspaceID, input.PublicationRecordID); resolveErr != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, input.PublicationRecordID, "record-excerpt", ErrNotFound)
		return FeedbackExcerpt{}, ErrNotFound
	}

	tags := input.Tags
	if tags == nil {
		tags = []string{}
	}
	excerpt := FeedbackExcerpt{
		FeedbackExcerptID:   s.newID(),
		WorkspaceID:         workspaceID,
		PublicationRecordID: input.PublicationRecordID,
		SourceType:          input.SourceType,
		RedactedExcerpt:     input.RedactedExcerpt,
		Interpretation:      input.Interpretation,
		Tags:                tags,
		OccurredAt:          occurredAt.UTC(),
		RecordedBy:          actor,
	}
	ctx, err = s.audit(ctx, tx, workspaceID, actor, excerpt.FeedbackExcerptID, "record-excerpt")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, excerpt.FeedbackExcerptID, "record-excerpt", err)
		return FeedbackExcerpt{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO content_feedback_excerpt
		(feedback_excerpt_id, workspace_id, publication_record_id, source_type,
		 redacted_excerpt, interpretation, tags, occurred_at, recorded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING created_at`,
		excerpt.FeedbackExcerptID, workspaceID, excerpt.PublicationRecordID,
		string(excerpt.SourceType), excerpt.RedactedExcerpt, excerpt.Interpretation,
		excerpt.Tags, excerpt.OccurredAt, actor).
		Scan(&excerpt.CreatedAt); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, excerpt.FeedbackExcerptID, "record-excerpt", err)
		return FeedbackExcerpt{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, excerpt.FeedbackExcerptID, "record-excerpt", err)
		return FeedbackExcerpt{}, ErrStorage
	}
	return excerpt, nil
}

// ListExcerpts returns the excerpts of one publication record, or of the whole
// workspace, newest occurrence first.
//
// Rows, in order. No counting by source, no tag tally, no sentiment - 10.2 is
// where reading them adds up to something.
func (s *Store) ListExcerpts(ctx context.Context, workspaceID, actor, publicationRecordID string) ([]FeedbackExcerpt, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-excerpts", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-excerpts", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, excerptSelect+
		` WHERE workspace_id=$1 AND ($2='' OR publication_record_id=$2)
		  ORDER BY occurred_at DESC, feedback_excerpt_id`,
		workspaceID, publicationRecordID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-excerpts", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	excerpts := []FeedbackExcerpt{}
	for rows.Next() {
		item, scanErr := scanExcerpt(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-excerpts", scanErr)
			return nil, ErrStorage
		}
		excerpts = append(excerpts, item)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-excerpts", rows.Err())
		return nil, ErrStorage
	}
	return excerpts, nil
}

// ReviewPlaceholder is the whole of this card's AI review.
//
// There is no report table and nothing that could write one, so the answer is
// always pending_data. It is a method rather than a constant so the page asks
// instead of assuming, and so the day EP-08 lands the call site already exists.
func (s *Store) ReviewPlaceholder(context.Context, string, string) ReviewState {
	return ReviewStateFor(0)
}
