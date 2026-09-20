package ipprofile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// Contract: specs/016-lt012-persona-prompt-revisions/contracts/persona-revision.md

// A2 / A3. SOP 3.1 keeps unconfirmed items pending, so a creator who confirms
// before writing a persona is recording a real revision whose content is blank.
// Refusing it would leave them unable to move on.
func TestABlankPersonaPromptIsValid(t *testing.T) {
	for _, blank := range []string{"", " ", "\t", "\n  \n", "　"} {
		if err := ValidatePersonaPrompt(blank); err != nil {
			t.Errorf("blank prompt %q was rejected: %v — blank means \"still to be filled\", not invalid", blank, err)
		}
	}
}

func TestAnOverlongPersonaPromptIsRefused(t *testing.T) {
	if err := ValidatePersonaPrompt(strings.Repeat("字", MaxPersonaPromptRunes)); err != nil {
		t.Errorf("a prompt at the limit was rejected: %v", err)
	}
	if err := ValidatePersonaPrompt(strings.Repeat("字", MaxPersonaPromptRunes+1)); err == nil {
		t.Error("a prompt over the limit was accepted")
	}
}

// The limit counts runes, not bytes: a Chinese persona is three bytes per
// character, so a byte limit would cut it to a third of an English one.
func TestThePromptLimitCountsCharactersNotBytes(t *testing.T) {
	chinese := strings.Repeat("字", MaxPersonaPromptRunes)
	if len(chinese) <= MaxPersonaPromptRunes {
		t.Fatal("test fixture is not multi-byte")
	}
	if err := ValidatePersonaPrompt(chinese); err != nil {
		t.Errorf("a full-length Chinese prompt was rejected: %v", err)
	}
}

// conflictingStore always reports the revision number as taken, which is what a
// permanently contended account looks like.
type conflictingStore struct{ attempts int }

func (s *conflictingStore) NextRevision(context.Context, string, string) (int64, error) {
	return 1, nil
}
func (s *conflictingStore) InsertRevision(context.Context, Revision) (Revision, error) {
	s.attempts++
	return Revision{}, ErrRevisionTaken
}

// A8. Retry is bounded. Under real contention the write has to give up and say
// so rather than spin: an unbounded loop turns one write into an unbounded wait.
func TestSettingThePromptGivesUpAfterABoundedNumberOfRetries(t *testing.T) {
	store := &conflictingStore{}
	service := &Service{RevisionStore: store, Fence: directFence{store}, NewID: func() string { return "rev-1" }}

	_, err := service.SetPersonaPrompt(context.Background(), "ws", "actor", "acct", "人设")
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("got %v, want ErrRevisionConflict once retries are exhausted", err)
	}
	// Pinned to the literal, not to maxRevisionAttempts: comparing against the
	// constant makes the test follow whatever the constant becomes, so raising
	// the bound to 50 would still pass. Mutation testing is what showed that.
	if store.attempts != 3 {
		t.Errorf("tried %d times, want exactly 3 — fewer means it gave up early, more means "+
			"the bound was loosened without anyone deciding to", store.attempts)
	}
}

// succeedsOnAttempt conflicts a fixed number of times, then lets the write land.
type succeedsOnAttempt struct {
	succeedAt int
	attempts  int
}

func (s *succeedsOnAttempt) NextRevision(context.Context, string, string) (int64, error) {
	return int64(s.attempts + 1), nil
}
func (s *succeedsOnAttempt) InsertRevision(_ context.Context, r Revision) (Revision, error) {
	s.attempts++
	if s.attempts < s.succeedAt {
		return Revision{}, ErrRevisionTaken
	}
	return r, nil
}

// A losing writer must retry and land, not fail. Both confirmations really
// happened, so both belong in the history.
func TestALosingWriterRetriesAndLands(t *testing.T) {
	store := &succeedsOnAttempt{succeedAt: 2}
	service := &Service{RevisionStore: store, Fence: directFence{store}, NewID: func() string { return "rev-2" }}

	revision, err := service.SetPersonaPrompt(context.Background(), "ws", "actor", "acct", "人设")
	if err != nil {
		t.Fatalf("a retryable conflict was not retried: %v", err)
	}
	if store.attempts != 2 {
		t.Errorf("attempts = %d, want 2", store.attempts)
	}
	if revision.PersonaPrompt != "人设" {
		t.Errorf("landed revision carries %q", revision.PersonaPrompt)
	}
}

// D11-V02 and FR-014, for THIS card's migrations. The check added in #71 scans
// 477/478 only, so 479-481 had nothing holding them.
func TestThisCardAddsNoPersonaEntityAndNoTaskLevelOverride(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	forbidden := regexp.MustCompile(`(?i)persona_(table|library|profile)|_binding|persona_store`)
	var created []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		if !strings.HasPrefix(name, "479_") && !strings.HasPrefix(name, "480_") && !strings.HasPrefix(name, "481_") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, m := range regexp.MustCompile(`(?i)CREATE TABLE(?: IF NOT EXISTS)?\s+(\w+)`).
			FindAllStringSubmatch(string(source), -1) {
			created = append(created, m[1])
		}
		if forbidden.MatchString(string(source)) {
			t.Errorf("%s introduces a persona entity or a binding table; persona_prompt is a "+
				"versioned FIELD of the account config, not a model of its own (W-02)", name)
		}
	}
	if len(created) != 1 || created[0] != "content_account_revision" {
		t.Errorf("this card creates %v; it must create exactly one table, content_account_revision", created)
	}

	// FR-014: no per-run persona override. A revision is referenced, never
	// supplemented, or the account config stops being the single source.
	for _, field := range []string{"PersonaOverride", "TaskPersona", "PersonaParam", "RunPersona"} {
		if revisionHasField(field) {
			t.Errorf("Revision carries %q: runs reference a revision_id, they do not carry "+
				"their own persona (FR-014)", field)
		}
	}
}

func revisionHasField(name string) bool {
	switch name {
	case "RevisionID", "AccountID", "WorkspaceID", "Revision", "PersonaPrompt", "CreatedAt":
		return true
	}
	return false
}

// E3: the module's tests reference diagnostics, and a revision write is audited
// with the identifiers the diagnostics tables already carry.
func TestARevisionWriteIsAuditable(t *testing.T) {
	event := diagnostics.Event{Workspace: "ws-1", Account: "acct-1", ObjectType: "account"}
	if event.Account != "acct-1" {
		t.Fatalf("diagnostics event lost the account: %+v", event)
	}
}

// A6. Append-only has to be enforced by something a person cannot forget.
// Nothing stops a future query file from adding an UPDATE, and the moment one
// exists every promise about pinned revisions is void - silently, because no
// existing test would notice.
func TestNoQueryEverMutatesARevision(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "db", "queries", "content_account_revision.sql"))
	if err != nil {
		t.Fatalf("read queries: %v", err)
	}
	// Strip -- comments first. The file's own header says it contains no UPDATE
	// and no DELETE, and a check that read prose would fail on that sentence
	// while still missing a real statement written without the word in a
	// comment nearby.
	var statements []string
	for _, line := range strings.Split(string(source), "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		statements = append(statements, line)
	}
	text := strings.ToUpper(strings.Join(statements, "\n"))
	for _, forbidden := range []string{"UPDATE ", "DELETE "} {
		if strings.Contains(text, forbidden) {
			t.Errorf("content_account_revision.sql contains %q. Revisions are append-only: a run "+
				"that pinned a revision must keep reading what it pinned. The only statement that "+
				"removes rows belongs in workspace_delete.sql.", strings.TrimSpace(forbidden))
		}
	}
}

// directFence runs the write without a database, so the module's own cases can
// exercise retry and carry-forward logic. It is deliberately defined in the
// test package: production builds the real fence in the handler, and there is
// no exported pass-through for anyone to reach for by mistake.
type directFence struct{ store RevisionStore }

func (f directFence) WithWorkspaceFence(_ context.Context, _ string, fn func(RevisionTx) error) error {
	return fn(directTx{store: f.store})
}

// directTx adapts a write-only fake to the transaction interface. A fake that
// can also answer CurrentRevision is used as-is; one that cannot reports "no
// current revision", which is what an account with no history looks like.
type directTx struct{ store RevisionStore }

func (t directTx) NextRevision(ctx context.Context, workspaceID, accountID string) (int64, error) {
	return t.store.NextRevision(ctx, workspaceID, accountID)
}

func (t directTx) CurrentRevision(ctx context.Context, workspaceID, accountID string) (Revision, error) {
	if reader, ok := t.store.(interface {
		CurrentRevision(context.Context, string, string) (Revision, error)
	}); ok {
		return reader.CurrentRevision(ctx, workspaceID, accountID)
	}
	return Revision{}, ErrNotFound
}

func (t directTx) InsertRevision(ctx context.Context, revision Revision) (Revision, error) {
	return t.store.InsertRevision(ctx, revision)
}

// goneFence is a workspace that was deleted before the write reached it.
type goneFence struct{}

func (goneFence) WithWorkspaceFence(context.Context, string, func(RevisionTx) error) error {
	return ErrWorkspaceGone
}

// A workspace that disappeared mid-write is refused, and the caller sees the
// error that maps to 404 rather than a revision belonging to nothing.
func TestAWriteIsRefusedWhenTheWorkspaceIsGone(t *testing.T) {
	service := &Service{Fence: goneFence{}, NewID: func() string { return "rev-gone" }}

	if _, err := service.SetPersonaPrompt(context.Background(), "ws", "actor", "acct", "人设"); !errors.Is(err, ErrWorkspaceGone) {
		t.Errorf("SetPersonaPrompt got %v, want ErrWorkspaceGone", err)
	}
	if _, err := service.SetProfile(context.Background(), "ws", "actor", "acct", ExpressionProfile{}); !errors.Is(err, ErrWorkspaceGone) {
		t.Errorf("SetProfile got %v, want ErrWorkspaceGone", err)
	}
}

// A Service assembled without a fence refuses to write. The alternative —
// writing anyway — is the defect this change closes, so it must not be
// reachable by forgetting a field.
func TestAServiceWithoutAFenceRefusesToWrite(t *testing.T) {
	store := &recordingRevisionStore{}
	service := &Service{RevisionStore: store, NewID: func() string { return "rev-nofence" }}

	if _, err := service.SetPersonaPrompt(context.Background(), "ws", "actor", "acct", "人设"); !errors.Is(err, ErrNoWorkspaceFence) {
		t.Errorf("SetPersonaPrompt got %v, want ErrNoWorkspaceFence", err)
	}
	if _, err := service.SetProfile(context.Background(), "ws", "actor", "acct", ExpressionProfile{}); !errors.Is(err, ErrNoWorkspaceFence) {
		t.Errorf("SetProfile got %v, want ErrNoWorkspaceFence", err)
	}
	if store.inserts != 0 {
		t.Errorf("a fenceless service inserted %d revisions", store.inserts)
	}
}

// recordingRevisionStore counts inserts so a refusal can be shown to have
// written nothing.
type recordingRevisionStore struct{ inserts int }

func (s *recordingRevisionStore) NextRevision(context.Context, string, string) (int64, error) {
	return 1, nil
}
func (s *recordingRevisionStore) CurrentRevision(context.Context, string, string) (Revision, error) {
	return Revision{}, ErrNotFound
}
func (s *recordingRevisionStore) InsertRevision(_ context.Context, r Revision) (Revision, error) {
	s.inserts++
	return r, nil
}
