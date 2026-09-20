package feedbacklearning

import (
	"context"
	"time"
)

// Recording numbers a person copied off a platform (SOP 10.1).

// Record writes one observation from the form.
//
// source is set by the CALLER's entry point, not by the request body: the
// handler passes SourceManual from the form endpoint and SourceCSVImport from
// the import endpoint. A body field would make "where did this come from"
// something the caller declares, and then it is not a source.
func (s *Store) Record(ctx context.Context, workspaceID, actor string, input MetricInput, source MetricSource) (ManualMetric, error) {
	written, err := s.RecordBatch(ctx, workspaceID, actor, []MetricInput{input}, source)
	if err != nil {
		return ManualMetric{}, err
	}
	return written[0], nil
}

// RecordBatch writes every row or none of them.
//
// All or nothing is the point: a partial write leaves somebody believing all
// forty pasted rows landed, and "how many got in" is the only question this
// feature has to answer. Validation runs over the whole batch first, so the
// refusal names the row before anything is opened.
func (s *Store) RecordBatch(ctx context.Context, workspaceID, actor string, rows []MetricInput, source MetricSource) ([]ManualMetric, error) {
	if s == nil {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "record-metric", ErrInvalid)
		return nil, ErrInvalid
	}
	if err := ValidateMetricSource(string(source)); err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "record-metric", err)
		return nil, err
	}
	if err := ValidateMetricBatch(rows); err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "record-metric", err)
		return nil, err
	}
	if s.Publications == nil {
		s.reportFailure(ctx, workspaceID, actor, "", "record-metric", ErrStorage)
		return nil, ErrStorage
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "record-metric", err)
		return nil, err
	}
	defer tx.Rollback(ctx)

	ctx, err = s.audit(ctx, tx, workspaceID, actor, rows[0].PublicationRecordID, "record-metric")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, "", "record-metric", err)
		return nil, err
	}

	written := make([]ManualMetric, 0, len(rows))
	for index, row := range rows {
		// Inside the fence: the publication record has to still exist and
		// still be this workspace's at the moment the row is written.
		workID, artifactID, versionID, resolveErr := s.Publications.Resolve(ctx, workspaceID, row.PublicationRecordID)
		if resolveErr != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, row.PublicationRecordID, "record-metric", ErrNotFound)
			return nil, ErrNotFound
		}
		_, _ = workID, artifactID

		sampledAt, parseErr := time.Parse(time.RFC3339, row.SampledAt)
		if parseErr != nil {
			_ = tx.Rollback(ctx)
			return nil, FieldError{Field: "sampled_at", Reason: "not an RFC 3339 timestamp", Row: index + 1}
		}
		metric := ManualMetric{
			ManualMetricID:      s.newID(),
			WorkspaceID:         workspaceID,
			PublicationRecordID: row.PublicationRecordID,
			Platform:            row.Platform,
			AccountID:           row.AccountID,
			Metric:              row.Metric,
			// Passed through untouched. A nil here is a NULL column and comes
			// back nil; nothing in this function decides that unknown is 0.
			Value:        row.Value,
			Unit:         row.Unit,
			StatWindow:   row.StatWindow,
			SampledAt:    sampledAt.UTC(),
			RecordedBy:   actor,
			EvidenceNote: row.EvidenceNote,
			SourceType:   source,
			VersionID:    versionID,
		}
		if err = tx.QueryRow(ctx, `INSERT INTO content_manual_metric
			(manual_metric_id, workspace_id, publication_record_id, platform,
			 account_id, metric, value, unit, stat_window, sampled_at,
			 recorded_by, evidence_note, source_type)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			RETURNING created_at`,
			metric.ManualMetricID, workspaceID, metric.PublicationRecordID,
			string(metric.Platform), metric.AccountID, string(metric.Metric),
			metric.Value, metric.Unit, metric.StatWindow, metric.SampledAt,
			actor, metric.EvidenceNote, string(source)).
			Scan(&metric.CreatedAt); err != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, metric.ManualMetricID, "record-metric", err)
			return nil, ErrStorage
		}
		written = append(written, metric)
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "record-metric", err)
		return nil, ErrStorage
	}
	return written, nil
}

// ListMetrics returns the observations of one publication record, or of the
// whole workspace, newest sample first.
//
// It returns rows, not totals. This module aggregates nothing: no sum, no
// average, no ranking. 10.2 is where that belongs, and a "convenience" total
// here would be the first step towards ranking two platforms' incomparable
// numbers against each other.
func (s *Store) ListMetrics(ctx context.Context, workspaceID, actor, publicationRecordID, metric string) ([]ManualMetric, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-metrics", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-metrics", ErrInvalid)
		return nil, ErrInvalid
	}
	if metric != "" {
		if err := ValidateMetric(metric); err != nil {
			s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-metrics", err)
			return nil, err
		}
	}
	rows, err := s.DB.Query(ctx, metricSelect+
		` WHERE workspace_id=$1 AND ($2='' OR publication_record_id=$2)
		    AND ($3='' OR metric=$3)
		  ORDER BY sampled_at DESC, manual_metric_id`,
		workspaceID, publicationRecordID, metric)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-metrics", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	metrics := []ManualMetric{}
	for rows.Next() {
		item, scanErr := scanMetric(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-metrics", scanErr)
			return nil, ErrStorage
		}
		metrics = append(metrics, item)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, publicationRecordID, "list-metrics", rows.Err())
		return nil, ErrStorage
	}
	// The version is resolved per record, on read. It is "" when there is no
	// delivery task behind the publication - a history entry, for one - and
	// that is a real answer, not an error.
	if s.Publications != nil {
		resolved := map[string]string{}
		for i := range metrics {
			id := metrics[i].PublicationRecordID
			version, seen := resolved[id]
			if !seen {
				_, _, version, _ = s.Publications.Resolve(ctx, workspaceID, id)
				resolved[id] = version
			}
			metrics[i].VersionID = version
		}
	}
	return metrics, nil
}

// PendingRegistrations lists the publication records nobody has recorded
// numbers for yet. This is SOP 2's fifth workbench item.
//
// No time comparison anywhere: SOP 3.2's brand-level 反馈观察时点 does not
// exist yet, so "is it due" has no answer, and a hard-coded number of days
// would be a rule the SOP never stated sitting where no operator can see it.
// The query says exactly what the predicate says - published, and no metrics.
func (s *Store) PendingRegistrations(ctx context.Context, workspaceID, actor string) ([]PendingRegistration, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "pending-registrations", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "pending-registrations", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT p.publication_record_id, p.work_id,
		p.artifact_id, p.channel, p.status, p.published_at, p.created_at
		FROM content_publication_record p
		WHERE p.workspace_id=$1
		  AND p.status = ANY($2::text[])
		  AND NOT EXISTS (
		      SELECT 1 FROM content_manual_metric m
		      WHERE m.workspace_id = p.workspace_id
		        AND m.publication_record_id = p.publication_record_id)
		ORDER BY p.created_at DESC, p.publication_record_id`,
		workspaceID, PublishedStatuses)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "pending-registrations", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	pending := []PendingRegistration{}
	for rows.Next() {
		var item PendingRegistration
		var publishedAt *time.Time
		if scanErr := rows.Scan(&item.PublicationRecordID, &item.WorkID,
			&item.ArtifactID, &item.Channel, &item.Status, &publishedAt,
			&item.CreatedAt); scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "pending-registrations", scanErr)
			return nil, ErrStorage
		}
		if publishedAt != nil {
			formatted := publishedAt.UTC().Format(time.RFC3339)
			item.PublishedAt = &formatted
		}
		pending = append(pending, item)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "pending-registrations", rows.Err())
		return nil, ErrStorage
	}
	return pending, nil
}

// MetricCountFor is the other half of the derivation, for one record.
func (s *Store) MetricCountFor(ctx context.Context, workspaceID, publicationRecordID string) (int, error) {
	if s == nil || s.DB == nil {
		return 0, ErrStorage
	}
	var count int
	// This is the module's only count(*), and it counts rows to answer "has
	// anybody recorded anything", not to total up a metric. The guard test
	// allows exactly this one.
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM content_manual_metric
		WHERE workspace_id=$1 AND publication_record_id=$2`,
		workspaceID, publicationRecordID).Scan(&count); err != nil {
		return 0, ErrStorage
	}
	return count, nil
}
