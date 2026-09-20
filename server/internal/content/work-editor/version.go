package workeditor

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// The append-only version history.
//
// There is no UPDATE and no DELETE against content_artifact_version anywhere in
// this package, and TestVersionStoreHasNoUpdateOrDeletePath keeps it that way.
// SOP 7.1 requires that approved, handed over and published versions are always
// kept, and a history that can be rewritten promises nothing of the sort.
//
// Restoring and adopting both APPEND. Neither rewinds the history, because a
// history you can rewind is one you cannot answer "what did we send" from.

// SaveVersion stores the editing copy as a new version.
//
// (source, action) = (edited, saved). The document's draft_status becomes saved
// in the same transaction: the two must not be able to disagree across a crash.
func (s *Store) SaveVersion(ctx context.Context, workspaceID, actor, workID, artifactID string) (ArtifactVersion, error) {
	return s.appendVersion(ctx, workspaceID, actor, workID, artifactID, versionIntent{
		step: "save-version", source: SourceEdited, action: ActionSaved,
	})
}

// RestoreVersion appends a new version carrying an older one's content.
//
// The source stays `edited`: what comes back is still what a person wrote. That
// it arrived by restoring is the ACTION, with restored_from naming where from.
// The editing copy is updated too, or the screen would say "restored" while the
// editor still showed the newer text.
func (s *Store) RestoreVersion(ctx context.Context, workspaceID, actor, workID, artifactID, versionID string) (ArtifactVersion, error) {
	return s.appendVersion(ctx, workspaceID, actor, workID, artifactID, versionIntent{
		step: "restore-version", source: SourceEdited, action: ActionRestored,
		fromVersionID: versionID,
	})
}

// AdoptVersion appends a new version marking a baseline for the next step.
//
// SOP 7.1: "采用当前版本表示『此版是下一步工作基线』". A mutable pointer would
// be cheaper, and would lose the history of which versions were ever adopted -
// which is the question the record exists to answer.
func (s *Store) AdoptVersion(ctx context.Context, workspaceID, actor, workID, artifactID, versionID string) (ArtifactVersion, error) {
	return s.appendVersion(ctx, workspaceID, actor, workID, artifactID, versionIntent{
		step: "adopt-version", source: SourceAdopted, action: ActionAdopted,
		fromVersionID: versionID,
	})
}

type versionIntent struct {
	step   string
	source Source
	action Action
	// fromVersionID is set for restore and adopt. Empty means the content comes
	// from the editing copy.
	fromVersionID string
}

// appendVersion is the only path that writes a version.
//
// The revision number is taken in TWO statements, and that is not incidental.
// READ COMMITTED gives a statement one snapshot, taken when the statement
// starts, and waiting for a row lock does not refresh it. Counting inside the
// locking statement therefore counts the versions as they were before the
// writer that won the lock committed, picks a number that writer already used,
// and turns a legitimate save into a unique-index failure the caller sees as an
// error. topic-planning shipped the other order and it cost Issue #109; the
// second statement starts after the lock is granted, so its snapshot includes
// whatever the winner wrote.
func (s *Store) appendVersion(ctx context.Context, workspaceID, actor, workID, artifactID string, intent versionIntent) (ArtifactVersion, error) {
	if s == nil {
		return ArtifactVersion{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" || artifactID == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, ErrInvalid)
		return ArtifactVersion{}, ErrInvalid
	}

	// The source and action are chosen by the code above, not by a caller, so
	// this catches a mistake in THIS package rather than bad input. Without it
	// the database CHECK is the only thing that notices, and it surfaces as an
	// opaque storage failure instead of naming the value that was wrong.
	if err := ValidateSource(string(intent.source)); err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, err
	}
	if err := ValidateAction(string(intent.action)); err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, err
	}
	defer tx.Rollback(ctx)
	ctx, err = s.audit(ctx, tx, workspaceID, actor, artifactID, intent.step)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, err
	}

	// Statement one: take the document's row lock. Scoped to the work as well,
	// so a document id from another work is refused exactly like a missing one.
	artifact, err := scanArtifact(tx.QueryRow(ctx, artifactSelect+
		` WHERE workspace_id=$1 AND work_id=$2 AND artifact_id=$3 FOR UPDATE`,
		workspaceID, workID, artifactID))
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, ErrNotFound)
		return ArtifactVersion{}, ErrNotFound
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, ErrStorage
	}

	body := artifact.DraftBody
	if intent.fromVersionID != "" {
		source, sourceErr := scanVersion(tx.QueryRow(ctx, versionSelect+
			` WHERE workspace_id=$1 AND artifact_id=$2 AND version_id=$3`,
			workspaceID, artifactID, intent.fromVersionID))
		if errors.Is(sourceErr, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, ErrNotFound)
			return ArtifactVersion{}, ErrNotFound
		}
		if sourceErr != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, sourceErr)
			return ArtifactVersion{}, ErrStorage
		}
		body = source.Body
	}

	// Statement two: number the version. Its snapshot is taken now, after the
	// lock was granted, so it sees the winner's row.
	var next int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(revision), 0) + 1
		FROM content_artifact_version WHERE workspace_id=$1 AND artifact_id=$2`,
		workspaceID, artifactID).Scan(&next); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, ErrStorage
	}

	version := ArtifactVersion{
		VersionID: s.newID(), ArtifactID: artifactID, WorkID: workID,
		WorkspaceID: workspaceID, Revision: next, Source: intent.source,
		Action: intent.action, Body: body, ActorID: actor,
	}
	switch intent.action {
	case ActionRestored:
		version.RestoredFrom = intent.fromVersionID
	case ActionAdopted:
		version.AdoptedFrom = intent.fromVersionID
	}

	if err = tx.QueryRow(ctx, `INSERT INTO content_artifact_version
		(version_id, artifact_id, work_id, workspace_id, revision, source, action,
		 body, restored_from, adopted_from, actor_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING created_at`,
		version.VersionID, artifactID, workID, workspaceID, next, version.Source,
		version.Action, version.Body, version.RestoredFrom, version.AdoptedFrom,
		actor).Scan(&version.CreatedAt); err != nil {
		_ = tx.Rollback(ctx)
		if isUniqueViolation(err) {
			// Somebody else took this number. Reported as a conflict, not as a
			// storage failure: the caller retries and succeeds.
			s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, ErrConflict)
			return ArtifactVersion{}, ErrConflict
		}
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, ErrStorage
	}

	// The editing copy now matches this version, so its status is saved. A
	// restore also replaces the text: without that the editor would still show
	// the newer draft while the history said the old one had been restored.
	if _, err = tx.Exec(ctx, `UPDATE content_artifact
		SET draft_body=$4, draft_status=$5, draft_saved_at=now(), updated_at=now()
		WHERE workspace_id=$1 AND work_id=$2 AND artifact_id=$3`,
		workspaceID, workID, artifactID, body, DraftSaved); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, ErrStorage
	}

	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, err)
		return ArtifactVersion{}, ErrStorage
	}
	return version, nil
}

// ListVersions returns a document's history, newest first.
func (s *Store) ListVersions(ctx context.Context, workspaceID, actor, workID, artifactID string) ([]ArtifactVersion, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-versions", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" || artifactID == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-versions", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, versionSelect+
		` WHERE workspace_id=$1 AND work_id=$2 AND artifact_id=$3 ORDER BY revision DESC`,
		workspaceID, workID, artifactID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-versions", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	versions := []ArtifactVersion{}
	for rows.Next() {
		version, scanErr := scanVersion(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-versions", scanErr)
			return nil, ErrStorage
		}
		versions = append(versions, version)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-versions", rows.Err())
		return nil, ErrStorage
	}
	return versions, nil
}

// GetVersion returns one version by its stable key, scoped to the workspace AND
// the document: a version id from another brand is refused exactly as a missing
// one, so an id cannot be used to probe.
func (s *Store) GetVersion(ctx context.Context, workspaceID, actor, workID, artifactID, versionID string) (ArtifactVersion, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, versionID, "get-version", ErrStorage)
		}
		return ArtifactVersion{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" || artifactID == "" || versionID == "" {
		s.reportFailure(ctx, workspaceID, actor, versionID, "get-version", ErrInvalid)
		return ArtifactVersion{}, ErrInvalid
	}
	version, err := scanVersion(s.DB.QueryRow(ctx, versionSelect+
		` WHERE workspace_id=$1 AND work_id=$2 AND artifact_id=$3 AND version_id=$4`,
		workspaceID, workID, artifactID, versionID))
	if errors.Is(err, pgx.ErrNoRows) {
		s.reportFailure(ctx, workspaceID, actor, versionID, "get-version", ErrNotFound)
		return ArtifactVersion{}, ErrNotFound
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, versionID, "get-version", err)
		return ArtifactVersion{}, ErrStorage
	}
	return version, nil
}
