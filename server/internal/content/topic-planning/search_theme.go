package topicplanning

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
)

// Search theme storage (specs/036 PR 1): create, revise (archiving
// included), read, history and list.
//
// Every write follows contract §7.1 and FR-103, in this order:
//
//  1. shape checks, outside any transaction (pure);
//  2. begin, whose first statement is the workspace delete fence - a
//     workspace whose deletion has committed answers ErrNotFound here and
//     nothing below runs;
//  3. for a revision: the theme's current revision (ErrNotFound if none);
//  4. the account, materials, topic cards and briefs, inside the fence;
//  5. for a revision: base_revision against the current one (409);
//  6. the business row, then the audit row, in the same transaction.
//
// content_search_theme_revision is insert-only: nothing in this package
// updates or deletes it (search_guards_test.go). Two writers of the same
// revision number are told apart by the unique index on (workspace_id,
// theme_id, revision): the loser gets the same 409 a stale base gets.

const searchThemeColumns = `workspace_id, theme_id, revision, voided, name, platform, account_id,
	business_goal, questions, keywords, intent, origin, origin_note, source_ids, topic_card_ids,
	brief_revision_ids, note, recorded_by, created_at`

// themeInsertHook is a test seam carried on the context, never set in
// production: it runs after every check has passed and just before the
// revision INSERT, so a test can hold two writers there and prove that the
// unique index, not the read check, decides between them.
type themeInsertHook struct{}

func runThemeInsertHook(ctx context.Context) {
	if hook, ok := ctx.Value(themeInsertHook{}).(func()); ok {
		hook()
	}
}

type themeRow interface {
	Scan(dest ...any) error
}

func scanTheme(row themeRow) (SearchTheme, error) {
	var theme SearchTheme
	var workspaceID string
	var questions, keywords, sources, cards, briefs []byte
	err := row.Scan(&workspaceID, &theme.ThemeID, &theme.Revision, &theme.Voided, &theme.Name,
		&theme.Platform, &theme.AccountID, &theme.BusinessGoal, &questions, &keywords,
		&theme.Intent, &theme.Origin, &theme.OriginNote, &sources, &cards, &briefs,
		&theme.Note, &theme.RecordedBy, &theme.CreatedAt)
	if err != nil {
		return SearchTheme{}, err
	}
	for _, list := range []struct {
		raw    []byte
		target *[]string
	}{
		{questions, &theme.Questions}, {keywords, &theme.Keywords}, {sources, &theme.SourceIDs},
		{cards, &theme.TopicCardIDs}, {briefs, &theme.BriefRevisionIDs},
	} {
		values, decodeErr := decodeStrings(list.raw)
		if decodeErr != nil {
			return SearchTheme{}, ErrStorage
		}
		*list.target = values
	}
	return theme, nil
}

// checkThemeReferences refuses an account, material, topic card or brief
// that is not this brand's, naming the field (FR-013). A foreign id and a
// missing one get the same refusal, so the answer cannot be used to learn
// what exists elsewhere. It runs inside the fenced write transaction.
func (s *Store) checkThemeReferences(ctx context.Context, tx pgx.Tx, workspaceID string, content ThemeContent) error {
	if content.AccountID != "" {
		if s.Accounts == nil {
			return ErrStorage
		}
		account, err := s.Accounts.Get(ctx, workspaceID, content.AccountID)
		switch {
		case errors.Is(err, ipprofile.ErrNotFound), errors.Is(err, ErrNotFound):
			return FieldError{Field: "account_id", Reason: "not found"}
		case err != nil:
			return ErrStorage
		}
		if account.Platform != content.Platform {
			return FieldError{Field: "platform", Reason: "not the account's platform"}
		}
	}
	if len(content.SourceIDs) > 0 {
		if s.Sources == nil {
			return ErrStorage
		}
		for _, id := range content.SourceIDs {
			exists, err := s.Sources.Exists(ctx, workspaceID, id)
			switch {
			case errors.Is(err, ErrNotFound) || err == nil && !exists:
				return FieldError{Field: "source_ids", Reason: "not found"}
			case err != nil:
				return ErrStorage
			}
		}
	}
	for _, own := range []struct {
		field, sql string
		ids        []string
	}{
		{"topic_card_ids", `SELECT count(*) FROM content_topic_card WHERE workspace_id = $1 AND topic_card_id = ANY($2::text[])`, content.TopicCardIDs},
		{"brief_revision_ids", `SELECT count(*) FROM content_brief_revision WHERE workspace_id = $1 AND brief_revision_id = ANY($2::text[])`, content.BriefRevisionIDs},
	} {
		if len(own.ids) == 0 {
			continue
		}
		var found int
		if err := tx.QueryRow(ctx, own.sql, workspaceID, own.ids).Scan(&found); err != nil {
			return ErrStorage
		}
		if found != len(own.ids) {
			return FieldError{Field: own.field, Reason: "not found"}
		}
	}
	return nil
}

// insertTheme writes one revision row. A unique violation is a second
// writer of the same revision number that got past the read check.
func insertTheme(ctx context.Context, tx pgx.Tx, workspaceID string, theme SearchTheme) (SearchTheme, error) {
	encoded := make([][]byte, 0, 5)
	for _, list := range [][]string{theme.Questions, theme.Keywords, theme.SourceIDs, theme.TopicCardIDs, theme.BriefRevisionIDs} {
		raw, err := json.Marshal(normalizeStrings(list))
		if err != nil {
			return SearchTheme{}, ErrInvalid
		}
		encoded = append(encoded, raw)
	}
	runThemeInsertHook(ctx)
	row := tx.QueryRow(ctx, `
		INSERT INTO content_search_theme_revision (
			workspace_id, theme_id, revision, voided, name, platform, account_id, business_goal,
			questions, keywords, intent, origin, origin_note, source_ids, topic_card_ids,
			brief_revision_ids, note, recorded_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING `+searchThemeColumns,
		workspaceID, theme.ThemeID, theme.Revision, theme.Voided, theme.Name, theme.Platform,
		theme.AccountID, theme.BusinessGoal, encoded[0], encoded[1], theme.Intent, theme.Origin,
		theme.OriginNote, encoded[2], encoded[3], encoded[4], theme.Note, theme.RecordedBy)
	stored, err := scanTheme(row)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return SearchTheme{}, SearchConflict{Field: "base_revision"}
		}
		return SearchTheme{}, ErrStorage
	}
	return stored, nil
}

// reportThemeFailure logs a failed write. A conflict is the caller's input
// racing someone else's, logged as an input conflict, not as storage down.
func (s *Store) reportThemeFailure(ctx context.Context, workspaceID, actor, themeID, step string, err error) {
	if _, ok := errors.AsType[SearchConflict](err); ok {
		err = ErrInvalid
	}
	s.reportFailure(ctx, workspaceID, actor, themeID, step, err)
}

// CreateSearchTheme records revision 1 of a new theme.
func (s *Store) CreateSearchTheme(ctx context.Context, workspaceID, actor string, content ThemeContent) (SearchThemeView, error) {
	const step = "create-search-theme"
	if s == nil {
		return SearchThemeView{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportThemeFailure(ctx, workspaceID, actor, "", step, ErrInvalid)
		return SearchThemeView{}, ErrInvalid
	}
	content, err := NormalizeThemeContent(content)
	if err != nil {
		s.reportThemeFailure(ctx, workspaceID, actor, "", step, err)
		return SearchThemeView{}, err
	}
	themeID := s.newID()
	fail := func(err error) (SearchThemeView, error) {
		s.reportThemeFailure(ctx, workspaceID, actor, themeID, step, err)
		return SearchThemeView{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if err = s.checkThemeReferences(ctx, tx, workspaceID, content); err != nil {
		return fail(err)
	}
	stored, err := insertTheme(ctx, tx, workspaceID, SearchTheme{
		ThemeID: themeID, Revision: 1, ThemeContent: content, RecordedBy: actor,
	})
	if err != nil {
		return fail(err)
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, themeID, step); err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	return ViewTheme(stored), nil
}

// ReviseSearchTheme appends the next revision of a theme. The whole content
// is submitted; voided = true archives the theme, false keeps or makes it
// current again. Earlier revisions are never touched (FR-010, FR-016).
func (s *Store) ReviseSearchTheme(ctx context.Context, workspaceID, actor, themeID string, req ThemeRevisionRequest) (SearchThemeView, error) {
	step := "revise-search-theme"
	if req.Voided {
		step = "archive-search-theme"
	}
	if s == nil {
		return SearchThemeView{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || themeID == "" {
		s.reportThemeFailure(ctx, workspaceID, actor, themeID, step, ErrNotFound)
		return SearchThemeView{}, ErrNotFound
	}
	if req.BaseRevision < 1 {
		err := FieldError{Field: "base_revision", Reason: "required"}
		s.reportThemeFailure(ctx, workspaceID, actor, themeID, step, err)
		return SearchThemeView{}, err
	}
	content, err := NormalizeThemeContent(req.Content)
	if err != nil {
		s.reportThemeFailure(ctx, workspaceID, actor, themeID, step, err)
		return SearchThemeView{}, err
	}
	fail := func(err error) (SearchThemeView, error) {
		s.reportThemeFailure(ctx, workspaceID, actor, themeID, step, err)
		return SearchThemeView{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	current, err := scanTheme(tx.QueryRow(ctx, `SELECT `+searchThemeColumns+`
		FROM content_search_theme_revision WHERE workspace_id = $1 AND theme_id = $2
		ORDER BY revision DESC LIMIT 1`, workspaceID, themeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(ErrNotFound)
	}
	if err != nil {
		return fail(ErrStorage)
	}
	if err = s.checkThemeReferences(ctx, tx, workspaceID, content); err != nil {
		return fail(err)
	}
	if req.BaseRevision != current.Revision {
		return fail(SearchConflict{Field: "base_revision"})
	}
	stored, err := insertTheme(ctx, tx, workspaceID, SearchTheme{
		ThemeID: themeID, Revision: current.Revision + 1, Voided: req.Voided,
		ThemeContent: content, RecordedBy: actor,
	})
	if err != nil {
		return fail(err)
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, themeID, step); err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	return ViewTheme(stored), nil
}

// GetSearchTheme answers a theme's current revision, archived or not.
func (s *Store) GetSearchTheme(ctx context.Context, workspaceID, actor, themeID string) (SearchThemeView, error) {
	if s == nil || s.DB == nil {
		return SearchThemeView{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || themeID == "" {
		return SearchThemeView{}, ErrNotFound
	}
	theme, err := scanTheme(s.DB.QueryRow(ctx, `SELECT `+searchThemeColumns+`
		FROM content_search_theme_revision WHERE workspace_id = $1 AND theme_id = $2
		ORDER BY revision DESC LIMIT 1`, workspaceID, themeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SearchThemeView{}, ErrNotFound
	}
	if err != nil {
		return SearchThemeView{}, ErrStorage
	}
	return ViewTheme(theme), nil
}

func (s *Store) queryThemes(ctx context.Context, sql string, args ...any) ([]SearchTheme, error) {
	rows, err := s.DB.Query(ctx, sql, args...)
	if err != nil {
		return nil, ErrStorage
	}
	defer rows.Close()
	themes := []SearchTheme{}
	for rows.Next() {
		theme, scanErr := scanTheme(rows)
		if scanErr != nil {
			return nil, ErrStorage
		}
		themes = append(themes, theme)
	}
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	return themes, nil
}

// ListSearchThemeRevisions answers every revision of a theme, newest first
// (FR-016): the record of who wrote what, and when.
func (s *Store) ListSearchThemeRevisions(ctx context.Context, workspaceID, actor, themeID string) ([]SearchThemeView, error) {
	if s == nil || s.DB == nil {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || themeID == "" {
		return nil, ErrNotFound
	}
	themes, err := s.queryThemes(ctx, `SELECT `+searchThemeColumns+`
		FROM content_search_theme_revision WHERE workspace_id = $1 AND theme_id = $2
		ORDER BY revision DESC`, workspaceID, themeID)
	if err != nil {
		return nil, err
	}
	if len(themes) == 0 {
		return nil, ErrNotFound
	}
	views := make([]SearchThemeView, 0, len(themes))
	for _, theme := range themes {
		views = append(views, ViewTheme(theme))
	}
	return views, nil
}

// ListSearchThemes answers the current revision of each theme, filtered in
// Go because the id lists are jsonb (plan: Technical Context), and ordered
// by name, then theme_id - never by any number (FR-015).
func (s *Store) ListSearchThemes(ctx context.Context, workspaceID, actor string, filter ThemeFilter) ([]SearchThemeView, error) {
	if s == nil || s.DB == nil {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrNotFound
	}
	themes, err := s.queryThemes(ctx, `SELECT DISTINCT ON (theme_id) `+searchThemeColumns+`
		FROM content_search_theme_revision WHERE workspace_id = $1
		ORDER BY theme_id, revision DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	views := []SearchThemeView{}
	for _, theme := range themes {
		if theme.Voided && !filter.IncludeArchived {
			continue
		}
		if filter.Platform != "" && theme.Platform != filter.Platform {
			continue
		}
		if filter.AccountID != "" && theme.AccountID != filter.AccountID {
			continue
		}
		if filter.TopicCardID != "" && !slices.Contains(theme.TopicCardIDs, filter.TopicCardID) {
			continue
		}
		views = append(views, ViewTheme(theme))
	}
	slices.SortFunc(views, func(a, b SearchThemeView) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ThemeID, b.ThemeID))
	})
	return views, nil
}
