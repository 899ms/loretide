package feedbacklearning

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// SearchThemes answers "is this search theme here". Implemented by
// handler/content_search_observations.go over topic-planning's Store:
// feedback-learning does not import topic-planning (FR-078).
//
// It answers nil, ErrNotFound for a theme that is not this workspace's -
// missing, another brand's, or gone with a deleted workspace - and
// ErrStorage for anything else. An archived theme is still here: an
// observation made while it was current stays tied to it.
type SearchThemes interface {
	ThemeExists(ctx context.Context, workspaceID, actor, themeID string) error
}

// SearchStore is the search half of this module (specs/036 PR 4). It shares
// Store's database, fence, audit sink and publication adapter, and adds the
// account check 034 already uses and the theme check above.
type SearchStore struct {
	*Store
	Accounts Accounts
	Themes   SearchThemes
	// Now is the server's clock for observed_at. nil is time.Now.
	Now func() time.Time
	// BeforeRevisionInsert is a test seam and nil in production. It runs
	// right before an observation revision is inserted, which is the window
	// in which two writers can both believe they hold the latest revision.
	BeforeRevisionInsert func(ctx context.Context, observationID string)
}

func (s *SearchStore) ready() bool {
	return s != nil && s.Store != nil && s.DB != nil
}

func (s *SearchStore) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// inTx runs one write path in the order every write here keeps: the
// workspace delete fence first (begin), then body - references, the
// revision check, one INSERT - then the audit event, in the same
// transaction. body returns the object id for the audit event.
func (s *SearchStore) inTx(ctx context.Context, workspaceID, actor string, step func() string,
	body func(ctx context.Context, tx pgx.Tx) (string, error)) error {
	if !s.ready() {
		return ErrStorage
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", step(), err)
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	objectID, err := body(ctx, tx)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, objectID, step(), err)
		return err
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, objectID, step()); err != nil {
		s.reportFailure(ctx, workspaceID, actor, objectID, step(), err)
		return ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, objectID, step(), err)
		return ErrStorage
	}
	return nil
}

// checkSearchPublication answers whether a publication record is this
// workspace's, through the same adapter 027 uses.
func (s *SearchStore) checkSearchPublication(ctx context.Context, workspaceID, publicationRecordID string) error {
	if publicationRecordID == "" {
		return nil
	}
	if s.Publications == nil {
		return ErrStorage
	}
	_, _, _, err := s.Publications.Resolve(ctx, workspaceID, publicationRecordID)
	return adapterError(err)
}

func (s *SearchStore) checkSearchAccount(ctx context.Context, workspaceID, accountID string) error {
	if accountID == "" {
		return nil
	}
	if s.Accounts == nil {
		return ErrStorage
	}
	return adapterError(s.Accounts.AccountExists(ctx, workspaceID, accountID))
}

func (s *SearchStore) checkSearchTheme(ctx context.Context, workspaceID, actor, themeID string) error {
	if themeID == "" {
		return nil
	}
	if s.Themes == nil {
		return ErrStorage
	}
	return adapterError(s.Themes.ThemeExists(ctx, workspaceID, actor, themeID))
}

// referenceField turns "that reference is not here" into a 400 naming the
// field. A foreign id and a missing one answer the same. Anything else -
// ErrStorage above all - is returned as it is.
func referenceField(field string, err error) error {
	if errors.Is(err, ErrNotFound) {
		return FieldError{Field: field, Reason: "not found"}
	}
	return err
}
