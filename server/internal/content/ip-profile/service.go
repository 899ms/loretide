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
//
// This is the read half only. Creating, patching and re-scoping an account all
// go through AccountTx instead, inside the workspace fence, so there is no
// unfenced way to write an account left to reach for (Issue #104).
type Store interface {
	GetAccount(ctx context.Context, workspaceID, accountID string) (Account, error)
	ListAccounts(ctx context.Context, workspaceID string) ([]Account, error)
}

// AccountTx is the write half, bound to one transaction that already holds the
// workspace fence. GetAccount is here as well because SetScope has to read the
// settings it merges into, and a read that decides what gets written belongs
// inside the same transaction as the write: between an outside read and the
// insert, DeleteWorkspace could commit.
type AccountTx interface {
	CreateAccount(ctx context.Context, account Account) (Account, error)
	GetAccount(ctx context.Context, workspaceID, accountID string) (Account, error)
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
	// Fence is the workspace delete/write protocol every revision write holds.
	// Required: a Service without it refuses to write rather than writing
	// outside the fence.
	Fence WorkspaceFence
	Audit Auditor
	Build string
	NewID func() string
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
	if err := validateScopeSetting(settings); err != nil {
		return Account{}, err
	}
	var account Account
	err := s.withAccountFence(ctx, workspaceID, func(tx AccountTx) error {
		created, createErr := tx.CreateAccount(ctx, Account{
			AccountID:   s.newID(),
			WorkspaceID: workspaceID,
			Platform:    platform,
			DisplayName: displayName,
			Settings:    settings,
		})
		account = created
		return createErr
	})
	if err != nil {
		return Account{}, err
	}
	s.audit(ctx, workspaceID, actor, account.AccountID, "create")
	return account, nil
}

// Get fills the scope preference on the way out. The stored row is left alone:
// an account that never chose still has no scope key after this call, and only
// the response is complete (LT-014, contracts/account-scope.md).
func (s *Service) Get(ctx context.Context, workspaceID, accountID string) (Account, error) {
	account, err := s.Store.GetAccount(ctx, workspaceID, accountID)
	if err != nil {
		return Account{}, err
	}
	account.Settings = scopeFilled(account.Settings)
	return account, nil
}

func (s *Service) List(ctx context.Context, workspaceID string) ([]Account, error) {
	accounts, err := s.Store.ListAccounts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		accounts[i].Settings = scopeFilled(accounts[i].Settings)
	}
	return accounts, nil
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
	if patch.Settings != nil {
		// The dedicated scope endpoint is not the only way into this key. With
		// only that path checked, a PATCH could store a value every reader
		// refuses, and the endpoint's validation would be decoration.
		if err := validateScopeSetting(patch.Settings); err != nil {
			return Account{}, err
		}
	}
	var account Account
	err := s.withAccountFence(ctx, workspaceID, func(tx AccountTx) error {
		updated, updateErr := tx.UpdateAccount(ctx, workspaceID, accountID, patch)
		account = updated
		return updateErr
	})
	if err != nil {
		return Account{}, err
	}
	s.audit(ctx, workspaceID, actor, accountID, "update")
	account.Settings = scopeFilled(account.Settings)
	return account, nil
}

// SetScope records the account's material scope preference.
//
// Read-modify-write rather than a partial patch: the update query replaces the
// settings blob wholesale, so sending this key alone would delete every other
// setting the account has. Doing the merge here means no caller - and no future
// caller - has to know that.
//
// An unrecognised value is refused before anything is read, so a rejected edit
// leaves storage exactly as it was.
func (s *Service) SetScope(ctx context.Context, workspaceID, actor, accountID, scope string) (Account, error) {
	if err := ValidateScope(scope); err != nil {
		return Account{}, err
	}
	var account Account
	err := s.withAccountFence(ctx, workspaceID, func(tx AccountTx) error {
		current, readErr := tx.GetAccount(ctx, workspaceID, accountID)
		if readErr != nil {
			return readErr
		}
		updated, updateErr := tx.UpdateAccount(ctx, workspaceID, accountID,
			Patch{Settings: withScope(current.Settings, scope)})
		account = updated
		return updateErr
	})
	if err != nil {
		return Account{}, err
	}
	// Audited even when the value did not change: "who confirmed this choice,
	// and when" is itself the thing an operator goes looking for later.
	s.audit(ctx, workspaceID, actor, accountID, "update")
	account.Settings = scopeFilled(account.Settings)
	return account, nil
}

// withAccountFence runs an account write inside the workspace delete/write
// protocol. A Service without a fence refuses rather than writing outside it:
// content_account has no foreign key, so an unfenced write is how an account
// outlives the brand it belongs to.
func (s *Service) withAccountFence(ctx context.Context, workspaceID string, fn func(AccountTx) error) error {
	if s.Fence == nil {
		return ErrNoWorkspaceFence
	}
	return s.Fence.WithAccountFence(ctx, workspaceID, fn)
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
