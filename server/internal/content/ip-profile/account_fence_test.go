package ipprofile

import (
	"context"
	"errors"
	"testing"
)

// memoryAccounts is an account store bound to a fence. Every method records
// whether it was called from inside the fenced callback, which is the property
// under test: a read that decides what gets written, and the write itself, must
// be in the same transaction as the lock on the workspace row (Issue #104).
type memoryAccounts struct {
	accounts map[string]Account
	inside   bool
	calls    []string
}

func (m *memoryAccounts) record(name string) {
	m.calls = append(m.calls, name)
	if !m.inside {
		m.calls = append(m.calls, name+"-outside-the-fence")
	}
}

func (m *memoryAccounts) CreateAccount(_ context.Context, account Account) (Account, error) {
	m.record("create")
	m.accounts[account.AccountID] = account
	return account, nil
}

func (m *memoryAccounts) GetAccount(_ context.Context, workspaceID, accountID string) (Account, error) {
	m.record("get")
	account, ok := m.accounts[accountID]
	if !ok || account.WorkspaceID != workspaceID {
		return Account{}, ErrNotFound
	}
	return account, nil
}

func (m *memoryAccounts) UpdateAccount(_ context.Context, workspaceID, accountID string, patch Patch) (Account, error) {
	m.record("update")
	account, ok := m.accounts[accountID]
	if !ok || account.WorkspaceID != workspaceID {
		return Account{}, ErrNotFound
	}
	if patch.Platform != nil {
		account.Platform = *patch.Platform
	}
	if patch.DisplayName != nil {
		account.DisplayName = *patch.DisplayName
	}
	if patch.Settings != nil {
		account.Settings = patch.Settings
	}
	m.accounts[accountID] = account
	return account, nil
}

func (m *memoryAccounts) ListAccounts(context.Context, string) ([]Account, error) {
	m.record("list")
	return nil, nil
}

// countingAccountFence stands in for the handler's transaction. It marks the
// window the statements must fall inside and counts how many transactions the
// service opened.
type countingAccountFence struct {
	accounts *memoryAccounts
	entered  int
}

func (f *countingAccountFence) WithWorkspaceFence(context.Context, string, func(RevisionTx) error) error {
	return errors.New("countingAccountFence has no revision storage")
}

func (f *countingAccountFence) WithAccountFence(_ context.Context, _ string, fn func(AccountTx) error) error {
	f.entered++
	f.accounts.inside = true
	defer func() { f.accounts.inside = false }()
	return fn(f.accounts)
}

func newFencedAccountService() (*Service, *countingAccountFence, *memoryAccounts) {
	accounts := &memoryAccounts{accounts: map[string]Account{}}
	fence := &countingAccountFence{accounts: accounts}
	return &Service{Store: accounts, Fence: fence, NewID: func() string { return "account-1" }}, fence, accounts
}

func TestAccountWritesRunInsideTheWorkspaceFence(t *testing.T) {
	service, fence, accounts := newFencedAccountService()
	ctx := context.Background()

	if _, err := service.Create(ctx, "ws", "actor", "zhihu", "品牌号", nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := service.Update(ctx, "ws", "actor", "account-1",
		Patch{DisplayName: stringPtr("改名")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := service.SetScope(ctx, "ws", "actor", "account-1", string(ScopeWeb)); err != nil {
		t.Fatalf("set scope: %v", err)
	}

	if fence.entered != 3 {
		t.Errorf("the three writes opened %d fenced transactions, want 3", fence.entered)
	}
	for _, call := range accounts.calls {
		if len(call) > len("-outside-the-fence") &&
			call[len(call)-len("-outside-the-fence"):] == "-outside-the-fence" {
			t.Errorf("%s ran outside the fence; calls were %v", call, accounts.calls)
		}
	}
}

// SetScope reads the settings it merges into. That read decides what the update
// writes, so it belongs in the same transaction: a read taken outside it could
// be answered before a delete commits and the update applied after.
func TestSetScopeReadsAndWritesInOneFencedTransaction(t *testing.T) {
	service, fence, accounts := newFencedAccountService()
	ctx := context.Background()
	if _, err := service.Create(ctx, "ws", "actor", "zhihu", "品牌号",
		map[string]any{"keep": "me"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	fence.entered = 0
	accounts.calls = nil

	account, err := service.SetScope(ctx, "ws", "actor", "account-1", string(ScopeAll))
	if err != nil {
		t.Fatalf("set scope: %v", err)
	}
	if fence.entered != 1 {
		t.Fatalf("SetScope opened %d transactions, want 1", fence.entered)
	}
	if len(accounts.calls) != 2 || accounts.calls[0] != "get" || accounts.calls[1] != "update" {
		t.Fatalf("SetScope made %v, want one get then one update inside the fence", accounts.calls)
	}
	if account.Settings["keep"] != "me" {
		t.Errorf("SetScope dropped the other settings: %v", account.Settings)
	}
	if account.Settings[ScopeSettingsKey] != string(ScopeAll) {
		t.Errorf("scope = %v, want %s", account.Settings[ScopeSettingsKey], ScopeAll)
	}
}

func stringPtr(value string) *string { return &value }
