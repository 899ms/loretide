package feedbacklearning

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Ranking observations (specs/036 PR 4, FR-073 to FR-076): one person
// looking once. Revision-based: the current state of an observation is its
// highest revision, and a correction or a void is one more revision - the
// first one is never rewritten.
//
// The list answers observations one by one, newest first. Nothing here
// combines two of them: no average, no best, no "current rank" taken from the
// most recent one. Each carries rank.single_observation.

const rankObservationColumns = `observation_id, revision, voided, platform, account_id,
	query, theme_id, publication_record_id, observed_at, conditions, result_kind,
	position, scanned_depth, evidence_note, recorded_by, created_at`

func scanRankObservation(row scanner) (RankObservation, error) {
	var observation RankObservation
	err := row.Scan(&observation.ObservationID, &observation.Revision, &observation.Voided,
		&observation.Platform, &observation.AccountID, &observation.Query, &observation.ThemeID,
		&observation.PublicationRecordID, &observation.ObservedAt, &observation.Conditions,
		&observation.ResultKind, &observation.Position, &observation.ScannedDepth,
		&observation.EvidenceNote, &observation.RecordedBy, &observation.CreatedAt)
	observation.ObservedAt = observation.ObservedAt.UTC()
	observation.CreatedAt = observation.CreatedAt.UTC()
	return ViewRankObservation(observation), err
}

// latestRankObservation reads the current revision of one observation.
func latestRankObservation(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, workspaceID, observationID string) (RankObservation, error) {
	observation, err := scanRankObservation(q.QueryRow(ctx, `SELECT `+rankObservationColumns+`
		FROM content_search_rank_observation_revision
		WHERE workspace_id = $1 AND observation_id = $2
		ORDER BY revision DESC LIMIT 1`, workspaceID, observationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return RankObservation{}, ErrNotFound
	}
	if err != nil {
		return RankObservation{}, ErrStorage
	}
	return observation, nil
}

// GetRankObservation answers the current revision of one observation,
// voided or not. Not this brand's answers ErrNotFound.
func (s *SearchStore) GetRankObservation(ctx context.Context, workspaceID, actor, observationID string) (RankObservation, error) {
	if !s.ready() {
		return RankObservation{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || observationID == "" {
		return RankObservation{}, ErrNotFound
	}
	return latestRankObservation(ctx, s.DB, workspaceID, observationID)
}

// RecordRankObservation writes revision 1 of a new observation.
func (s *SearchStore) RecordRankObservation(ctx context.Context, workspaceID, actor string, input RankObservationInput) (RankObservation, error) {
	return s.writeRankObservation(ctx, workspaceID, actor, "", RankObservationRevision{Input: input})
}

// ReviseRankObservation writes the next revision of an observation; voided =
// true voids it. The revision it was based on must still be the latest.
func (s *SearchStore) ReviseRankObservation(ctx context.Context, workspaceID, actor, observationID string, revision RankObservationRevision) (RankObservation, error) {
	if observationID == "" {
		return RankObservation{}, ErrNotFound
	}
	return s.writeRankObservation(ctx, workspaceID, actor, observationID, revision)
}

// writeRankObservation keeps contract §7.1's order after the path: the
// input's own rules (400 by field), then inside the fenced transaction the
// current revision, the references (400 by field; a foreign id and a missing
// one alike), base_revision (409), one INSERT and the audit event.
func (s *SearchStore) writeRankObservation(ctx context.Context, workspaceID, actor, observationID string,
	revision RankObservationRevision) (RankObservation, error) {
	if !s.ready() {
		return RankObservation{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return RankObservation{}, ErrInvalid
	}
	create := observationID == ""
	step := func() string {
		switch {
		case create:
			return "record-rank-observation"
		case revision.Voided:
			return "void-rank-observation"
		default:
			return "revise-rank-observation"
		}
	}
	record, err := ValidateRankObservation(revision.Input, s.now())
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, observationID, step(), err)
		return RankObservation{}, err
	}
	err = s.inTx(ctx, workspaceID, actor, step, func(ctx context.Context, tx pgx.Tx) (string, error) {
		latest := 0
		if !create {
			current, err := latestRankObservation(ctx, tx, workspaceID, observationID)
			if err != nil {
				return observationID, err
			}
			latest = current.Revision
		}
		if err := s.checkSearchAccount(ctx, workspaceID, record.AccountID); err != nil {
			return observationID, referenceField("account_id", err)
		}
		if err := s.checkSearchTheme(ctx, workspaceID, actor, record.ThemeID); err != nil {
			return observationID, referenceField("theme_id", err)
		}
		if err := s.checkSearchPublication(ctx, workspaceID, record.PublicationRecordID); err != nil {
			return observationID, referenceField("publication_record_id", err)
		}
		if create {
			observationID = s.newID()
		} else if revision.BaseRevision != latest {
			return observationID, RevisionConflict{Field: "base_revision"}
		}
		record.ObservationID, record.Revision, record.Voided = observationID, latest+1, revision.Voided
		record.RecordedBy = actor
		if s.BeforeRevisionInsert != nil {
			s.BeforeRevisionInsert(ctx, observationID)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO content_search_rank_observation_revision
			(workspace_id, observation_id, revision, voided, platform, account_id, query,
			 theme_id, publication_record_id, observed_at, conditions, result_kind, position,
			 scanned_depth, evidence_note, recorded_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			RETURNING created_at`,
			workspaceID, record.ObservationID, record.Revision, record.Voided, string(record.Platform),
			record.AccountID, record.Query, record.ThemeID, record.PublicationRecordID,
			record.ObservedAt, record.Conditions, string(record.ResultKind), record.Position,
			record.ScannedDepth, record.EvidenceNote, actor).
			Scan(&record.CreatedAt); err != nil {
			return observationID, insertError(err)
		}
		record.CreatedAt = record.CreatedAt.UTC()
		return observationID, nil
	})
	if err != nil {
		return RankObservation{}, err
	}
	return ViewRankObservation(record), nil
}

// ListRankObservations answers the current revision of each matching
// observation, newest observation first, then by observation_id (FR-075).
// Voided observations only when asked. One row per observation, each on its
// own: there is no summary of them.
func (s *SearchStore) ListRankObservations(ctx context.Context, workspaceID, actor string, filter RankObservationFilter) ([]RankObservation, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	if filter.ThemeID == "" && filter.PublicationRecordID == "" && filter.Query == "" {
		return nil, FieldError{Field: "theme_id", Reason: "one of theme_id, publication_record_id or query is required"}
	}
	rows, err := s.DB.Query(ctx, `SELECT `+rankObservationColumns+` FROM (
			SELECT DISTINCT ON (observation_id) `+rankObservationColumns+`
			FROM content_search_rank_observation_revision WHERE workspace_id = $1
			ORDER BY observation_id, revision DESC) latest
		WHERE ($2::boolean OR NOT voided)
		  AND ($3::text = '' OR theme_id = $3::text)
		  AND ($4::text = '' OR publication_record_id = $4::text)
		  AND ($5::text = '' OR query = $5::text)
		ORDER BY observed_at DESC, observation_id`,
		workspaceID, filter.IncludeVoided, filter.ThemeID, filter.PublicationRecordID, filter.Query)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "list-rank-observations", err)
		return nil, ErrStorage
	}
	return collect(rows, scanRankObservation)
}
