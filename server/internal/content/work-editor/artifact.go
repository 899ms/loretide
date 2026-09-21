package workeditor

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/idempotency"
)

// Documents and their editing copies.
//
// Autosave writes the editing copy and produces NO version. That is the whole
// reason the two live in different tables: saving must be cheap and frequent,
// and a history that grows on every keystroke is one nobody can restore from.

// CreateArtifact adds a document to a work.
func (s *Store) CreateArtifact(ctx context.Context, workspaceID, actor, workID string, input Artifact, requests ...idempotency.Request) (Artifact, error) {
	if s == nil {
		return Artifact{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" {
		s.reportFailure(ctx, workspaceID, actor, workID, "create-artifact", ErrInvalid)
		return Artifact{}, ErrInvalid
	}
	if err := ValidateKind(string(input.Kind)); err != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "create-artifact", err)
		return Artifact{}, err
	}
	if err := ValidateTitle(input.Title); err != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "create-artifact", err)
		return Artifact{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "create-artifact", err)
		return Artifact{}, err
	}
	defer tx.Rollback(ctx)
	var request idempotency.Request
	if len(requests) > 0 {
		request = requests[0]
		replay, replayed, claimErr := idempotency.Claim(ctx, tx, workspaceID, request)
		if claimErr != nil {
			s.reportFailure(ctx, workspaceID, actor, workID, "create-artifact", claimErr)
			return Artifact{}, claimErr
		}
		if replayed {
			var prior Artifact
			if json.Unmarshal(replay, &prior) != nil {
				return Artifact{}, idempotency.ErrStorage
			}
			return prior, nil
		}
	}
	created := Artifact{
		ArtifactID: s.newID(), WorkID: workID, WorkspaceID: workspaceID,
		Kind: input.Kind, Title: input.Title, Position: input.Position,
		// A brand-new document has no version and nothing typed, so it is
		// saved: there is nothing unsaved about it. DraftStatusFor agrees, and
		// a test holds the two together.
		DraftStatus: DraftSaved,
	}
	ctx, err = s.audit(ctx, tx, workspaceID, actor, created.ArtifactID, "create-artifact")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.ArtifactID, "create-artifact", err)
		return Artifact{}, err
	}
	// The work is confirmed inside the transaction: a document must not be
	// created under a work that belongs to another brand, and the refusal has
	// to look exactly like "no such work".
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM content_work
		WHERE workspace_id=$1 AND work_id=$2)`, workspaceID, workID).Scan(&exists); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, workID, "create-artifact", err)
		return Artifact{}, ErrStorage
	}
	if !exists {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, workID, "create-artifact", ErrNotFound)
		return Artifact{}, ErrNotFound
	}
	if err = tx.QueryRow(ctx, `INSERT INTO content_artifact
		(artifact_id, work_id, workspace_id, kind, title, position, draft_body, draft_status)
		VALUES ($1,$2,$3,$4,$5,$6,'',$7)
		RETURNING draft_saved_at, created_at, updated_at`,
		created.ArtifactID, workID, workspaceID, created.Kind, created.Title,
		created.Position, created.DraftStatus).
		Scan(&created.DraftSavedAt, &created.CreatedAt, &created.UpdatedAt); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.ArtifactID, "create-artifact", err)
		return Artifact{}, ErrStorage
	}
	if len(requests) > 0 {
		if err = idempotency.Complete(ctx, tx, workspaceID, request, created); err != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, created.ArtifactID, "create-artifact", err)
			return Artifact{}, ErrStorage
		}
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, created.ArtifactID, "create-artifact", err)
		return Artifact{}, ErrStorage
	}
	return created, nil
}

func (s *Store) ListArtifacts(ctx context.Context, workspaceID, actor, workID string) ([]Artifact, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, workID, "list-artifacts", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" {
		s.reportFailure(ctx, workspaceID, actor, workID, "list-artifacts", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, artifactSelect+
		` WHERE workspace_id=$1 AND work_id=$2 ORDER BY position, artifact_id`,
		workspaceID, workID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "list-artifacts", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	artifacts := []Artifact{}
	for rows.Next() {
		artifact, scanErr := scanArtifact(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, workID, "list-artifacts", scanErr)
			return nil, ErrStorage
		}
		artifacts = append(artifacts, artifact)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "list-artifacts", rows.Err())
		return nil, ErrStorage
	}
	return artifacts, nil
}

func (s *Store) GetArtifact(ctx context.Context, workspaceID, actor, workID, artifactID string) (Artifact, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "get-artifact", ErrStorage)
		}
		return Artifact{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" || artifactID == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "get-artifact", ErrInvalid)
		return Artifact{}, ErrInvalid
	}
	artifact, err := scanArtifact(s.DB.QueryRow(ctx, artifactSelect+
		` WHERE workspace_id=$1 AND work_id=$2 AND artifact_id=$3`,
		workspaceID, workID, artifactID))
	if errors.Is(err, pgx.ErrNoRows) {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "get-artifact", ErrNotFound)
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "get-artifact", err)
		return Artifact{}, ErrStorage
	}
	return artifact, nil
}

// ArtifactPatch is what autosave sends. Every field is optional: a keystroke
// changes the body and nothing else, and a rename changes the title and must
// not blank the draft.
type ArtifactPatch struct {
	Title     *string `json:"title,omitempty"`
	Position  *int64  `json:"position,omitempty"`
	DraftBody *string `json:"draft_body,omitempty"`
}

// PatchArtifact is the autosave path. It NEVER produces a version.
//
// When the body changes, draft_status is recomputed from the document's latest
// version rather than assumed: typing a character back to what the last version
// said really does return the document to saved, and a hard-coded "working"
// would tell the reader there is unsaved work when there is none.
func (s *Store) PatchArtifact(ctx context.Context, workspaceID, actor, workID, artifactID string, patch ArtifactPatch) (Artifact, error) {
	if s == nil {
		return Artifact{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" || artifactID == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", ErrInvalid)
		return Artifact{}, ErrInvalid
	}
	if patch.Title != nil {
		if err := ValidateTitle(*patch.Title); err != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", err)
			return Artifact{}, err
		}
	}
	if patch.DraftBody != nil {
		if err := ValidateBody(*patch.DraftBody); err != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", err)
			return Artifact{}, err
		}
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", err)
		return Artifact{}, err
	}
	defer tx.Rollback(ctx)
	ctx, err = s.audit(ctx, tx, workspaceID, actor, artifactID, "patch-artifact")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", err)
		return Artifact{}, err
	}

	current, err := scanArtifact(tx.QueryRow(ctx, artifactSelect+
		` WHERE workspace_id=$1 AND work_id=$2 AND artifact_id=$3 FOR UPDATE`,
		workspaceID, workID, artifactID))
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", ErrNotFound)
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", err)
		return Artifact{}, ErrStorage
	}

	next := current
	if patch.Title != nil {
		next.Title = *patch.Title
	}
	if patch.Position != nil {
		next.Position = *patch.Position
	}
	if patch.DraftBody != nil {
		next.DraftBody = *patch.DraftBody
		latest, latestErr := latestVersionTx(ctx, tx, workspaceID, artifactID)
		if latestErr != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", latestErr)
			return Artifact{}, ErrStorage
		}
		next.DraftStatus = DraftStatusFor(next.DraftBody, latest)
	}

	updated, err := scanArtifact(tx.QueryRow(ctx, `UPDATE content_artifact
		SET title=$4, position=$5, draft_body=$6, draft_status=$7,
		    draft_saved_at=now(), updated_at=now()
		WHERE workspace_id=$1 AND work_id=$2 AND artifact_id=$3
		RETURNING artifact_id, work_id, workspace_id, kind, title, position,
		          draft_body, draft_status, draft_saved_at, created_at, updated_at`,
		workspaceID, workID, artifactID, next.Title, next.Position,
		next.DraftBody, next.DraftStatus))
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", err)
		return Artifact{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "patch-artifact", err)
		return Artifact{}, ErrStorage
	}
	return updated, nil
}

// latestVersionTx returns the document's newest version, or nil when it has
// none. nil is an ordinary state: a document that has never been saved as a
// version is normal, not an error.
func latestVersionTx(ctx context.Context, tx pgx.Tx, workspaceID, artifactID string) (*ArtifactVersion, error) {
	version, err := scanVersion(tx.QueryRow(ctx, versionSelect+
		` WHERE workspace_id=$1 AND artifact_id=$2 ORDER BY revision DESC LIMIT 1`,
		workspaceID, artifactID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}
