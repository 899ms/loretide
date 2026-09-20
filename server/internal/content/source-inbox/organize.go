package sourceinbox

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// BulkResult is what happened to one item in a bulk operation.
//
// Per item, not per batch: a batch that half worked has to say which half.
// Reporting "ok" for the whole call would make the failures invisible, and
// rolling the whole thing back would throw away work that succeeded.
type BulkResult struct {
	SourceID string `json:"source_id"`
	OK       bool   `json:"ok"`
	Reason   string `json:"reason,omitempty"`
}

// OrganizeSource changes the mutable half and records that it happened.
//
// The UPDATE names only the mutable columns. kind, url, captured_at,
// recorded_by and historical_import are facts about the act of collecting, and
// a guard test reads this file to check that no SET list mentions one.
func (s *Store) OrganizeSource(ctx context.Context, workspaceID, actor, sourceID string, patch Organize) (Source, error) {
	if s == nil || s.DB == nil {
		return Source{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || sourceID == "" {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "organize", ErrInvalid)
		return Source{}, ErrInvalid
	}
	if err := ValidateOrganize(patch); err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "organize", err)
		return Source{}, err
	}
	changed := ChangedFields(patch)
	if len(changed) == 0 {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "organize", ErrInvalid)
		return Source{}, ErrInvalid
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "organize", err)
		return Source{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	source, err := s.applyPatch(ctx, tx, workspaceID, sourceID, patch)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "organize", err)
		return Source{}, err
	}
	if err = s.appendRevision(ctx, tx, workspaceID, sourceID, actor, changed); err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "organize", err)
		return Source{}, ErrStorage
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, sourceID, "organize"); err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "organize", err)
		return Source{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "organize", err)
		return Source{}, ErrStorage
	}
	return source, nil
}

// applyPatch is the ONLY UPDATE of content_source in this package.
//
// COALESCE with a typed null is how "leave it" is expressed, so the statement
// is one shape rather than assembled from fragments: a builder would make the
// guard test's job - reading the SET list - a matter of guessing what the
// builder could emit.
func (s *Store) applyPatch(ctx context.Context, tx pgx.Tx, workspaceID, sourceID string, patch Organize) (Source, error) {
	row := tx.QueryRow(ctx, `UPDATE content_source SET
		title = COALESCE($3, title),
		tags = COALESCE($4, tags),
		annotation = COALESCE($5, annotation),
		personal_judgement = COALESCE($6, personal_judgement),
		status = COALESCE($7, status),
		updated_at = $8
		WHERE workspace_id = $1 AND source_id = $2
		RETURNING source_id, workspace_id, kind, url, captured_at, recorded_by,
		          historical_import, title, tags, annotation, personal_judgement,
		          status, updated_at`,
		workspaceID, sourceID, patch.Title, patch.Tags, patch.Annotation,
		patch.PersonalJudgement, statusArg(patch.Status), s.now())
	source, err := scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, ErrNotFound
	}
	if err != nil {
		return Source{}, ErrStorage
	}
	return source, nil
}

func statusArg(status *Status) *string {
	if status == nil {
		return nil
	}
	value := string(*status)
	return &value
}

// BulkOrganize applies the same patch to several items, one at a time.
//
// Each item gets its own revision row. A single "bulk" entry would make "when
// did this item get this tag" unanswerable, which is the question the log
// exists for.
//
// Items are processed in their own transactions rather than one: a bulk of
// thirty where the fourth is gone should leave twenty-nine organised and say
// which one was not, not undo the twenty-nine.
func (s *Store) BulkOrganize(ctx context.Context, workspaceID, actor string, sourceIDs []string, addTags []string, status *Status) ([]BulkResult, error) {
	if s == nil || s.DB == nil {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || len(sourceIDs) == 0 {
		s.reportFailure(ctx, workspaceID, actor, "", "bulk-organize", ErrInvalid)
		return nil, ErrInvalid
	}
	if len(addTags) == 0 && status == nil {
		s.reportFailure(ctx, workspaceID, actor, "", "bulk-organize", ErrInvalid)
		return nil, ErrInvalid
	}
	if status != nil {
		if err := ValidateStatus(string(*status)); err != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "bulk-organize", err)
			return nil, err
		}
	}
	if err := validateFields("", "", "", addTags); err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "bulk-organize", err)
		return nil, err
	}

	results := make([]BulkResult, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		result := BulkResult{SourceID: sourceID, OK: true}
		if err := s.bulkOne(ctx, workspaceID, actor, sourceID, addTags, status); err != nil {
			result.OK = false
			result.Reason = bulkReason(err)
		}
		results = append(results, result)
	}
	return results, nil
}

func bulkReason(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrInvalid):
		return "invalid"
	default:
		return "storage"
	}
}

func (s *Store) bulkOne(ctx context.Context, workspaceID, actor, sourceID string, addTags []string, status *Status) error {
	if sourceID == "" {
		return ErrInvalid
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	changed := []string{}
	if len(addTags) > 0 {
		changed = append(changed, "tags")
	}
	if status != nil {
		changed = append(changed, "status")
	}

	// Tags are unioned rather than replaced: "add a tag to these thirty" must
	// not silently drop the tags each of them already carries.
	row := tx.QueryRow(ctx, `UPDATE content_source SET
		tags = (SELECT COALESCE(array_agg(DISTINCT tag), '{}') FROM unnest(tags || $3::text[]) AS tag),
		status = COALESCE($4, status),
		updated_at = $5
		WHERE workspace_id = $1 AND source_id = $2
		RETURNING source_id`,
		workspaceID, sourceID, addTags, statusArg(status), s.now())
	var updated string
	if err = row.Scan(&updated); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return ErrStorage
	}
	if err = s.appendRevision(ctx, tx, workspaceID, sourceID, actor, changed); err != nil {
		return ErrStorage
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, sourceID, "bulk-organize"); err != nil {
		return ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrStorage
	}
	return nil
}
