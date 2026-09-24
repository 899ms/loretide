package topicplanning

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Marketing node storage (specs/033 PR 1): create, import, revise, confirm,
// cancel, read, list and history.
//
// Every write follows the same order, the one SetAccount documents:
//
//  1. shape checks, outside any transaction (pure);
//  2. begin, whose first statement is the workspace delete fence;
//  3. the audit row, in the same transaction;
//  4. the brand checks on accounts and materials, inside the fence (FR-044) -
//     a check answered before the fence could be answered before a workspace
//     deletion and the write applied after it;
//  5. for an existing node: its row FOR UPDATE, then base_revision;
//  6. insert the next revision, then move the node pointer.
//
// The revision table is insert-only: there is no UPDATE or DELETE of it in
// this package, and marketing_node_guards_test.go fails the build if one
// appears.

func (s *Store) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// reportNodeFailure records a failed node operation. A revision conflict is
// the caller's input racing someone else's, not a storage failure, so it is
// logged with the input-conflict code rather than as the database being down.
func (s *Store) reportNodeFailure(ctx context.Context, workspaceID, actor, objectID, step string, err error) {
	if errors.Is(err, ErrConflict) {
		err = ErrInvalid
	}
	s.reportFailure(ctx, workspaceID, actor, objectID, step, err)
}

// checkNodeReferences refuses an account or a material that is not this
// brand's, with the same ErrNotFound a foreign node gets. It must run inside
// the write transaction's fence.
func (s *Store) checkNodeReferences(ctx context.Context, workspaceID string, content NodeContent) (string, error) {
	for _, account := range content.Accounts {
		id := account.AccountID
		if err := s.checkAccount(ctx, workspaceID, &id); err != nil {
			return "accounts", err
		}
	}
	if err := s.checkSources(ctx, workspaceID, content.MaterialSourceIDs, nil); err != nil {
		return "material_source_ids", err
	}
	return "", nil
}

func checkNote(note string) error {
	if tooLong(note, MaxNodeNoteLength) {
		return FieldError{Field: "note", Reason: "too long"}
	}
	return nil
}

// CreateNode records a node a person entered by hand: active, origin manual,
// revision 1 of kind create.
func (s *Store) CreateNode(ctx context.Context, workspaceID, actor string, req CreateNodeRequest) (MarketingNode, error) {
	const step = "create-marketing-node"
	if s == nil {
		return MarketingNode{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportNodeFailure(ctx, workspaceID, actor, "", step, ErrInvalid)
		return MarketingNode{}, ErrInvalid
	}
	content, err := NormalizeNodeContent(req.Content)
	if err == nil {
		err = checkNote(req.Note)
	}
	if err != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, "", step, err)
		return MarketingNode{}, err
	}
	nodeID := s.newID()
	fail := func(err error) (MarketingNode, error) {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
		return MarketingNode{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if ctx, err = s.audit(ctx, tx, workspaceID, actor, nodeID, step); err != nil {
		return fail(err)
	}
	if _, err = s.checkNodeReferences(ctx, workspaceID, content); err != nil {
		return fail(err)
	}
	node, err := s.insertNode(ctx, tx, workspaceID, actor, nodeID, NodeStatusActive, NodeOriginManual, content, req.Note)
	if err != nil {
		return fail(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	return s.withTiming(node)
}

// ImportNodes creates one unconfirmed, origin-import node per valid,
// non-duplicate row, and reports every row (FR-008, FR-009). A row that is
// malformed, refers to a foreign account or material, or repeats a node is
// reported and skipped; it does not fail the others. A storage failure does
// fail the whole import, so nothing is half-written.
func (s *Store) ImportNodes(ctx context.Context, workspaceID, actor string, rows []ImportRow) (ImportResult, error) {
	const step = "import-marketing-nodes"
	if s == nil {
		return ImportResult{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || len(rows) == 0 || len(rows) > MaxNodeImportRows {
		s.reportNodeFailure(ctx, workspaceID, actor, "", step, ErrInvalid)
		return ImportResult{}, ErrInvalid
	}
	fail := func(err error) (ImportResult, error) {
		s.reportNodeFailure(ctx, workspaceID, actor, "", step, err)
		return ImportResult{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if ctx, err = s.audit(ctx, tx, workspaceID, actor, "", step); err != nil {
		return fail(err)
	}

	results := make([]ImportRowResult, len(rows))
	// Rows created earlier in this same batch, keyed like the stored check.
	batch := map[string]string{}
	for i, row := range rows {
		result := ImportRowResult{Row: i + 1}
		if row.Err != nil {
			result.Outcome, result.Field = ImportInvalid, invalidField(row.Err)
			results[i] = result
			continue
		}
		field, refErr := s.checkNodeReferences(ctx, workspaceID, row.Content)
		if errors.Is(refErr, ErrNotFound) {
			result.Outcome, result.Field = ImportInvalid, field
			results[i] = result
			continue
		}
		if refErr != nil {
			return fail(refErr)
		}
		key := row.Content.Name + "\x00" + row.Content.StartsOn
		if existing, dup := batch[key]; dup {
			result.Outcome, result.NodeID = ImportDuplicate, existing
			results[i] = result
			continue
		}
		existing, found, err := findDuplicateNode(ctx, tx, workspaceID, row.Content)
		if err != nil {
			return fail(err)
		}
		if found {
			result.Outcome, result.NodeID = ImportDuplicate, existing
			results[i] = result
			continue
		}
		nodeID := s.newID()
		if _, err = s.insertNode(ctx, tx, workspaceID, actor, nodeID, NodeStatusUnconfirmed, NodeOriginImport, row.Content, ""); err != nil {
			return fail(err)
		}
		batch[key] = nodeID
		result.Outcome, result.NodeID = ImportCreated, nodeID
		results[i] = result
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	return ImportResult{Results: results}, nil
}

func invalidField(err error) string {
	var fieldErr FieldError
	if errors.As(err, &fieldErr) {
		return fieldErr.Field
	}
	return "row"
}

// findDuplicateNode looks for a node of this brand, not cancelled, whose
// current revision has the same trimmed name and the same start date. The
// workspace filter is what keeps one brand's import from being judged against
// another brand's nodes (FR-009, SC-009).
func findDuplicateNode(ctx context.Context, tx pgx.Tx, workspaceID string, content NodeContent) (string, bool, error) {
	var nodeID string
	err := tx.QueryRow(ctx, `
		SELECT n.node_id
		FROM content_marketing_node n
		JOIN content_marketing_node_revision r
		  ON r.workspace_id = n.workspace_id AND r.node_id = n.node_id AND r.revision = n.current_revision
		WHERE n.workspace_id = $1 AND n.status <> 'cancelled'
		  AND btrim(r.name) = btrim($2) AND r.starts_on = $3::date
		ORDER BY n.created_at, n.node_id
		LIMIT 1`, workspaceID, content.Name, content.StartsOn).Scan(&nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, ErrStorage
	}
	return nodeID, true, nil
}

// ReviseNode appends an edit or a reschedule. The whole content is submitted;
// the change kind is decided here from what changed (FR-006).
func (s *Store) ReviseNode(ctx context.Context, workspaceID, actor, nodeID string, req ReviseNodeRequest) (MarketingNode, error) {
	const step = "revise-marketing-node"
	if s == nil {
		return MarketingNode{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" || req.BaseRevision < 1 {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, ErrInvalid)
		return MarketingNode{}, ErrInvalid
	}
	content, err := NormalizeNodeContent(req.Content)
	if err == nil {
		err = checkNote(req.Note)
	}
	if err != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
		return MarketingNode{}, err
	}
	return s.appendRevision(ctx, workspaceID, actor, nodeID, step, req.BaseRevision,
		func(ctx context.Context, status NodeStatus, current NodeRevision) (NodeStatus, ChangeKind, NodeContent, error) {
			if status == NodeStatusCancelled {
				return "", "", NodeContent{}, FieldError{Field: "status", Reason: "node is cancelled"}
			}
			kind, err := ClassifyChange(current.NodeContent, content)
			if err != nil {
				return "", "", NodeContent{}, err
			}
			if _, err = s.checkNodeReferences(ctx, workspaceID, content); err != nil {
				return "", "", NodeContent{}, err
			}
			return status, kind, content, nil
		}, req.Note)
}

// ConfirmNode moves an unconfirmed (imported) node to active. It changes the
// status only: the content, date_certainty included, is carried over as it
// was (FR-010).
func (s *Store) ConfirmNode(ctx context.Context, workspaceID, actor, nodeID string, req TransitionRequest) (MarketingNode, error) {
	const step = "confirm-marketing-node"
	return s.transition(ctx, workspaceID, actor, nodeID, step, req,
		func(status NodeStatus) (NodeStatus, ChangeKind, error) {
			if status != NodeStatusUnconfirmed {
				return "", "", FieldError{Field: "status", Reason: "only an unconfirmed node can be confirmed"}
			}
			return NodeStatusActive, ChangeConfirm, nil
		})
}

// CancelNode ends a node. Cancelled is terminal (FR-003, FR-037).
func (s *Store) CancelNode(ctx context.Context, workspaceID, actor, nodeID string, req TransitionRequest) (MarketingNode, error) {
	const step = "cancel-marketing-node"
	return s.transition(ctx, workspaceID, actor, nodeID, step, req,
		func(status NodeStatus) (NodeStatus, ChangeKind, error) {
			if status == NodeStatusCancelled {
				return "", "", FieldError{Field: "status", Reason: "node is already cancelled"}
			}
			return NodeStatusCancelled, ChangeCancel, nil
		})
}

func (s *Store) transition(ctx context.Context, workspaceID, actor, nodeID, step string, req TransitionRequest,
	next func(NodeStatus) (NodeStatus, ChangeKind, error)) (MarketingNode, error) {
	if s == nil {
		return MarketingNode{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" || req.BaseRevision < 1 {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, ErrInvalid)
		return MarketingNode{}, ErrInvalid
	}
	if err := checkNote(req.Note); err != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
		return MarketingNode{}, err
	}
	return s.appendRevision(ctx, workspaceID, actor, nodeID, step, req.BaseRevision,
		func(_ context.Context, status NodeStatus, current NodeRevision) (NodeStatus, ChangeKind, NodeContent, error) {
			after, kind, err := next(status)
			return after, kind, current.NodeContent, err
		}, req.Note)
}

// decideRevision inspects the locked node and says what the next revision
// is: the status after it, its change kind and its full content.
type decideRevision func(ctx context.Context, status NodeStatus, current NodeRevision) (NodeStatus, ChangeKind, NodeContent, error)

// appendRevision is the shared body of revise, confirm and cancel: fence,
// audit, lock the node row, compare base_revision, let decide judge, insert
// revision current+1, move the pointer.
func (s *Store) appendRevision(ctx context.Context, workspaceID, actor, nodeID, step string, baseRevision int64, decide decideRevision, note string) (MarketingNode, error) {
	fail := func(err error) (MarketingNode, error) {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
		return MarketingNode{}, err
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if ctx, err = s.audit(ctx, tx, workspaceID, actor, nodeID, step); err != nil {
		return fail(err)
	}

	var node MarketingNode
	err = tx.QueryRow(ctx, `
		SELECT node_id, workspace_id, status, origin, current_revision, created_at
		FROM content_marketing_node
		WHERE workspace_id = $1 AND node_id = $2
		FOR UPDATE`, workspaceID, nodeID).
		Scan(&node.NodeID, &node.WorkspaceID, &node.Status, &node.Origin, &node.CurrentRevision, &node.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(ErrNotFound)
	}
	if err != nil {
		return fail(ErrStorage)
	}
	if node.CurrentRevision != baseRevision {
		return fail(ErrConflict)
	}
	current, err := scanNodeRevision(tx.QueryRow(ctx, nodeRevisionSelect+`
		WHERE r.workspace_id = $1 AND r.node_id = $2 AND r.revision = $3`,
		workspaceID, nodeID, node.CurrentRevision))
	if err != nil {
		return fail(err)
	}
	statusAfter, kind, content, err := decide(ctx, node.Status, current)
	if err != nil {
		return fail(err)
	}
	revision, err := s.insertRevision(ctx, tx, workspaceID, actor, nodeID, node.CurrentRevision+1, kind, statusAfter, content, note)
	if err != nil {
		return fail(err)
	}
	err = tx.QueryRow(ctx, `
		UPDATE content_marketing_node
		SET status = $3, current_revision = $4, updated_at = now()
		WHERE workspace_id = $1 AND node_id = $2
		RETURNING updated_at`, workspaceID, nodeID, statusAfter, revision.Revision).Scan(&node.UpdatedAt)
	if err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	node.Status = statusAfter
	node.CurrentRevision = revision.Revision
	node.Current = revision
	return s.withTiming(node)
}

// insertNode writes the node pointer and its first revision.
func (s *Store) insertNode(ctx context.Context, tx pgx.Tx, workspaceID, actor, nodeID string, status NodeStatus, origin NodeOrigin, content NodeContent, note string) (MarketingNode, error) {
	node := MarketingNode{NodeID: nodeID, WorkspaceID: workspaceID, Status: status, Origin: origin, CurrentRevision: 1}
	err := tx.QueryRow(ctx, `
		INSERT INTO content_marketing_node (node_id, workspace_id, status, origin, current_revision)
		VALUES ($1, $2, $3, $4, 1)
		RETURNING created_at, updated_at`, nodeID, workspaceID, status, origin).
		Scan(&node.CreatedAt, &node.UpdatedAt)
	if err != nil {
		return MarketingNode{}, ErrStorage
	}
	node.Current, err = s.insertRevision(ctx, tx, workspaceID, actor, nodeID, 1, ChangeCreate, status, content, note)
	if err != nil {
		return MarketingNode{}, err
	}
	return node, nil
}

func (s *Store) insertRevision(ctx context.Context, tx pgx.Tx, workspaceID, actor, nodeID string, revision int64, kind ChangeKind, statusAfter NodeStatus, content NodeContent, note string) (NodeRevision, error) {
	if content.Accounts == nil {
		content.Accounts = []NodeAccount{}
	}
	content.MaterialSourceIDs = normalizeStrings(content.MaterialSourceIDs)
	accounts, err := json.Marshal(content.Accounts)
	if err != nil {
		return NodeRevision{}, ErrInvalid
	}
	sources, err := encodeStrings(content.MaterialSourceIDs)
	if err != nil {
		return NodeRevision{}, ErrInvalid
	}
	row := NodeRevision{
		RevisionID: s.newID(), NodeID: nodeID, WorkspaceID: workspaceID, Revision: revision,
		ChangeKind: kind, StatusAfter: statusAfter, NodeContent: content, Note: note, Actor: actor,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO content_marketing_node_revision (
			revision_id, node_id, workspace_id, revision, change_kind, status_after,
			name, kind, starts_on, ends_on, timezone, lead_days, accounts, goal,
			material_source_ids, date_certainty, date_basis, note, actor
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::date,$10::date,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING created_at`,
		row.RevisionID, nodeID, workspaceID, revision, kind, statusAfter,
		content.Name, content.Kind, content.StartsOn, content.EndsOn, content.Timezone,
		content.LeadDays, accounts, content.Goal, sources, content.DateCertainty,
		content.DateBasis, note, actor).Scan(&row.CreatedAt)
	if err != nil {
		return NodeRevision{}, ErrStorage
	}
	return row, nil
}

func (s *Store) withTiming(node MarketingNode) (MarketingNode, error) {
	timing, err := ComputeTiming(s.now(), node.Current.NodeContent)
	if err != nil {
		return MarketingNode{}, err
	}
	node.Timing = timing
	return node, nil
}

// GetNode reads one node of this brand. A foreign or missing id is the same
// ErrNotFound (FR-042).
func (s *Store) GetNode(ctx context.Context, workspaceID, actor, nodeID string) (MarketingNode, error) {
	const step = "get-marketing-node"
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, ErrStorage)
		}
		return MarketingNode{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, ErrInvalid)
		return MarketingNode{}, ErrInvalid
	}
	node, err := s.scanNode(s.DB.QueryRow(ctx, nodeSelect+`
		WHERE n.workspace_id = $1 AND n.node_id = $2`, workspaceID, nodeID))
	if err != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
	}
	return node, err
}

// ListNodes returns the brand's nodes by start date, optionally one status.
func (s *Store) ListNodes(ctx context.Context, workspaceID, actor string, status NodeStatus) ([]MarketingNode, error) {
	const step = "list-marketing-nodes"
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportNodeFailure(ctx, workspaceID, actor, "", step, ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || (status != "" && !status.valid()) {
		s.reportNodeFailure(ctx, workspaceID, actor, "", step, ErrInvalid)
		return nil, ErrInvalid
	}
	where, args := ` WHERE n.workspace_id = $1`, []any{workspaceID}
	if status != "" {
		where += ` AND n.status = $2`
		args = append(args, status)
	}
	rows, err := s.DB.Query(ctx, nodeSelect+where+` ORDER BY r.starts_on, n.created_at, n.node_id`, args...)
	if err != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, "", step, err)
		return nil, ErrStorage
	}
	defer rows.Close()
	nodes := []MarketingNode{}
	for rows.Next() {
		node, scanErr := s.scanNode(rows)
		if scanErr != nil {
			s.reportNodeFailure(ctx, workspaceID, actor, "", step, scanErr)
			return nil, scanErr
		}
		nodes = append(nodes, node)
	}
	if rows.Err() != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, "", step, rows.Err())
		return nil, ErrStorage
	}
	return nodes, nil
}

// ListNodeRevisions returns a node's history, oldest first. A node that is not
// this brand's has no history to show: ErrNotFound, not an empty list.
func (s *Store) ListNodeRevisions(ctx context.Context, workspaceID, actor, nodeID string) ([]NodeRevision, error) {
	const step = "list-marketing-node-revisions"
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || nodeID == "" {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, ErrInvalid)
		return nil, ErrInvalid
	}
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM content_marketing_node WHERE workspace_id = $1 AND node_id = $2
	)`, workspaceID, nodeID).Scan(&exists); err != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
		return nil, ErrStorage
	}
	if !exists {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, ErrNotFound)
		return nil, ErrNotFound
	}
	rows, err := s.DB.Query(ctx, nodeRevisionSelect+`
		WHERE r.workspace_id = $1 AND r.node_id = $2 ORDER BY r.revision`, workspaceID, nodeID)
	if err != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, err)
		return nil, ErrStorage
	}
	defer rows.Close()
	revisions := []NodeRevision{}
	for rows.Next() {
		revision, scanErr := scanNodeRevision(rows)
		if scanErr != nil {
			s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, scanErr)
			return nil, scanErr
		}
		revisions = append(revisions, revision)
	}
	if rows.Err() != nil {
		s.reportNodeFailure(ctx, workspaceID, actor, nodeID, step, rows.Err())
		return nil, ErrStorage
	}
	return revisions, nil
}

const nodeRevisionColumns = `r.revision_id, r.node_id, r.workspace_id, r.revision,
	r.change_kind, r.status_after, r.name, r.kind,
	to_char(r.starts_on, 'YYYY-MM-DD'), to_char(r.ends_on, 'YYYY-MM-DD'),
	r.timezone, r.lead_days, r.accounts, r.goal, r.material_source_ids,
	r.date_certainty, r.date_basis, r.note, r.actor, r.created_at`

const nodeRevisionSelect = `SELECT ` + nodeRevisionColumns + `
	FROM content_marketing_node_revision r`

const nodeSelect = `SELECT n.node_id, n.workspace_id, n.status, n.origin,
	n.current_revision, n.created_at, n.updated_at, ` + nodeRevisionColumns + `
	FROM content_marketing_node n
	JOIN content_marketing_node_revision r
	  ON r.workspace_id = n.workspace_id AND r.node_id = n.node_id AND r.revision = n.current_revision`

func nodeRevisionTargets(r *NodeRevision, accounts, sources *[]byte) []any {
	return []any{&r.RevisionID, &r.NodeID, &r.WorkspaceID, &r.Revision,
		&r.ChangeKind, &r.StatusAfter, &r.Name, &r.Kind, &r.StartsOn, &r.EndsOn,
		&r.Timezone, &r.LeadDays, accounts, &r.Goal, sources,
		&r.DateCertainty, &r.DateBasis, &r.Note, &r.Actor, &r.CreatedAt}
}

func decodeNodeJSON(r *NodeRevision, accounts, sources []byte) error {
	r.Accounts = []NodeAccount{}
	if err := json.Unmarshal(accounts, &r.Accounts); err != nil {
		return ErrStorage
	}
	if r.Accounts == nil {
		r.Accounts = []NodeAccount{}
	}
	var err error
	r.MaterialSourceIDs, err = decodeStrings(sources)
	return err
}

func scanNodeRevision(row scanner) (NodeRevision, error) {
	var revision NodeRevision
	var accounts, sources []byte
	err := row.Scan(nodeRevisionTargets(&revision, &accounts, &sources)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return NodeRevision{}, ErrNotFound
	}
	if err != nil {
		return NodeRevision{}, ErrStorage
	}
	if err = decodeNodeJSON(&revision, accounts, sources); err != nil {
		return NodeRevision{}, err
	}
	return revision, nil
}

func (s *Store) scanNode(row scanner) (MarketingNode, error) {
	var node MarketingNode
	var accounts, sources []byte
	targets := append([]any{&node.NodeID, &node.WorkspaceID, &node.Status, &node.Origin,
		&node.CurrentRevision, &node.CreatedAt, &node.UpdatedAt},
		nodeRevisionTargets(&node.Current, &accounts, &sources)...)
	err := row.Scan(targets...)
	if errors.Is(err, pgx.ErrNoRows) {
		return MarketingNode{}, ErrNotFound
	}
	if err != nil {
		return MarketingNode{}, ErrStorage
	}
	if err = decodeNodeJSON(&node.Current, accounts, sources); err != nil {
		return MarketingNode{}, err
	}
	return s.withTiming(node)
}
