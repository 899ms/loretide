package ipprofile

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// A confirmed configuration snapshot for one account.
//
// Revisions are append-only. Setting the persona prompt writes a NEW revision
// rather than changing the last one, because a run that pinned an earlier
// revision has to keep seeing what it pinned - that is the whole meaning of
// "old revisions stay readable, only new runs take the new config".
//
// RevisionID is the cross-table reference. Revision is a per-account counter
// for reading and ordering; it is not a key, because "version 3" names a
// different thing for every account.
type Revision struct {
	RevisionID    string `json:"revision_id"`
	AccountID     string `json:"account_id"`
	WorkspaceID   string `json:"workspace_id"`
	Revision      int64  `json:"revision"`
	PersonaPrompt string `json:"persona_prompt"`
	// Profile is the rest of the expression configuration (SOP 3.1). It rides
	// the same revision as the prompt, so one confirmation is one snapshot and
	// a run pins one revision_id.
	Profile   ExpressionProfile `json:"profile"`
	CreatedAt string            `json:"created_at"`
}

// MaxPersonaPromptRunes bounds one prompt. Counted in runes, not bytes: a
// Chinese persona costs three bytes per character, and a byte limit would give
// it a third of the room an English one gets.
const MaxPersonaPromptRunes = 20000

// maxRevisionAttempts bounds the retry on a revision-number collision. Retrying
// at all is deliberate - both confirmations really happened and both belong in
// the history - but retrying forever turns one write into an unbounded wait.
const maxRevisionAttempts = 3

var (
	ErrPersonaPromptTooLong = errors.New("persona prompt is too long")
	// ErrRevisionTaken is what a store reports when the revision number it tried
	// is already used. Retryable.
	ErrRevisionTaken = errors.New("revision number already taken")
	// ErrRevisionConflict is what the caller sees once retries are exhausted.
	// Answered as 409: the write did not happen and the caller may try again.
	ErrRevisionConflict = errors.New("revision conflict")
)

// ValidatePersonaPrompt accepts blank.
//
// SOP 3.1 keeps unconfirmed items pending, so "the creator confirmed and the
// persona is still empty" is a real state and a real revision. Rejecting blank
// would leave them unable to move on, and would turn a normal step of the flow
// into an error message.
//
// Whitespace is not trimmed into a rejection either, for the same reason.
func ValidatePersonaPrompt(prompt string) error {
	if utf8.RuneCountInString(prompt) > MaxPersonaPromptRunes {
		return ErrPersonaPromptTooLong
	}
	return nil
}

// RevisionStore is the append-only persistence for revisions. There is no
// update and no delete here by design; the only removal is workspace deletion,
// which is a different statement in a different file.
type RevisionStore interface {
	NextRevision(ctx context.Context, workspaceID, accountID string) (int64, error)
	InsertRevision(ctx context.Context, revision Revision) (Revision, error)
}

// RevisionReader is the read half. Separate from RevisionStore so a test that
// only exercises the write path does not have to stub four readers it never
// calls.
type RevisionReader interface {
	GetRevision(ctx context.Context, workspaceID, revisionID string) (Revision, error)
	CurrentRevision(ctx context.Context, workspaceID, accountID string) (Revision, error)
	ListRevisions(ctx context.Context, workspaceID, accountID string) ([]Revision, error)
}

// CurrentPersonaRevision returns the highest-numbered revision for an account.
func (s *Service) CurrentPersonaRevision(ctx context.Context, workspaceID, accountID string) (Revision, error) {
	reader, ok := s.RevisionStore.(RevisionReader)
	if !ok {
		return Revision{}, ErrNotFound
	}
	return reader.CurrentRevision(ctx, workspaceID, accountID)
}

// PersonaRevision returns one revision by id - the determinate snapshot a run
// pins. The account id is checked too, so a revision id from another account
// cannot be read through this one.
func (s *Service) PersonaRevision(ctx context.Context, workspaceID, accountID, revisionID string) (Revision, error) {
	reader, ok := s.RevisionStore.(RevisionReader)
	if !ok {
		return Revision{}, ErrNotFound
	}
	revision, err := reader.GetRevision(ctx, workspaceID, revisionID)
	if err != nil {
		return Revision{}, err
	}
	if revision.AccountID != accountID {
		// Right workspace, wrong account: same answer as not existing.
		return Revision{}, ErrNotFound
	}
	return revision, nil
}

func (s *Service) ListPersonaRevisions(ctx context.Context, workspaceID, accountID string) ([]Revision, error) {
	reader, ok := s.RevisionStore.(RevisionReader)
	if !ok {
		return nil, ErrNotFound
	}
	return reader.ListRevisions(ctx, workspaceID, accountID)
}

// SetPersonaPrompt records a new revision.
//
// Two writers can read the same current revision and both try to claim N+1. The
// unique index on (account_id, revision) lets exactly one of them land; the
// other gets ErrRevisionTaken and comes back around, so both confirmations end
// up in the history as N+1 and N+2 rather than one of them vanishing.
//
// After maxRevisionAttempts it reports ErrRevisionConflict instead of spinning.
func (s *Service) SetPersonaPrompt(ctx context.Context, workspaceID, actor, accountID, prompt string) (Revision, error) {
	if err := ValidatePersonaPrompt(prompt); err != nil {
		return Revision{}, err
	}
	for attempt := 0; attempt < maxRevisionAttempts; attempt++ {
		next, err := s.RevisionStore.NextRevision(ctx, workspaceID, accountID)
		if err != nil {
			return Revision{}, err
		}
		// Read the other half after choosing the candidate number. If another
		// writer lands before this read, it is carried. If it lands after this
		// read, both writers contend for the same number and the loser retries,
		// refreshing the profile instead of quietly restoring an older one.
		carried, err := s.carriedProfile(ctx, workspaceID, accountID)
		if err != nil {
			return Revision{}, err
		}
		written, err := s.RevisionStore.InsertRevision(ctx, Revision{
			RevisionID:    s.newID(),
			AccountID:     accountID,
			WorkspaceID:   workspaceID,
			Revision:      next,
			PersonaPrompt: prompt,
			Profile:       carried,
		})
		if err == nil {
			s.auditRevision(ctx, workspaceID, actor, accountID, written.Revision)
			return written, nil
		}
		if !errors.Is(err, ErrRevisionTaken) {
			return Revision{}, err
		}
		// Someone else claimed this number. Read the new maximum and try again.
	}
	return Revision{}, ErrRevisionConflict
}

// SetProfile records a new revision carrying the confirmed expression profile.
//
// The persona prompt comes along unchanged, for the same reason the profile
// does when the prompt is what changed: a revision is the whole configuration,
// and a write that only knows about half of it would drop the other half.
//
// The caller's original shape is validated before normalisation can discard a
// blank list entry or lower a status. Valid blank content is then normalised
// and the result is validated again for controlled values. A blank field marked
// confirmed is lowered to pending rather than recording a decision the creator
// did not make, while an unknown status is refused even when its value is blank.
func (s *Service) SetProfile(ctx context.Context, workspaceID, actor, accountID string, profile ExpressionProfile) (Revision, error) {
	if err := validateProfileShape(profile); err != nil {
		return Revision{}, err
	}
	normalized := NormalizeProfile(profile)
	if err := ValidateProfile(normalized); err != nil {
		return Revision{}, err
	}

	for attempt := 0; attempt < maxRevisionAttempts; attempt++ {
		next, err := s.RevisionStore.NextRevision(ctx, workspaceID, accountID)
		if err != nil {
			return Revision{}, err
		}
		// Same ordering as SetPersonaPrompt: either observe a concurrent prompt
		// now, or collide on the candidate number and refresh it on the retry.
		carried, err := s.carriedPersonaPrompt(ctx, workspaceID, accountID)
		if err != nil {
			return Revision{}, err
		}
		written, err := s.RevisionStore.InsertRevision(ctx, Revision{
			RevisionID:    s.newID(),
			AccountID:     accountID,
			WorkspaceID:   workspaceID,
			Revision:      next,
			PersonaPrompt: carried,
			Profile:       normalized,
		})
		if err == nil {
			s.auditRevision(ctx, workspaceID, actor, accountID, written.Revision)
			return written, nil
		}
		if !errors.Is(err, ErrRevisionTaken) {
			return Revision{}, err
		}
	}
	return Revision{}, ErrRevisionConflict
}

// carriedProfile is the profile the next revision inherits. An account with no
// revision yet inherits an empty one, which is the same thing as every field
// pending.
func (s *Service) carriedProfile(ctx context.Context, workspaceID, accountID string) (ExpressionProfile, error) {
	current, err := s.CurrentPersonaRevision(ctx, workspaceID, accountID)
	if errors.Is(err, ErrNotFound) {
		// No revision yet is not a failure: the first prompt is written against
		// an empty profile.
		return ExpressionProfile{}, nil
	}
	if err != nil {
		return ExpressionProfile{}, err
	}
	return current.Profile, nil
}

// carriedPersonaPrompt is the prompt the next profile revision inherits. Only
// an account with no revision starts blank; a storage failure must stop the
// write, otherwise it would quietly turn a real prompt into an empty one.
func (s *Service) carriedPersonaPrompt(ctx context.Context, workspaceID, accountID string) (string, error) {
	current, err := s.CurrentPersonaRevision(ctx, workspaceID, accountID)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return current.PersonaPrompt, nil
}

// auditRevision records who confirmed which account's configuration, and which
// version it became. This is the module's real diagnostics integration: a
// persona change is exactly the kind of thing someone later needs to trace when
// output shifts without an obvious cause.
func (s *Service) auditRevision(ctx context.Context, workspaceID, actor, accountID string, revision int64) {
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
		Action:     "execute",
		Outcome:    "success",
		Component:  "ip-profile",
		Severity:   "info",
		Build:      s.Build,
		Attempt:    int(revision),
	})
}
