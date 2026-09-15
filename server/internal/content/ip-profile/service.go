package ipprofile

import (
	"context"
	"errors"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// ErrNotFound is returned when no account matches the id WITHIN the caller's
// workspace. It deliberately does not distinguish "belongs to someone else"
// from "does not exist": the HTTP layer answers both with the same 404, and a
// service that told them apart would make that impossible to honour.
var ErrNotFound = errors.New("account not found")

// Store is the persistence this module needs, expressed in its own terms.
//
// The generated db package cannot be imported here - the content boundary
// checker only approves pgx, uuid, websocket and otel as upstream imports - so
// the handler adapts sqlc to this interface. That constraint happens to be the
// right design anyway: the module stays testable without a database.
//
// Every method takes workspaceID, including the ones that already have the
// account id. Isolation belongs to the query, not to the caller's memory.
type Store interface {
	CreateAccount(ctx context.Context, account Account) (Account, error)
	GetAccount(ctx context.Context, workspaceID, accountID string) (Account, error)
	ListAccounts(ctx context.Context, workspaceID string) ([]Account, error)
	UpdateAccount(ctx context.Context, workspaceID, accountID string, patch Patch) (Account, error)
}

// Auditor records what happened to an account. Creating and updating an account
// changes what a brand publishes under, so both are audited; this is the
// module's real diagnostics integration, not a call added to satisfy a check.
type Auditor interface {
	Audit(ctx context.Context, event diagnostics.Event) error
}

// Patch is a partial update. A nil field means "leave it alone" - distinct from
// a pointer to the empty string, which would mean "set it to empty" and is
// refused for the display name.
type Patch struct {
	Platform    *string
	DisplayName *string
	Settings    map[string]any
}

// Service is the module's entry point. Authorization happens before this: the
// HTTP layer decides with content/workspace-core and passes an already-approved
// workspace id. Keeping that out of here means there is exactly one place where
// the decision is made, which is the whole point of LT-010.
type Service struct {
	Store Store
	// RevisionStore is the append-only persona revision history (LT-012). Its
	// own interface because it has no update and no delete; folding it into
	// Store would suggest otherwise.
	RevisionStore RevisionStore
	Audit         Auditor
	Build         string
	NewID         func() string
}

func (s *Service) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return diagnostics.NewID()
}

// Create validates before writing. The platform and the display name are
// checked here rather than relied on from the CHECK constraint, because a
// constraint violation surfaces as a database error, not as the 400 diagnostic
// error object the contract promises.
func (s *Service) Create(ctx context.Context, workspaceID, actor, platform, displayName string, settings map[string]any) (Account, error) {
	if err := ValidatePlatform(platform); err != nil {
		return Account{}, err
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return Account{}, err
	}
	if settings == nil {
		settings = map[string]any{}
	}
	account, err := s.Store.CreateAccount(ctx, Account{
		AccountID:   s.newID(),
		WorkspaceID: workspaceID,
		Platform:    platform,
		DisplayName: displayName,
		Settings:    settings,
	})
	if err != nil {
		return Account{}, err
	}
	s.audit(ctx, workspaceID, actor, account.AccountID, "create")
	return account, nil
}

func (s *Service) Get(ctx context.Context, workspaceID, accountID string) (Account, error) {
	return s.Store.GetAccount(ctx, workspaceID, accountID)
}

func (s *Service) List(ctx context.Context, workspaceID string) ([]Account, error) {
	return s.Store.ListAccounts(ctx, workspaceID)
}

// Update validates only the fields that were sent. A patch that omits the
// platform must not be rejected for not carrying one.
func (s *Service) Update(ctx context.Context, workspaceID, actor, accountID string, patch Patch) (Account, error) {
	if patch.Platform != nil {
		if err := ValidatePlatform(*patch.Platform); err != nil {
			return Account{}, err
		}
	}
	if patch.DisplayName != nil {
		if err := ValidateDisplayName(*patch.DisplayName); err != nil {
			return Account{}, err
		}
	}
	account, err := s.Store.UpdateAccount(ctx, workspaceID, accountID, patch)
	if err != nil {
		return Account{}, err
	}
	s.audit(ctx, workspaceID, actor, accountID, "update")
	return account, nil
}

// audit records a change to an account. The event names the workspace and the
// account, which is exactly the pair the diagnostics tables have been carrying
// columns for since migration 468.
func (s *Service) audit(ctx context.Context, workspaceID, actor, accountID, action string) {
	if s.Audit == nil {
		return
	}
	_ = s.Audit.Audit(ctx, diagnostics.Event{
		ID:         diagnostics.NewID(),
		Workspace:  workspaceID,
		Account:    accountID,
		Actor:      actor,
		ActorKind:  "human",
		ObjectType: "account",
		ObjectID:   accountID,
		Action:     action,
		Outcome:    "success",
		Component:  "ip-profile",
		Severity:   "info",
		Build:      s.Build,
	})
}
