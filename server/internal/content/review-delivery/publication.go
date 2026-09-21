package reviewdelivery

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Publication records (SOP 9.1, 9.2).
//
// Append-only. There is no update endpoint, no delete endpoint, and no UPDATE
// or DELETE statement against content_publication_record anywhere in this
// package - a guard test scans for both, and also for the INSERT, so that an
// empty module cannot satisfy it.
//
// The current status of a piece is the latest row. There is no stored "current
// status": it would be a second truth, and it would disagree with the row
// stream after the first concurrent entry with nothing to notice.

// RecordRequest is one statement about what happened on a platform.
//
// ActorID is deliberately NOT here. It is taken from the session by the caller
// of Record, because it is the one field on this row that must not be
// forgeable.
type RecordRequest struct {
	ArtifactID         string            `json:"artifact_id"`
	DeliveryTaskID     string            `json:"delivery_task_id"`
	Channel            Channel           `json:"channel"`
	Status             PublicationStatus `json:"status"`
	DeclaredBy         string            `json:"declared_by"`
	PageURLOrContentID string            `json:"page_url_or_content_id"`
	ReceiptNote        string            `json:"receipt_note"`
	VerificationNote   string            `json:"verification_note"`
	PublishedAt        *string           `json:"published_at"`
	PlatformAccount    string            `json:"platform_account"`
	PlatformEdited     bool              `json:"platform_edited"`
	EditNote           string            `json:"edit_note"`
	VersionMatch       VersionMatch      `json:"version_match"`
	// VersionID is SOP 3.3's 发布后快照, set when the caller knows which
	// version was published without there being a delivery task to walk. A
	// caller that went through the normal flow leaves it empty and the two
	// hops answer instead.
	VersionID string `json:"version_id"`
	// HistoricalImport is set by the import path. It cannot be un-set later
	// because there is no update path to this table at all.
	HistoricalImport bool `json:"historical_import"`
}

// Record appends one publication record.
//
// Nothing about it reaches a platform. It does not check the link, it does not
// fetch the page, it does not confirm anything: it writes down what a person
// said and how they say they checked it. SOP 9.2 - "系统不保存平台发布密钥，也不
// 提供发布执行接口" - and three guard tests.
func (s *Store) Record(ctx context.Context, workspaceID, actor string, request RecordRequest) (PublicationRecord, error) {
	if s == nil {
		return PublicationRecord{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || request.ArtifactID == "" {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "record-publication", ErrInvalid)
		return PublicationRecord{}, invalidField("artifact_id")
	}
	record := PublicationRecord{
		PublicationRecordID: s.newID(),
		WorkspaceID:         workspaceID,
		ArtifactID:          request.ArtifactID,
		DeliveryTaskID:      request.DeliveryTaskID,
		Channel:             request.Channel,
		Status:              request.Status,
		// From the session, never from the body.
		ActorID:            actor,
		DeclaredBy:         request.DeclaredBy,
		PageURLOrContentID: request.PageURLOrContentID,
		ReceiptNote:        request.ReceiptNote,
		VerificationNote:   request.VerificationNote,
		PlatformAccount:    request.PlatformAccount,
		PlatformEdited:     request.PlatformEdited,
		EditNote:           request.EditNote,
		VersionMatch:       request.VersionMatch,
		VersionID:          request.VersionID,
		HistoricalImport:   request.HistoricalImport,
	}
	if record.VersionMatch == "" {
		// SOP 9.1's own default for "I could not get the full text".
		record.VersionMatch = VersionUnknown
	}
	publishedAt, err := parseOptionalTime(request.PublishedAt)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "record-publication", err)
		return PublicationRecord{}, err
	}
	record.PublishedAt = publishedAt
	if err = ValidatePublicationRecord(record); err != nil {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "record-publication", err)
		return PublicationRecord{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "record-publication", err)
		return PublicationRecord{}, err
	}
	defer tx.Rollback(ctx)

	if s.Artifacts != nil {
		workID, _, resolveErr := s.Artifacts.ResolveVersion(ctx, workspaceID, request.ArtifactID, "")
		if resolveErr != nil {
			_ = tx.Rollback(ctx)
			s.reportFailure(ctx, workspaceID, actor, request.ArtifactID, "record-publication", ErrNotFound)
			return PublicationRecord{}, ErrNotFound
		}
		record.WorkID = workID
	}
	ctx, err = s.audit(ctx, tx, workspaceID, actor, record.PublicationRecordID, "record-publication")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, record.PublicationRecordID, "record-publication", err)
		return PublicationRecord{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, declared_by, page_url_or_content_id, receipt_note,
		 verification_note, published_at, platform_account, platform_edited,
		 edit_note, version_match, version_id, historical_import)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING created_at`,
		record.PublicationRecordID, workspaceID, record.WorkID, record.ArtifactID,
		record.DeliveryTaskID, string(record.Channel), string(record.Status),
		record.ActorID, record.DeclaredBy, record.PageURLOrContentID, record.ReceiptNote,
		record.VerificationNote, record.PublishedAt, record.PlatformAccount,
		record.PlatformEdited, record.EditNote, string(record.VersionMatch),
		record.VersionID, record.HistoricalImport).
		Scan(&record.CreatedAt); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, record.PublicationRecordID, "record-publication", err)
		return PublicationRecord{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, record.PublicationRecordID, "record-publication", err)
		return PublicationRecord{}, ErrStorage
	}
	return record, nil
}

// ListPublications returns the records of one document, or of the workspace,
// newest first. The first element is the current status.
func (s *Store) ListPublications(ctx context.Context, workspaceID, actor, artifactID string) ([]PublicationRecord, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-publications", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-publications", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, publicationSelect+
		` WHERE workspace_id=$1 AND ($2='' OR artifact_id=$2)
		  ORDER BY created_at DESC, publication_record_id`, workspaceID, artifactID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-publications", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	records := []PublicationRecord{}
	for rows.Next() {
		record, scanErr := scanPublication(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "list-publications", scanErr)
			return nil, ErrStorage
		}
		records = append(records, record)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "list-publications", rows.Err())
		return nil, ErrStorage
	}
	return records, nil
}

// CurrentPublication returns the latest record for a document, if any.
//
// This is the whole of "current publication status": a query, not a column.
func (s *Store) CurrentPublication(ctx context.Context, workspaceID, actor, artifactID string) (PublicationRecord, bool, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, artifactID, "current-publication", ErrStorage)
		}
		return PublicationRecord{}, false, ErrStorage
	}
	if workspaceID == "" || actor == "" || artifactID == "" {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "current-publication", ErrInvalid)
		return PublicationRecord{}, false, ErrInvalid
	}
	record, err := scanPublication(s.DB.QueryRow(ctx, publicationSelect+
		` WHERE workspace_id=$1 AND artifact_id=$2
		  ORDER BY created_at DESC, publication_record_id LIMIT 1`, workspaceID, artifactID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicationRecord{}, false, nil
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, artifactID, "current-publication", err)
		return PublicationRecord{}, false, ErrStorage
	}
	return record, true, nil
}

// parseOptionalTime reads an optional RFC 3339 timestamp from the request.
//
// A malformed one is named rather than silently dropped: someone typing when
// the piece went out deserves to be told the format was wrong, not to find the
// field empty afterwards.
func parseOptionalTime(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, FieldError{Field: "published_at", Reason: "not an RFC 3339 timestamp"}
	}
	utc := parsed.UTC()
	return &utc, nil
}
