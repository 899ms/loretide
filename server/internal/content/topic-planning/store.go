package topicplanning

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	"go.opentelemetry.io/otel/trace"
)

type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type DiagnosticStore interface {
	AuditTx(context.Context, pgx.Tx, diagnostics.Scope, diagnostics.Event) error
	Technical(context.Context, diagnostics.Event)
}

type AccountReader interface {
	Get(context.Context, string, string) (ipprofile.Account, error)
	// CurrentPersonaRevision is what a start pins: the revision id becomes the
	// snapshot's persona_ref, and the expression profile on it is what the
	// minimum start condition is judged from (EP-04b). Read through the same
	// interface as Get so a test can hand over one fake, not two.
	CurrentPersonaRevision(context.Context, string, string) (ipprofile.Revision, error)
}

type Store struct {
	DB          Database
	Diagnostics DiagnosticStore
	Accounts    AccountReader
	Sources     SourceReader
	// Guard is workspace-core's delete/write fence. Every write transaction
	// takes it as its first statement, so a workspace deletion that has
	// already committed cannot be followed by an orphan card or brief. The
	// audit write takes the same lock, but only on paths that audit; leaning
	// on that would make the fence a side effect of logging and lose it the
	// day a write path stops auditing (Issue #104).
	Guard diagnostics.WorkspaceWriteGuard
	Build string
	NewID func() string
}

func (s *Store) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return diagnostics.NewID()
}

func normalizeStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func encodeStrings(values []string) ([]byte, error) {
	return json.Marshal(normalizeStrings(values))
}

func decodeStrings(raw []byte) ([]string, error) {
	values := []string{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, ErrStorage
	}
	return values, nil
}

// begin opens a write transaction and fences it against workspace deletion.
// The fence is taken before any topic statement, in the same lock order
// DeleteWorkspace uses, so the delete either waits and then sweeps the
// committed rows or commits first and makes this return ErrNotFound. Without a
// guard there is no protocol to honour, so the write fails closed rather than
// running unfenced.
func (s *Store) begin(ctx context.Context, workspaceID string) (pgx.Tx, error) {
	if s == nil || s.DB == nil || s.Diagnostics == nil || s.Guard == nil {
		return nil, ErrStorage
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, ErrStorage
	}
	if err = s.Guard.LockForContentDiagnosticWrite(ctx, tx, workspaceID); err != nil {
		_ = tx.Rollback(ctx)
		// A workspace that is gone is reported exactly as a foreign one, so the
		// boundary answers 404 and the response cannot be used to tell a
		// deleted workspace from one the caller never had.
		if errors.Is(err, diagnostics.ErrDenied) {
			return nil, ErrNotFound
		}
		return nil, ErrStorage
	}
	return tx, nil
}

func (s *Store) event(ctx context.Context, workspaceID, actor, objectID, step string) (context.Context, diagnostics.Event) {
	child, parent := diagnostics.Child(ctx)
	span := trace.SpanContextFromContext(child)
	// Built unsanitized on purpose. Sanitize is the sink's job - AuditTx and
	// Store.Technical each apply it - and running it here as well would do two
	// things wrong: it blanks Component to "unknown" before the event has left
	// the module (the allowlist has no content-module name in it), and it
	// derives Message/Next/Retryable from a Code that reportFailure has not set
	// yet. ip-profile and workspace-core hand their events over unsanitized for
	// the same reason.
	return child, diagnostics.Event{
		ID:         s.newID(),
		Workspace:  workspaceID,
		Actor:      actor,
		ActorKind:  "human",
		ObjectType: "work",
		ObjectID:   objectID,
		Action:     "execute",
		Outcome:    "success",
		Trace:      span.TraceID().String(),
		Span:       span.SpanID().String(),
		Parent:     parent,
		Step:       step,
		Component:  "topic-planning",
		Severity:   "info",
		Build:      s.Build,
		Occurred:   time.Now().UTC(),
	}
}

func (s *Store) audit(ctx context.Context, tx pgx.Tx, workspaceID, actor, objectID, step string) (context.Context, error) {
	child, event := s.event(ctx, workspaceID, actor, objectID, step)
	err := s.Diagnostics.AuditTx(child, tx, diagnostics.Scope{Workspace: workspaceID, Actor: actor}, event)
	return child, err
}

func (s *Store) reportFailure(ctx context.Context, workspaceID, actor, objectID, step string, err error) {
	if s == nil || s.Diagnostics == nil {
		return
	}
	child, event := s.event(ctx, workspaceID, actor, objectID, step)
	event.Outcome = "failed"
	event.Severity = "error"
	event.Code = "DATABASE_UNAVAILABLE"
	if errors.Is(err, ErrInvalid) {
		event.Code = "INPUT_CONFLICT"
		event.Severity = "warn"
	}
	if errors.Is(err, ErrNotFound) {
		event.Code = "INPUT_CONFLICT"
		event.Severity = "warn"
	}
	s.Diagnostics.Technical(child, event)
}

func (s *Store) Create(ctx context.Context, actor string, card TopicCard) (TopicCard, error) {
	if s == nil {
		return TopicCard{}, ErrStorage
	}
	if actor == "" || card.WorkspaceID == "" {
		s.reportFailure(ctx, card.WorkspaceID, actor, "", "create", ErrInvalid)
		return TopicCard{}, ErrInvalid
	}
	if err := s.checkAccount(ctx, card.WorkspaceID, card.AccountID); err != nil {
		s.reportFailure(ctx, card.WorkspaceID, actor, "", "create", err)
		return TopicCard{}, err
	}
	card.TopicCardID = s.newID()
	card.Status = StatusDraft
	card.Channels = normalizeStrings(card.Channels)
	card.DecisionReason = ""
	card.DecisionNote = ""
	card.StartedBriefRevisionID = nil
	channels, err := encodeStrings(card.Channels)
	if err != nil {
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", ErrInvalid)
		return TopicCard{}, ErrInvalid
	}

	fitSources, err := NormalizeSourceIDs("fit_source_ids", card.FitSourceIDs)
	if err != nil {
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", err)
		return TopicCard{}, err
	}
	evidenceSources, err := NormalizeSourceIDs("evidence_source_ids", card.EvidenceSourceIDs)
	if err != nil {
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", err)
		return TopicCard{}, err
	}
	card.FitSourceIDs = fitSources
	card.EvidenceSourceIDs = evidenceSources
	encodedFit, err := encodeStrings(card.FitSourceIDs)
	if err != nil {
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", ErrInvalid)
		return TopicCard{}, ErrInvalid
	}
	encodedEvidence, err := encodeStrings(card.EvidenceSourceIDs)
	if err != nil {
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", ErrInvalid)
		return TopicCard{}, ErrInvalid
	}

	tx, err := s.begin(ctx, card.WorkspaceID)
	if err != nil {
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", err)
		return TopicCard{}, err
	}
	defer tx.Rollback(ctx)
	ctx, err = s.audit(ctx, tx, card.WorkspaceID, actor, card.TopicCardID, "create")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", err)
		return TopicCard{}, err
	}
	if err = s.checkSources(ctx, card.WorkspaceID, card.FitSourceIDs, card.EvidenceSourceIDs); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", err)
		return TopicCard{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO content_topic_card (
			topic_card_id, workspace_id, account_id, audience_problem_judgment,
			ip_fit, timing, existing_content_relation, evidence_gaps_and_investment,
			channels, fit_source_ids, evidence_source_ids, recommended_action, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING created_at, updated_at`,
		card.TopicCardID, card.WorkspaceID, card.AccountID,
		card.AudienceProblemJudgment, card.IPFit, card.Timing,
		card.ExistingContentRelation, card.EvidenceGapsAndInvestment,
		channels, encodedFit, encodedEvidence, card.RecommendedAction, card.Status,
	).Scan(&card.CreatedAt, &card.UpdatedAt)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", err)
		return TopicCard{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, card.WorkspaceID, actor, card.TopicCardID, "create", err)
		return TopicCard{}, ErrStorage
	}
	return card, nil
}

// checkAccount refuses an account that is not this brand's.
//
// Nil means "no account", which is a legitimate state: a card may be written
// before anyone decides which account publishes it. A foreign or missing
// account is ErrNotFound, the same answer a foreign card gets, so a refusal
// cannot be used to find out which account ids exist.
func (s *Store) checkAccount(ctx context.Context, workspaceID string, accountID *string) error {
	if accountID == nil {
		return nil
	}
	if s == nil || s.Accounts == nil {
		return ErrStorage
	}
	if _, err := s.Accounts.Get(ctx, workspaceID, *accountID); err != nil {
		if errors.Is(err, ipprofile.ErrNotFound) {
			return ErrNotFound
		}
		return ErrStorage
	}
	return nil
}

// checkSources refuses any source id that is not this brand's or does not exist.
//
// Empty list means no source references to validate. If source IDs are present,
// Sources must be non-nil. A foreign or missing source is ErrNotFound, the same
// answer a foreign card gets, so a refusal cannot be used to find out which source
// ids exist across workspaces.
func (s *Store) checkSources(ctx context.Context, workspaceID string, fitIDs, evidenceIDs []string) error {
	allCount := len(fitIDs) + len(evidenceIDs)
	if allCount == 0 {
		return nil
	}
	if s == nil || s.Sources == nil {
		return ErrStorage
	}
	seen := make(map[string]struct{}, allCount)
	for _, id := range fitIDs {
		seen[id] = struct{}{}
	}
	for _, id := range evidenceIDs {
		seen[id] = struct{}{}
	}
	for id := range seen {
		exists, err := s.Sources.Exists(ctx, workspaceID, id)
		if err != nil {
			return ErrStorage
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

// SetAccount attaches the card to an account, or detaches it when accountID is
// nil. Its own entry point rather than a field on the four decision actions:
// those change what happens to the card, this changes who it is written for,
// and folding them together would make "save" able to silently re-target a
// card (SOP 5.2 asks the card to say why it fits THIS IP).
//
// The whole thing runs inside the workspace delete fence, the brand check
// included: a check taken outside it could be answered before the workspace is
// deleted and the write applied after.
func (s *Store) SetAccount(ctx context.Context, workspaceID, actor, topicCardID string, accountID *string) (TopicCard, error) {
	if workspaceID == "" || actor == "" || topicCardID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-account", ErrInvalid)
		return TopicCard{}, ErrInvalid
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-account", err)
		return TopicCard{}, err
	}
	defer tx.Rollback(ctx)
	ctx, err = s.audit(ctx, tx, workspaceID, actor, topicCardID, "link-account")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-account", err)
		return TopicCard{}, err
	}
	if err = s.checkAccount(ctx, workspaceID, accountID); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-account", err)
		return TopicCard{}, err
	}
	card, err := scanTopicCard(tx.QueryRow(ctx, topicCardSelect+` WHERE workspace_id=$1 AND topic_card_id=$2 FOR UPDATE`, workspaceID, topicCardID))
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-account", err)
		return TopicCard{}, err
	}
	err = tx.QueryRow(ctx, `
		UPDATE content_topic_card SET account_id=$3, updated_at=now()
		WHERE workspace_id=$1 AND topic_card_id=$2
		RETURNING updated_at`, workspaceID, topicCardID, accountID).Scan(&card.UpdatedAt)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-account", err)
		return TopicCard{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-account", err)
		return TopicCard{}, ErrStorage
	}
	card.AccountID = accountID
	return card, nil
}

// SetSources sets or clears the source references linked to a topic card.
//
// Its own entry point rather than a field on the four decision actions: those
// change what happens to the card, this changes the materials that justify it
// or provide evidence for it.
//
// Whole-column replacement: a provided slice replaces the column.
// Absence != clear: nil input means "do not change this column". Clearing a column
// requires an explicit empty slice.
//
// Runs inside the workspace delete fence, source validation included: a check taken
// outside could be answered before the workspace is deleted and the write applied after.
//
// Does NOT touch status, decision_reason, decision_note, started_brief_revision_id,
// account_id, or any of the seven free-text fields.
func (s *Store) SetSources(ctx context.Context, workspaceID, actor, topicCardID string, fitSourceIDs, evidenceSourceIDs *[]string) (TopicCard, error) {
	if workspaceID == "" || actor == "" || topicCardID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", ErrInvalid)
		return TopicCard{}, ErrInvalid
	}

	var cleanedFit, cleanedEvidence []string
	var err error
	if fitSourceIDs != nil {
		cleanedFit, err = NormalizeSourceIDs("fit_source_ids", *fitSourceIDs)
		if err != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
			return TopicCard{}, err
		}
	}
	if evidenceSourceIDs != nil {
		cleanedEvidence, err = NormalizeSourceIDs("evidence_source_ids", *evidenceSourceIDs)
		if err != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
			return TopicCard{}, err
		}
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
		return TopicCard{}, err
	}
	defer tx.Rollback(ctx)

	ctx, err = s.audit(ctx, tx, workspaceID, actor, topicCardID, "link-sources")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
		return TopicCard{}, err
	}

	checkFit := cleanedFit
	if fitSourceIDs == nil {
		checkFit = nil
	}
	checkEvidence := cleanedEvidence
	if evidenceSourceIDs == nil {
		checkEvidence = nil
	}
	if err = s.checkSources(ctx, workspaceID, checkFit, checkEvidence); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
		return TopicCard{}, err
	}

	card, err := scanTopicCard(tx.QueryRow(ctx, topicCardSelect+` WHERE workspace_id=$1 AND topic_card_id=$2 FOR UPDATE`, workspaceID, topicCardID))
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
		return TopicCard{}, err
	}

	if fitSourceIDs != nil {
		card.FitSourceIDs = cleanedFit
	}
	if evidenceSourceIDs != nil {
		card.EvidenceSourceIDs = cleanedEvidence
	}

	encodedFit, err := encodeStrings(card.FitSourceIDs)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
		return TopicCard{}, ErrStorage
	}
	encodedEvidence, err := encodeStrings(card.EvidenceSourceIDs)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
		return TopicCard{}, ErrStorage
	}

	err = tx.QueryRow(ctx, `
		UPDATE content_topic_card
		SET fit_source_ids=$3, evidence_source_ids=$4, updated_at=now()
		WHERE workspace_id=$1 AND topic_card_id=$2
		RETURNING updated_at`, workspaceID, topicCardID, encodedFit, encodedEvidence).Scan(&card.UpdatedAt)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
		return TopicCard{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "link-sources", err)
		return TopicCard{}, ErrStorage
	}
	return card, nil
}

func (s *Store) Get(ctx context.Context, workspaceID, actor, topicCardID string) (TopicCard, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "get", ErrStorage)
		}
		return TopicCard{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || topicCardID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "get", ErrInvalid)
		return TopicCard{}, ErrInvalid
	}
	card, err := scanTopicCard(s.DB.QueryRow(ctx, topicCardSelect+` WHERE workspace_id=$1 AND topic_card_id=$2`, workspaceID, topicCardID))
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "get", err)
	}
	return card, err
}

// List returns the brand's cards, optionally narrowed to one account.
//
// accountFilter is empty for every card, AccountFilterNone for the cards no
// account has been chosen for, and an account id otherwise. The filter is part
// of the query rather than something the caller drops afterwards: a page that
// filters after reading would still have carried every card across the network.
func (s *Store) List(ctx context.Context, workspaceID, actor, accountFilter string) ([]TopicCard, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "list", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "list", ErrInvalid)
		return nil, ErrInvalid
	}
	where, args := ` WHERE workspace_id=$1`, []any{workspaceID}
	switch accountFilter {
	case "":
	case AccountFilterNone:
		where += ` AND account_id IS NULL`
	default:
		where += ` AND account_id=$2`
		args = append(args, accountFilter)
	}
	rows, err := s.DB.Query(ctx, topicCardSelect+where+` ORDER BY created_at DESC, topic_card_id`, args...)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "list", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	cards := []TopicCard{}
	for rows.Next() {
		card, scanErr := scanTopicCard(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "list", scanErr)
			return nil, scanErr
		}
		cards = append(cards, card)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "list", rows.Err())
		return nil, ErrStorage
	}
	return cards, nil
}

func (s *Store) Act(ctx context.Context, workspaceID, actor, topicCardID string, req ActionRequest) (ActionResult, error) {
	status, ok := req.Action.Status()
	if !ok || workspaceID == "" || actor == "" || topicCardID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, string(req.Action), ErrInvalid)
		return ActionResult{}, ErrInvalid
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, string(req.Action), err)
		return ActionResult{}, err
	}
	defer tx.Rollback(ctx)
	ctx, err = s.audit(ctx, tx, workspaceID, actor, topicCardID, string(req.Action))
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, string(req.Action), err)
		return ActionResult{}, err
	}

	card, err := scanTopicCard(tx.QueryRow(ctx, topicCardSelect+` WHERE workspace_id=$1 AND topic_card_id=$2 FOR UPDATE`, workspaceID, topicCardID))
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, string(req.Action), err)
		return ActionResult{}, err
	}
	if status != StatusDeferred && status != StatusDropped {
		req.Reason, req.Note = "", ""
	}

	var brief *BriefRevision
	if req.Action == ActionStart {
		if card.StartedBriefRevisionID == nil {
			created, createErr := insertBrief(ctx, tx, s.newID(), workspaceID, topicCardID, 1, req.Brief)
			if createErr != nil {
				_ = tx.Rollback(ctx)
				s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", createErr)
				return ActionResult{}, createErr
			}
			brief = &created
			card.StartedBriefRevisionID = &created.BriefRevisionID
		} else {
			existing, getErr := scanBrief(tx.QueryRow(ctx, briefSelect+` WHERE workspace_id=$1 AND topic_card_id=$2 AND brief_revision_id=$3`, workspaceID, topicCardID, *card.StartedBriefRevisionID))
			if getErr != nil {
				_ = tx.Rollback(ctx)
				s.reportFailure(ctx, workspaceID, actor, topicCardID, "start", getErr)
				return ActionResult{}, getErr
			}
			brief = &existing
		}
	}

	card.Status = status
	card.DecisionReason = req.Reason
	card.DecisionNote = req.Note
	err = tx.QueryRow(ctx, `
		UPDATE content_topic_card
		SET status=$3, decision_reason=$4, decision_note=$5,
			started_brief_revision_id=$6, updated_at=now()
		WHERE workspace_id=$1 AND topic_card_id=$2
		RETURNING updated_at`, workspaceID, topicCardID, status, req.Reason, req.Note,
		card.StartedBriefRevisionID).Scan(&card.UpdatedAt)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, string(req.Action), err)
		return ActionResult{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, string(req.Action), err)
		return ActionResult{}, ErrStorage
	}
	return ActionResult{TopicCard: card, Brief: brief}, nil
}

func (s *Store) AppendBrief(ctx context.Context, workspaceID, actor, topicCardID string, input BriefRevision) (BriefRevision, error) {
	if workspaceID == "" || actor == "" || topicCardID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", ErrInvalid)
		return BriefRevision{}, ErrInvalid
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", err)
		return BriefRevision{}, err
	}
	defer tx.Rollback(ctx)
	ctx, err = s.audit(ctx, tx, workspaceID, actor, topicCardID, "append-brief")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", err)
		return BriefRevision{}, err
	}

	// Lock the card first, number the revision second, in two statements.
	// READ COMMITTED gives a statement one snapshot, taken when the statement
	// starts, and waiting for a row lock does not refresh it. Numbering inside
	// the locking statement therefore counts the revisions as they were before
	// the appender that won the lock committed, picks a number that appender
	// already used, and turns a legitimate save into a unique-index failure the
	// caller sees as 503 (Issue #109). The second statement starts after the
	// lock is granted, so its snapshot includes whatever the winner wrote.
	var startedBriefRevisionID string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(started_brief_revision_id, '')
		FROM content_topic_card
		WHERE workspace_id=$1 AND topic_card_id=$2
		FOR UPDATE`, workspaceID, topicCardID).Scan(&startedBriefRevisionID)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", ErrNotFound)
		return BriefRevision{}, ErrNotFound
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", err)
		return BriefRevision{}, ErrStorage
	}
	if startedBriefRevisionID == "" {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", ErrInvalid)
		return BriefRevision{}, ErrInvalid
	}
	var next int64
	if err = tx.QueryRow(ctx, `
		SELECT COALESCE(max(revision), 0) + 1 FROM content_brief_revision
		WHERE workspace_id=$1 AND topic_card_id=$2`,
		workspaceID, topicCardID).Scan(&next); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", err)
		return BriefRevision{}, ErrStorage
	}
	brief, err := insertBrief(ctx, tx, s.newID(), workspaceID, topicCardID, next, input)
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", err)
		return BriefRevision{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "append-brief", err)
		return BriefRevision{}, ErrStorage
	}
	return brief, nil
}

func (s *Store) ListBriefs(ctx context.Context, workspaceID, actor, topicCardID string) ([]BriefRevision, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-briefs", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || topicCardID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-briefs", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, briefSelect+` WHERE workspace_id=$1 AND topic_card_id=$2 ORDER BY revision`, workspaceID, topicCardID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-briefs", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	briefs := []BriefRevision{}
	for rows.Next() {
		brief, scanErr := scanBrief(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-briefs", scanErr)
			return nil, scanErr
		}
		briefs = append(briefs, brief)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "list-briefs", rows.Err())
		return nil, ErrStorage
	}
	return briefs, nil
}

func (s *Store) GetBrief(ctx context.Context, workspaceID, actor, topicCardID, revisionID string) (BriefRevision, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, topicCardID, "get-brief", ErrStorage)
		}
		return BriefRevision{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || topicCardID == "" || revisionID == "" {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "get-brief", ErrInvalid)
		return BriefRevision{}, ErrInvalid
	}
	brief, err := scanBrief(s.DB.QueryRow(ctx, briefSelect+` WHERE workspace_id=$1 AND topic_card_id=$2 AND brief_revision_id=$3`, workspaceID, topicCardID, revisionID))
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, topicCardID, "get-brief", err)
	}
	return brief, err
}

const topicCardSelect = `SELECT topic_card_id, workspace_id, account_id,
	audience_problem_judgment, ip_fit, timing, existing_content_relation,
	evidence_gaps_and_investment, channels, fit_source_ids, evidence_source_ids,
	recommended_action, status, decision_reason, decision_note,
	started_brief_revision_id, created_at, updated_at
	FROM content_topic_card`

type scanner interface{ Scan(...any) error }

func scanTopicCard(row scanner) (TopicCard, error) {
	var card TopicCard
	var channels, fitSources, evidenceSources []byte
	err := row.Scan(&card.TopicCardID, &card.WorkspaceID, &card.AccountID,
		&card.AudienceProblemJudgment, &card.IPFit, &card.Timing,
		&card.ExistingContentRelation, &card.EvidenceGapsAndInvestment,
		&channels, &fitSources, &evidenceSources, &card.RecommendedAction,
		&card.Status, &card.DecisionReason, &card.DecisionNote,
		&card.StartedBriefRevisionID, &card.CreatedAt, &card.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TopicCard{}, ErrNotFound
	}
	if err != nil {
		return TopicCard{}, ErrStorage
	}
	card.Channels, err = decodeStrings(channels)
	if err != nil {
		return TopicCard{}, err
	}
	card.FitSourceIDs, err = decodeStrings(fitSources)
	if err != nil {
		return TopicCard{}, err
	}
	card.EvidenceSourceIDs, err = decodeStrings(evidenceSources)
	if err != nil {
		return TopicCard{}, err
	}
	return card, nil
}

const briefSelect = `SELECT brief_revision_id, topic_card_id, workspace_id,
	revision, audience, core_problem, claim_and_boundaries, channels, format,
	structure, citation_requirements, source_scope, deliverable, time_limit,
	cost_limit, created_at FROM content_brief_revision`

func scanBrief(row scanner) (BriefRevision, error) {
	var brief BriefRevision
	var channels []byte
	err := row.Scan(&brief.BriefRevisionID, &brief.TopicCardID, &brief.WorkspaceID,
		&brief.Revision, &brief.Audience, &brief.CoreProblem, &brief.ClaimAndBoundaries,
		&channels, &brief.Format, &brief.Structure, &brief.CitationRequirements,
		&brief.SourceScope, &brief.Deliverable, &brief.TimeLimit, &brief.CostLimit,
		&brief.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BriefRevision{}, ErrNotFound
	}
	if err != nil {
		return BriefRevision{}, ErrStorage
	}
	brief.Channels, err = decodeStrings(channels)
	if err != nil {
		return BriefRevision{}, err
	}
	return brief, nil
}

func insertBrief(ctx context.Context, tx pgx.Tx, revisionID, workspaceID, topicCardID string, revision int64, input BriefRevision) (BriefRevision, error) {
	channels, err := encodeStrings(input.Channels)
	if err != nil {
		return BriefRevision{}, ErrInvalid
	}
	brief := input
	brief.BriefRevisionID = revisionID
	brief.WorkspaceID = workspaceID
	brief.TopicCardID = topicCardID
	brief.Revision = revision
	brief.Channels = normalizeStrings(input.Channels)
	err = tx.QueryRow(ctx, `
		INSERT INTO content_brief_revision (
			brief_revision_id, topic_card_id, workspace_id, revision, audience,
			core_problem, claim_and_boundaries, channels, format, structure,
			citation_requirements, source_scope, deliverable, time_limit, cost_limit
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING created_at`, revisionID, topicCardID, workspaceID, revision,
		input.Audience, input.CoreProblem, input.ClaimAndBoundaries, channels,
		input.Format, input.Structure, input.CitationRequirements, input.SourceScope,
		input.Deliverable, input.TimeLimit, input.CostLimit).Scan(&brief.CreatedAt)
	if err != nil {
		return BriefRevision{}, ErrStorage
	}
	return brief, nil
}
