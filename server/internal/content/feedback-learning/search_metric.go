package feedbacklearning

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Search metrics (specs/036 PR 4, FR-070 to FR-072): one row per number a
// person copied off a platform's back end. Append-only; a correction is a new
// row. Rows are returned as rows: nothing here totals, averages or picks the
// latest value of anything.

const searchMetricColumns = `search_metric_id, publication_record_id, platform,
	account_id, metric, value, unit, stat_window, sampled_at, evidence_note,
	source_type, recorded_by, created_at`

// scanSearchMetric reads a row. Value is scanned into *int64, so a NULL
// column stays nil all the way out.
func scanSearchMetric(row scanner) (SearchMetricRecord, error) {
	var record SearchMetricRecord
	err := row.Scan(&record.SearchMetricID, &record.PublicationRecordID, &record.Platform,
		&record.AccountID, &record.Metric, &record.Value, &record.Unit, &record.StatWindow,
		&record.SampledAt, &record.EvidenceNote, &record.SourceType, &record.RecordedBy,
		&record.CreatedAt)
	record.SampledAt = record.SampledAt.UTC()
	record.CreatedAt = record.CreatedAt.UTC()
	record.DataOrigin = DataOriginManualOnly
	return record, err
}

// RecordSearchMetric writes one search metric. The source is the entry
// point's, never the body's; this version has one.
//
// Order: the input's own rules (400 by field), then inside the fenced
// transaction the publication record (not this brand's -> ErrNotFound, the
// same answer as a missing one) and the account (400 account_id), then the
// row and its audit event.
func (s *SearchStore) RecordSearchMetric(ctx context.Context, workspaceID, actor string, input SearchMetricInput) (SearchMetricRecord, error) {
	if !s.ready() {
		return SearchMetricRecord{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return SearchMetricRecord{}, ErrInvalid
	}
	step := func() string { return "record-search-metric" }
	sampledAt, err := ValidateSearchMetric(input)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", step(), err)
		return SearchMetricRecord{}, err
	}
	record := SearchMetricRecord{
		PublicationRecordID: input.PublicationRecordID, Platform: input.Platform,
		AccountID: input.AccountID, Metric: input.Metric,
		// Passed through untouched: nil reaches the column as NULL.
		Value: input.Value, Unit: input.Unit, StatWindow: input.StatWindow,
		SampledAt: sampledAt, EvidenceNote: input.EvidenceNote,
		SourceType: SearchSourceManual, RecordedBy: actor, DataOrigin: DataOriginManualOnly,
	}
	err = s.inTx(ctx, workspaceID, actor, step, func(ctx context.Context, tx pgx.Tx) (string, error) {
		if err := s.checkSearchPublication(ctx, workspaceID, record.PublicationRecordID); err != nil {
			return "", err
		}
		if err := s.checkSearchAccount(ctx, workspaceID, record.AccountID); err != nil {
			return "", referenceField("account_id", err)
		}
		record.SearchMetricID = s.newID()
		if err := tx.QueryRow(ctx, `INSERT INTO content_search_metric
			(workspace_id, search_metric_id, publication_record_id, platform, account_id,
			 metric, value, unit, stat_window, sampled_at, evidence_note, source_type,
			 recorded_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			RETURNING created_at`,
			workspaceID, record.SearchMetricID, record.PublicationRecordID, string(record.Platform),
			record.AccountID, string(record.Metric), record.Value, record.Unit, record.StatWindow,
			record.SampledAt, record.EvidenceNote, string(record.SourceType), actor).
			Scan(&record.CreatedAt); err != nil {
			return record.SearchMetricID, ErrStorage
		}
		record.CreatedAt = record.CreatedAt.UTC()
		return record.SearchMetricID, nil
	})
	if err != nil {
		return SearchMetricRecord{}, err
	}
	return record, nil
}

// ListSearchMetrics answers every search metric of one publication record,
// newest sample first. A metric nobody recorded is simply absent - the page
// shows it as unknown, never as 0 (FR-072). A record that is not this
// brand's answers ErrNotFound, the same as a missing one.
func (s *SearchStore) ListSearchMetrics(ctx context.Context, workspaceID, actor, publicationRecordID string) ([]SearchMetricRecord, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	if publicationRecordID == "" {
		return nil, invalidField("publication_record_id")
	}
	if err := s.checkSearchPublication(ctx, workspaceID, publicationRecordID); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT `+searchMetricColumns+`
		FROM content_search_metric
		WHERE workspace_id = $1 AND publication_record_id = $2
		ORDER BY sampled_at DESC, search_metric_id`, workspaceID, publicationRecordID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-search-metrics", err)
		return nil, ErrStorage
	}
	return collect(rows, scanSearchMetric)
}
