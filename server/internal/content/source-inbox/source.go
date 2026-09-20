package sourceinbox

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Created is what collecting answers with: the row, its snapshot when there is
// one, and the items that already hold the same content.
//
// The duplicates are a HINT. Nothing was merged and nothing was removed - §4
// says "重复素材先提示合并关联" and R-011 says "内容相同不删除独立的收藏上下文与
// 批注", so the two items stay two items and a person decides.
type Created struct {
	Source     Source
	Snapshot   *Snapshot
	Duplicates []string
}

// CreateSource records one act of collecting.
//
// A pasted text gets exactly one snapshot, written in the same transaction as
// the row: a source whose body did not land is not a source, and two statements
// that can half-commit would produce one.
//
// A url gets none. There is nothing to snapshot until something reads the page,
// and this card reads nothing.
func (s *Store) CreateSource(ctx context.Context, workspaceID, actor string, input NewSource) (Created, error) {
	if s == nil || s.DB == nil {
		return Created{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "create-source", ErrInvalid)
		return Created{}, ErrInvalid
	}
	if err := ValidateNew(input); err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "create-source", err)
		return Created{}, err
	}

	// The hint is read before the write so that the answer describes what was
	// already there. Reading it afterwards would include the row just written.
	var duplicates []string
	var hash string
	if input.Kind == KindPastedText {
		hash = ContentHash(input.Content)
		found, err := s.DuplicatesByHash(ctx, workspaceID, actor, hash)
		if err != nil {
			return Created{}, err
		}
		duplicates = found
	}
	if duplicates == nil {
		duplicates = []string{}
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "create-source", err)
		return Created{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sourceID := s.newID()
	tags := input.Tags
	if tags == nil {
		tags = []string{}
	}
	row := tx.QueryRow(ctx, `INSERT INTO content_source
		(source_id, workspace_id, kind, url, captured_at, recorded_by,
		 historical_import, title, tags, annotation, personal_judgement,
		 status, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$5)
		RETURNING source_id, workspace_id, kind, url, captured_at, recorded_by,
		          historical_import, title, tags, annotation, personal_judgement,
		          status, updated_at`,
		sourceID, workspaceID, string(input.Kind), input.URL, s.now(), actor,
		input.HistoricalImport, input.Title, tags, input.Annotation,
		input.PersonalJudgement, string(StatusInbox))
	source, err := scanSource(row)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "create-source", err)
		return Created{}, ErrStorage
	}

	var snapshot *Snapshot
	if input.Kind == KindPastedText {
		written, snapErr := s.insertSnapshot(ctx, tx, workspaceID, sourceID, input.Content, hash)
		if snapErr != nil {
			s.reportFailure(ctx, workspaceID, actor, sourceID, "create-source", snapErr)
			return Created{}, ErrStorage
		}
		snapshot = &written
	}

	if _, err = s.audit(ctx, tx, workspaceID, actor, sourceID, "create-source"); err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "create-source", err)
		return Created{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "create-source", err)
		return Created{}, ErrStorage
	}
	return Created{Source: source, Snapshot: snapshot, Duplicates: duplicates}, nil
}

// insertSnapshot is the only writer of content_source_snapshot, and it only
// ever INSERTs.
func (s *Store) insertSnapshot(ctx context.Context, tx pgx.Tx, workspaceID, sourceID, content, hash string) (Snapshot, error) {
	row := tx.QueryRow(ctx, `INSERT INTO content_source_snapshot
		(snapshot_id, workspace_id, source_id, content, content_hash, captured_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING snapshot_id, workspace_id, source_id, content, content_hash, captured_at`,
		s.newID(), workspaceID, sourceID, content, hash, s.now())
	return scanSnapshot(row)
}

// ListSources returns this brand's inbox, newest first.
//
// status and tag are both optional. An empty status means every status,
// including archived - the caller decides, because "the default list hides
// archived" is a page's rule, not storage's.
func (s *Store) ListSources(ctx context.Context, workspaceID, actor, status, tag string) ([]Source, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "list-sources", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "list-sources", ErrInvalid)
		return nil, ErrInvalid
	}
	if status != "" {
		if err := ValidateStatus(status); err != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "list-sources", err)
			return nil, err
		}
	}
	rows, err := s.DB.Query(ctx, `SELECT source_id, workspace_id, kind, url,
		captured_at, recorded_by, historical_import, title, tags, annotation,
		personal_judgement, status, updated_at
		FROM content_source
		WHERE workspace_id = $1
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR $3 = ANY(tags))
		ORDER BY captured_at DESC, source_id`, workspaceID, status, tag)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "list-sources", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	sources := []Source{}
	for rows.Next() {
		source, scanErr := scanSource(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "list-sources", scanErr)
			return nil, ErrStorage
		}
		sources = append(sources, source)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "list-sources", rows.Err())
		return nil, ErrStorage
	}
	return sources, nil
}

// GetSource returns one item and its snapshot, or ErrNotFound. A url source
// has no snapshot, and that is an ordinary answer rather than a missing row.
func (s *Store) GetSource(ctx context.Context, workspaceID, actor, sourceID string) (Source, *Snapshot, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, sourceID, "get-source", ErrStorage)
		}
		return Source{}, nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || sourceID == "" {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "get-source", ErrInvalid)
		return Source{}, nil, ErrInvalid
	}
	row := s.DB.QueryRow(ctx, `SELECT source_id, workspace_id, kind, url,
		captured_at, recorded_by, historical_import, title, tags, annotation,
		personal_judgement, status, updated_at
		FROM content_source WHERE workspace_id = $1 AND source_id = $2`,
		workspaceID, sourceID)
	source, err := scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "get-source", ErrNotFound)
		return Source{}, nil, ErrNotFound
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "get-source", err)
		return Source{}, nil, ErrStorage
	}
	snapRow := s.DB.QueryRow(ctx, `SELECT snapshot_id, workspace_id, source_id,
		content, content_hash, captured_at
		FROM content_source_snapshot
		WHERE workspace_id = $1 AND source_id = $2
		ORDER BY captured_at, snapshot_id LIMIT 1`, workspaceID, sourceID)
	snapshot, err := scanSnapshot(snapRow)
	if errors.Is(err, pgx.ErrNoRows) {
		return source, nil, nil
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "get-source", err)
		return Source{}, nil, ErrStorage
	}
	return source, &snapshot, nil
}

func scanSource(row scanner) (Source, error) {
	var source Source
	var kind, status string
	err := row.Scan(&source.SourceID, &source.WorkspaceID, &kind, &source.URL,
		&source.CapturedAt, &source.RecordedBy, &source.HistoricalImport,
		&source.Title, &source.Tags, &source.Annotation,
		&source.PersonalJudgement, &status, &source.UpdatedAt)
	source.Kind = Kind(kind)
	source.Status = Status(status)
	if source.Tags == nil {
		source.Tags = []string{}
	}
	return source, err
}

func scanSnapshot(row scanner) (Snapshot, error) {
	var snapshot Snapshot
	err := row.Scan(&snapshot.SnapshotID, &snapshot.WorkspaceID, &snapshot.SourceID,
		&snapshot.Content, &snapshot.ContentHash, &snapshot.CapturedAt)
	return snapshot, err
}
